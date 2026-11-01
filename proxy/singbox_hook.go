package proxy

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/handsomezhuzhu/fastproxy/config"
)

// needsSingBox mirrors singbox.NeedsSingBox without importing that package
// (the singbox package depends on config only, but keeping the rule local
// lets the proxy package stay free of the heavy sing-box dependency graph).
func needsSingBox(up config.Upstream) bool {
	switch up.Type {
	case config.UpstreamVmess:
		return true
	case config.UpstreamVless:
		return up.Security == config.SecurityReality ||
			up.Flow != "" || up.Transport == "ws" || up.Transport == "grpc"
	}
	return false
}

// dialViaSingBox routes the connection through the embedded sing-box manager.
func dialViaSingBox(up config.Upstream, addr string, timeout time.Duration) (net.Conn, error) {
	if SingBoxDialer == nil {
		return nil, fmt.Errorf("%w：上游 %s 需要 VMess/REALITY/Vision/WS/gRPC 出站", ErrVmessUnsupported, up.Name)
	}
	conn, err := SingBoxDialer(up, addr, timeout)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, errors.New("内置 sing-box 返回了空连接")
	}
	return conn, nil
}
