package config

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// UpstreamSecurity values for Upstream.Security.
const (
	SecurityNone    = "none"
	SecurityTLS     = "tls"
	SecurityReality = "reality"
)

// UpstreamFlowVision is the only VLESS flow FastProxy accepts.
const UpstreamFlowVision = "xtls-rprx-vision"

// UpstreamTransports lists the accepted transport values ("" means plain TCP).
var UpstreamTransports = []string{"tcp", "ws", "grpc"}

// VMessSecurities lists the accepted VMess security values.
var VMessSecurities = []string{"auto", "none", "zero", "aes-128-gcm", "chacha20-poly1305"}

// normalizeTLSLike fields applies defaults shared by VLESS and VMess and
// validates security/transport/flow. It must run after Type is canonical.
func (in *Upstream) normalizeTLSLike() error {
	in.Security = strings.ToLower(strings.TrimSpace(in.Security))
	in.PBK = strings.TrimSpace(in.PBK)
	in.SID = strings.ToLower(strings.TrimSpace(in.SID))
	in.Flow = strings.ToLower(strings.TrimSpace(in.Flow))
	in.Fingerprint = strings.TrimSpace(in.Fingerprint)
	in.Transport = strings.ToLower(strings.TrimSpace(in.Transport))
	in.Path = strings.TrimSpace(in.Path)
	in.HostHeader = strings.TrimSpace(in.HostHeader)
	in.ServiceName = strings.TrimSpace(in.ServiceName)
	in.VMessSecurity = strings.ToLower(strings.TrimSpace(in.VMessSecurity))

	if in.Security == "" {
		if in.TLS {
			in.Security = SecurityTLS
		} else {
			in.Security = SecurityNone
		}
	}
	switch in.Security {
	case SecurityNone, SecurityTLS, SecurityReality:
	default:
		return fmt.Errorf("security 仅支持 none/tls/reality（当前 %q）", in.Security)
	}
	if in.Transport == "" {
		in.Transport = "tcp"
	}
	switch in.Transport {
	case "tcp", "ws", "grpc":
	default:
		return fmt.Errorf("传输方式仅支持 tcp/ws/grpc（当前 %q）", in.Transport)
	}
	if in.Fingerprint == "" && in.Security == SecurityReality {
		in.Fingerprint = "chrome"
	}
	// TLS (and REALITY) are TLS-based: keep the legacy bool in sync so the
	// native VLESS client and older clients agree.
	in.TLS = in.Security == SecurityTLS || in.Security == SecurityReality
	return nil
}

// validateVless applies VLESS-specific rules after normalizeTLSLike.
func (in *Upstream) validateVless() error {
	switch in.Flow {
	case "", UpstreamFlowVision:
	default:
		return fmt.Errorf("VLESS flow 仅支持空或 %s（当前 %q）", UpstreamFlowVision, in.Flow)
	}
	if in.Flow != "" && in.Security != SecurityTLS && in.Security != SecurityReality {
		return fmt.Errorf("VLESS flow=%s 需要开启 TLS 或 REALITY", in.Flow)
	}
	if in.Security == SecurityReality {
		if in.PBK == "" {
			return errors.New("REALITY 上游必须填写公钥 pbk")
		}
		if !validRealityPublicKey(in.PBK) {
			return errors.New("REALITY 公钥 pbk 格式无效（应为 32 字节 X25519 公钥的 base64url）")
		}
		if in.SID != "" && !validRealityShortID(in.SID) {
			return errors.New("REALITY short id 格式无效（应为不超过 16 位的十六进制）")
		}
	}
	if in.Transport == "grpc" && in.ServiceName == "" {
		in.ServiceName = ""
	}
	return nil
}

// validateVmess applies VMess-specific rules after normalizeTLSLike.
func (in *Upstream) validateVmess() error {
	if in.VMessSecurity == "" {
		in.VMessSecurity = "auto"
	}
	for _, s := range VMessSecurities {
		if in.VMessSecurity == s {
			return nil
		}
	}
	return fmt.Errorf("VMess 加密方式仅支持 %s（当前 %q）", strings.Join(VMessSecurities, "/"), in.VMessSecurity)
}

func init() {
	// Hook invoked by NormalizeUpstream for vless/vmess types (keeps the
	// per-type switch readable).
	validateProtocolUpstream = func(in *Upstream) error {
		if in.Type != UpstreamVless && in.Type != UpstreamVmess {
			return nil
		}
		if err := in.normalizeTLSLike(); err != nil {
			return err
		}
		if in.Type == UpstreamVless {
			return in.validateVless()
		}
		return in.validateVmess()
	}
}

// validateProtocolUpstream is set in init to avoid a forward reference inside
// NormalizeUpstream's switch.
var validateProtocolUpstream func(*Upstream) error

// validRealityPublicKey accepts the base64 encodings used by share links
// (unpadded base64url is the norm; padded/standard forms also decode).
func validRealityPublicKey(s string) bool {
	encodings := []*base64.Encoding{
		base64.RawURLEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.StdEncoding,
	}
	for _, enc := range encodings {
		if raw, err := enc.DecodeString(s); err == nil && len(raw) == 32 {
			return true
		}
	}
	return false
}

// validRealityShortID accepts up to 8 bytes of hex (0-16 chars).
func validRealityShortID(s string) bool {
	if len(s) == 0 || len(s) > 16 || len(s)%2 != 0 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
