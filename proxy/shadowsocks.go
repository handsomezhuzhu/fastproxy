package proxy

import (
	"fmt"
	"net"
	"strings"
	"time"

	sscore "github.com/shadowsocks/go-shadowsocks2/core"

	"github.com/handsomezhuzhu/fastproxy/config"
)

// shadowsocksCipher builds the AEAD stream cipher for a Shadowsocks upstream.
// Only AEAD ciphers are accepted; legacy stream ciphers are refused with a
// clear error instead of silently weakening the tunnel.
//
// A fresh Cipher is required per connection: PickCipher attaches a replay
// filter (salt bloom filter) to the returned cipher, so sharing one instance
// would reject every connection after the first as a "repeated salt".
func shadowsocksCipher(up config.Upstream) (sscore.Cipher, error) {
	method := strings.ToLower(strings.TrimSpace(up.Method))
	if method == "" {
		return nil, fmt.Errorf("Shadowsocks 上游缺少加密方式（method）")
	}
	if up.Password == "" {
		return nil, fmt.Errorf("Shadowsocks 上游缺少密码")
	}
	switch method {
	case "aes-128-gcm", "aes-256-gcm", "chacha20-ietf-poly1305":
	default:
		return nil, fmt.Errorf("暂不支持的 Shadowsocks 加密方式 %q（支持: %s）",
			method, strings.Join(config.ShadowsocksMethods, ", "))
	}
	ciph, err := sscore.PickCipher(method, nil, up.Password)
	if err != nil {
		return nil, fmt.Errorf("Shadowsocks 加密初始化失败: %w", err)
	}
	return ciph, nil
}

// dialShadowsocks opens a Shadowsocks (AEAD TCP) tunnel to addr: the client
// dials the server, wraps the connection in the stream cipher and writes the
// target address as the first plaintext block. No server acknowledgement
// exists — failures surface as relay errors or EOF on the first read.
func dialShadowsocks(up config.Upstream, addr string, timeout time.Duration) (net.Conn, error) {
	ciph, err := shadowsocksCipher(up)
	if err != nil {
		return nil, err
	}
	raw, err := net.DialTimeout("tcp", up.Host, timeout)
	if err != nil {
		return nil, fmt.Errorf("连接上游代理 %s 失败: %w", up.Host, err)
	}
	target, err := encodeAddr(addr)
	if err != nil {
		raw.Close()
		return nil, err
	}
	shadow := ciph.StreamConn(raw)
	raw.SetDeadline(time.Now().Add(timeout))
	if _, err := shadow.Write(target); err != nil {
		shadow.Close()
		return nil, fmt.Errorf("Shadowsocks 发送目标地址失败: %w", err)
	}
	raw.SetDeadline(time.Time{})
	return shadow, nil
}
