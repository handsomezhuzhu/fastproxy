package admin

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
)

const pbkdf2Iters = 120000

// HashPassword derives a PBKDF2-SHA256 hash in a checkable format.
func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	dk, err := pbkdf2.Key(sha256.New, password, salt, pbkdf2Iters, 32)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", pbkdf2Iters, hex.EncodeToString(salt), hex.EncodeToString(dk)), nil
}

// VerifyPassword checks a password against a stored hash in constant time.
func VerifyPassword(hash, password string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	var iters int
	if _, err := fmt.Sscanf(parts[1], "%d", &iters); err != nil || iters < 1 || iters > 10_000_000 {
		return false
	}
	salt, err1 := hex.DecodeString(parts[2])
	dk, err2 := hex.DecodeString(parts[3])
	if err1 != nil || err2 != nil || len(salt) == 0 || len(dk) == 0 {
		return false
	}
	candidate, err := pbkdf2.Key(sha256.New, password, salt, iters, len(dk))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(candidate, dk) == 1
}
