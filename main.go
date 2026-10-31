// FastProxy — lightweight multi-port HTTP/SOCKS5 proxy forwarder with an
// admin panel. Each listening port is bound to one upstream proxy; inbound
// clients must authenticate with the rule's username/password.
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
	"github.com/handsomezhuzhu/fastproxy/proxy"
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

	cfgPath := getenv("CONFIG", "fastproxy.json")
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
	mgr := proxy.NewManager(store)
	defer mgr.Close()
	if err := mgr.Sync(); err != nil {
		log.Printf("部分端口启动失败: %v", err)
	}
	for _, p := range store.Ports() {
		if p.Enabled {
			log.Printf("线路: 端口 %d -> 上游 %s（备注: %s）", p.Port, orDirect(p.DefaultUpstream), remarkOrDash(p.Remark))
		}
	}
	log.Printf("账户共 %d 个", len(store.Accounts()))

	// Admin panel.
	adm := admin.NewServer(store, mgr, version)
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
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

func orDirect(id string) string {
	if id == "" {
		return "直连"
	}
	return id
}

func remarkOrDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
