//go:build with_utls

package singbox

import (
	"testing"
)

// REALITY end-to-end tests are intentionally absent: during development the
// embedded REALITY client and a REALITY server built from the same sing-box
// version never completed the handshake ("REALITY: processed invalid
// connection"), so the path is treated as unverified rather than reported as
// working. The wiring (outbound tls.utls + tls.reality, flow, key encoding)
// is covered by unit tests above.
func TestRealityEndToEndNotImplemented(t *testing.T) {
	t.Skip("REALITY handshake between embedded client and test server did not complete; see report")
}
