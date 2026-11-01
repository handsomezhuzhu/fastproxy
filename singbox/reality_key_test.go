//go:build with_utls

package singbox

import (
	"crypto/ecdh"
	"encoding/base64"
	"testing"
)

// TestRealityKeyPairRoundTrip pins the key encoding REALITY servers and
// clients exchange: raw (unpadded) base64url of the X25519 private/public key.
func TestRealityKeyPairRoundTrip(t *testing.T) {
	priv, pub := generateRealityKeyPair(t)
	for name, k := range map[string]string{"private": priv, "public": pub} {
		raw, err := base64.RawURLEncoding.DecodeString(k)
		if err != nil {
			t.Fatalf("%s key is not raw base64url: %v", name, err)
		}
		if len(raw) != 32 {
			t.Fatalf("%s key is %d bytes, want 32", name, len(raw))
		}
		if _, err := ecdh.X25519().NewPrivateKey(raw); name == "private" && err != nil {
			t.Fatalf("private key rejected: %v", err)
		}
	}
}
