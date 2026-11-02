package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// SecretPrefix marks an encrypted password stored in the database. Values
// without it are legacy plaintext and are read as-is (v3 encrypts them).
const SecretPrefix = "enc:v1:"

// secretKeyEnv is the environment variable that pins the encryption key.
const secretKeyEnv = "FASTPROXY_SECRET_KEY"

// SecretKeyFile is the per-database key file created next to the SQLite file
// when no key is supplied through FASTPROXY_SECRET_KEY.
const SecretKeyFile = ".secret_key"

// ErrSecretKeyMismatch is returned when a stored ciphertext cannot be opened
// with the current key (key file replaced, FASTPROXY_SECRET_KEY changed, or a
// database copied without its key).
var ErrSecretKeyMismatch = errors.New(
	"无法解密已保存的密码：加密密钥与数据库不匹配（请恢复同目录的 " + SecretKeyFile + " 或原来的 FASTPROXY_SECRET_KEY）")

// secretCipher seals and opens password values with AES-256-GCM.
type secretCipher struct {
	aead cipher.AEAD
}

var (
	activeCipherMu sync.RWMutex
	activeCipher   *secretCipher
)

// IsSealed reports whether a stored value carries the encryption prefix.
func IsSealed(v string) bool { return strings.HasPrefix(v, SecretPrefix) }

// SetSecretKey installs the process-wide key used to seal and open passwords.
// It is called by Open; an already installed key is replaced only when the
// new one is identical or when force is true (tests).
func SetSecretKey(key []byte, force bool) error {
	c, err := newSecretCipher(key)
	if err != nil {
		return err
	}
	activeCipherMu.Lock()
	defer activeCipherMu.Unlock()
	if activeCipher != nil && !force {
		return nil
	}
	activeCipher = c
	return nil
}

// ParseSecretKey accepts the FASTPROXY_SECRET_KEY encodings: 32 bytes as
// standard/URL base64 (padded or raw) or 64 hex characters.
func ParseSecretKey(raw string) ([]byte, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, errors.New("FASTPROXY_SECRET_KEY 为空")
	}
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding,
	} {
		if b, err := enc.DecodeString(s); err == nil && len(b) == 32 {
			return b, nil
		}
	}
	if len(s) == 64 {
		if b, err := hex.DecodeString(s); err == nil && len(b) == 32 {
			return b, nil
		}
	}
	return nil, errors.New("FASTPROXY_SECRET_KEY 需为 32 字节密钥（base64 标准/URL 或 64 位 hex）")
}

func newSecretCipher(key []byte) (*secretCipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("加密密钥长度应为 32 字节（当前 %d）", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &secretCipher{aead: aead}, nil
}

// seal encrypts plaintext into "enc:v1:<base64url(nonce|ciphertext)>".
func (c *secretCipher) seal(plaintext string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	// Additional data binds the ciphertext to the v1 format so a payload can
	// not be replayed under a future scheme without failing authentication.
	out := c.aead.Seal(nonce, nonce, []byte(plaintext), []byte(SecretPrefix))
	return SecretPrefix + base64.RawURLEncoding.EncodeToString(out), nil
}

// open decrypts a value produced by seal.
func (c *secretCipher) open(sealed string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(sealed, SecretPrefix))
	if err != nil || len(raw) < c.aead.NonceSize() {
		return "", ErrSecretKeyMismatch
	}
	nonce, body := raw[:c.aead.NonceSize()], raw[c.aead.NonceSize():]
	plain, err := c.aead.Open(nil, nonce, body, []byte(SecretPrefix))
	if err != nil {
		return "", ErrSecretKeyMismatch
	}
	return string(plain), nil
}

// loadSecretKey resolves the encryption key and installs it as the
// process-wide cipher (replacing any previously installed key): FASTPROXY_SECRET_KEY
// first, then (creating when absent) <db dir>/.secret_key with mode 0600.
func loadSecretKey(dbPath string) error {
	if raw := os.Getenv(secretKeyEnv); raw != "" {
		key, err := ParseSecretKey(raw)
		if err != nil {
			return fmt.Errorf("环境变量 %s 无效: %w", secretKeyEnv, err)
		}
		return SetSecretKey(key, true)
	}
	dir := filepath.Dir(dbPath)
	if dir == "" {
		dir = "."
	}
	path := filepath.Join(dir, SecretKeyFile)
	key, err := readOrCreateKeyFile(path)
	if err != nil {
		return err
	}
	return SetSecretKey(key, true)
}

// readOrCreateKeyFile loads the raw 32-byte key file, generating one with 0600
// permissions when it does not exist yet.
func readOrCreateKeyFile(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err == nil {
		key, perr := ParseSecretKey(string(raw))
		if perr != nil {
			return nil, fmt.Errorf("密钥文件 %s 无效: %w", path, perr)
		}
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("读取密钥文件 %s 失败: %w", path, err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	// O_EXCL: if another process won the race, read its key instead.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return readOrCreateKeyFile(path)
		}
		return nil, fmt.Errorf("创建密钥文件 %s 失败: %w", path, err)
	}
	if _, err := f.WriteString(base64.RawURLEncoding.EncodeToString(key) + "\n"); err != nil {
		f.Close()
		return nil, fmt.Errorf("写入密钥文件 %s 失败: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("写入密钥文件 %s 失败: %w", path, err)
	}
	// Some umasks/ACLs can widen the mode requested above; enforce 0600.
	_ = os.Chmod(path, 0o600)
	return key, nil
}

func currentCipher() (*secretCipher, error) {
	activeCipherMu.RLock()
	c := activeCipher
	activeCipherMu.RUnlock()
	if c == nil {
		return nil, errors.New("加密密钥尚未初始化")
	}
	return c, nil
}

// sealSecret encrypts a password for storage ("" stays "").
func sealSecret(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	c, err := currentCipher()
	if err != nil {
		return "", err
	}
	return c.seal(plaintext)
}

// openSecret decrypts a stored password. Legacy plaintext (no prefix) is
// returned unchanged; a sealed value under a different key is an error, never
// silently emptied.
func openSecret(stored string) (string, error) {
	if !IsSealed(stored) {
		return stored, nil
	}
	c, err := currentCipher()
	if err != nil {
		return "", err
	}
	return c.open(stored)
}
