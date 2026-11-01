package proxy

import "crypto/x509"

// testRoots is an extra certificate pool honoured by outbound TLS handshakes
// (vless/vmess). It is always nil in production builds; the test suite sets
// it to trust its throwaway self-signed certificate. This keeps the client
// strictly verifying in production (no InsecureSkipVerify switch anywhere).
var testRoots *x509.CertPool
