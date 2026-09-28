package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

var ErrNoKey = errors.New("encryption key not configured (set ENCRYPTION_KEY or BOT_SECRETS_KEY)")

// LoadKey reads BOT_SECRETS_KEY or ENCRYPTION_KEY. Accepts raw 32-byte, hex, or any string (hashed to 32 bytes).
func LoadKey() ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv("BOT_SECRETS_KEY"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("ENCRYPTION_KEY"))
	}
	if raw == "" {
		return nil, ErrNoKey
	}
	if len(raw) == 32 {
		return []byte(raw), nil
	}
	// Decode base64 if looks like it
	if b, err := base64.StdEncoding.DecodeString(raw); err == nil && (len(b) == 16 || len(b) == 24 || len(b) == 32) {
		sum := sha256.Sum256(b)
		return sum[:], nil
	}
	sum := sha256.Sum256([]byte(raw))
	return sum[:], nil
}

// Encrypt returns base64(nonce|ciphertext) using AES-GCM.
func Encrypt(key, plaintext []byte) (string, error) {
	if len(key) != 32 {
		sum := sha256.Sum256(key)
		key = sum[:]
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	out := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt reverses Encrypt.
func Decrypt(key []byte, encoded string) ([]byte, error) {
	if len(key) != 32 {
		sum := sha256.Sum256(key)
		key = sum[:]
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, fmt.Errorf("ciphertext decode: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(raw) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ct := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ct, nil)
}
