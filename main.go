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
	"github.com/handsomezhuzhu/fastproxy/subscription"
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

	// Inbound port window: startup-level configuration, validated before
	// anything touches the database (an invalid value must abort the boot).
	portRange, err := config.ParsePortRange(os.Getenv("PORT_RANGE"))
	if err != nil {
		log.Fatalf("%v（示例：PORT_RANGE=10001-10050，或单端口 PORT_RANGE=10080；默认 %s）",
			err, config.DefaultPortRange)
	}

	store, err := config.OpenWithPortRange(cfgPath, adminPort, portRange)
	if err != nil {
		log.Fatalf("读取配置 %s 失败: %v", cfgPath, err)
	}
	defer store.Close()
	log.Printf("线路端口范围: %s", store.PortRange())

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

	// Embedded sing-box: every VLESS and VMess upstream is tunnelled through
	// it (each gets a loopback SOCKS5 inbound inside the box; proxy.DialTunnel
	// reaches it through proxy.SingBoxDialer). A failure here is not fatal:
	// HTTP, SOCKS5 and Shadowsocks keep working natively, while VLESS/VMess
	// upstreams report the box error.
	sbMgr := singbox.New(store, logger)
	defer sbMgr.Close()
	if err := sbMgr.Start(context.Background()); err != nil {
		logger.Warnf("内置 sing-box 启动失败（VLESS/VMess 上游将不可用）: %v", err)
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

	// Remote subscriptions: refresh due lists in the background and reconcile
	// their nodes with the upstream pool (manual upstreams are untouched).
	subCtx, subCancel := context.WithCancel(context.Background())
	defer subCancel()
	subMgr := subscription.NewManager(store, logger, func() {
		if err := sbMgr.Reload(context.Background()); err != nil {
			logger.Warnf("订阅更新后重建 sing-box 失败: %v", err)
		}
	})
	adm.SetSubscriptions(subMgr)
	go subMgr.Start(subCtx)
	policy, problems := admin.EnvLoginPolicy()
	for _, p := range problems {
		logger.Warnf("%v", p)
	}
	adm.SetLoginPolicy(policy.MaxAttempts, policy.Lock)
	logger.Infof("登录限制: 连续失败 %d 次后锁定 %s", policy.MaxAttempts, policy.Lock)
	// Reverse proxy support: only peers in TRUSTED_PROXIES may supply
	// X-Forwarded-For/Proto (see docs/REVERSE_PROXY.md).
	trusted, err := admin.TrustedProxiesFromEnv()
	if err != nil {
		log.Fatalf("%v（示例：TRUSTED_PROXIES=127.0.0.1/32,172.16.0.0/12）", err)
	}
	adm.SetTrustedProxies(trusted)
	if len(trusted) > 0 {
		logger.Infof("已信任反向代理: %s", os.Getenv(admin.TrustedProxiesEnv))
	}
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
