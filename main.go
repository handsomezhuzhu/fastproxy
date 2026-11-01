// FastProxy — lightweight multi-port HTTP/SOCKS5 proxy forwarder with an
// admin panel. Each listening port is bound to an upstream proxy; inbound
// clients must authenticate with an account.
package main

import (
	"context"
	"crypto/rand"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/handsomezhuzhu/fastproxy/admin"
	"github.com/handsomezhuzhu/fastproxy/config"
	"github.com/handsomezhuzhu/fastproxy/logx"
	"github.com/handsomezhuzhu/fastproxy/probe"
	"github.com/handsomezhuzhu/fastproxy/proxy"
	"github.com/handsomezhuzhu/fastproxy/singbox"
)

// version is overridable at build time: -ldflags "-X main.version=v1.0.0"
var version = "dev"

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	log.SetFlags(log.LstdFlags)
	log.Printf("FastProxy %s 启动中...", version)

	cfgPath := getenv("CONFIG", "fastproxy.db")
	adminAddr := getenv("ADMIN_ADDR", ":7788")

	_, portStr, err := net.SplitHostPort(adminAddr)
	if err != nil {
		log.Fatalf("ADMIN_ADDR %q 格式错误（应为 :7788 或 127.0.0.1:7788）: %v", adminAddr, err)
	}
	adminPort, err := strconv.Atoi(portStr)
	if err != nil || adminPort <= 0 || adminPort > 65535 {
		log.Fatalf("ADMIN_ADDR 端口无效: %q", portStr)
	}

	store, err := config.Open(cfgPath, adminPort)
	if err != nil {
		log.Fatalf("读取配置 %s 失败: %v", cfgPath, err)
	}
	defer store.Close()

	// Structured logger: SQLite is authoritative (queryable history, retention
	// pruning); the in-memory ring covers the interval before batch flush.
	settings := store.Settings()
	logger := logx.New("", settings.LogBuffer, logx.ParseLevel(settings.LogLevel), settings.AccessLog)
	logger.SetPersist(store)
	defer logger.Close()

	// Signal context for the whole process (prober + server).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Prune expired access logs at startup and hourly (reads live settings).
	prune := func() {
		days := store.Settings().LogRetentionDays
		if n, err := store.PruneLogs(days); err == nil && n > 0 {
			logger.Infof("清理过期日志 %d 条（保留 %d 天）", n, days)
		}
	}
	go func() {
		prune()
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				prune()
			}
		}
	}()

	// Active probing: upstream health, latency and exit IP (periodic).
	prober := probe.NewRunner(store, logger)
	go prober.Start(ctx)

	// Admin password: ADMIN_PASSWORD env wins; otherwise generate one on
	// first boot and print it once.
	if pw := getenv("ADMIN_PASSWORD", ""); pw != "" {
		hash, err := admin.HashPassword(pw)
		if err != nil {
			log.Fatalf("生成密码哈希失败: %v", err)
		}
		if err := store.SetAdminPasswordHash(hash); err != nil {
			log.Fatalf("保存管理密码失败: %v", err)
		}
		log.Println("已通过环境变量 ADMIN_PASSWORD 设置管理密码")
	}
	if store.AdminPasswordHash() == "" {
		pw := randomPassword(12)
		hash, err := admin.HashPassword(pw)
		if err != nil {
			log.Fatalf("生成初始密码失败: %v", err)
		}
		if err := store.SetAdminPasswordHash(hash); err != nil {
			log.Fatalf("保存初始密码失败: %v", err)
		}
		log.Print("++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++")
		log.Printf("++ 初始管理密码: %s （仅本次打印，请登录面板后尽快修改）", pw)
		log.Print("++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++")
	}

	// Start proxy listeners.
	mgr := proxy.NewManager(store, logger)
	defer mgr.Close()
	if err := mgr.Sync(); err != nil {
		log.Printf("部分端口启动失败: %v", err)
	}

	// Embedded sing-box: fastproxy carries VMess plus VLESS with REALITY,
	// XTLS Vision, WebSocket or gRPC. Each such upstream gets a loopback
	// SOCKS5 inbound inside the embedded box; proxy.DialTunnel reaches it
	// through proxy.SingBoxDialer. A failure here is not fatal: plain
	// VLESS/TLS, HTTP, SOCKS5 and Shadowsocks keep working natively.
	sbMgr := singbox.New(store, logger)
	defer sbMgr.Close()
	if err := sbMgr.Start(context.Background()); err != nil {
		logger.Warnf("内置 sing-box 启动失败（VMess/REALITY 上游将不可用）: %v", err)
	} else if ports := sbMgr.Ports(); len(ports) > 0 {
		log.Printf("内置 sing-box 已为 %d 个上游提供出站", len(ports))
	}
	proxy.SingBoxDialer = sbMgr.Dial

	for _, p := range store.Ports() {
		if p.Enabled {
			log.Printf("线路: 端口 %d（备注: %s）", p.Port, remarkOrDash(p.Remark))
		}
	}
	log.Printf("账户共 %d 个，上游共 %d 个", len(store.Accounts()), len(store.Upstreams()))

	// Admin panel + API.
	adm := admin.NewServer(store, mgr, logger, prober, version)
	adm.SetSingBox(sbMgr)
	httpServer := &http.Server{
		Addr:              adminAddr,
		Handler:           adm.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	ln, err := net.Listen("tcp", adminAddr)
	if err != nil {
		log.Fatalf("管理面板监听 %s 失败: %v", adminAddr, err)
	}
	log.Printf("管理面板: http://%s", ln.Addr())

	go func() {
		<-ctx.Done()
		timeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpServer.Shutdown(timeout)
	}()

	if err := httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
		log.Fatalf("管理面板异常退出: %v", err)
	}
	log.Println("FastProxy 已退出")
}

// randomPassword generates a readable random password without ambiguous chars.
func randomPassword(n int) string {
	const pool = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failure is unrecoverable
	}
	for i := range b {
		b[i] = pool[int(b[i])%len(pool)]
	}
	return string(b)
}

func remarkOrDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
