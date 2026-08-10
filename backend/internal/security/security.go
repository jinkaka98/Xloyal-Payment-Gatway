package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"strings"
)

func HashSecret(value string) string {
	sum := sha256.Sum256([]byte(value))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

type Cipher struct{ aead cipher.AEAD }

func NewCipher(key []byte) (*Cipher, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

func (c *Cipher) Encrypt(plaintext []byte) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(c.aead.Seal(nonce, nonce, plaintext, nil)), nil
}

func (c *Cipher) Decrypt(encoded string) ([]byte, error) {
	data, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	n := c.aead.NonceSize()
	if len(data) < n {
		return nil, errors.New("invalid ciphertext")
	}
	return c.aead.Open(nil, data[:n], data[n:], nil)
}

var sensitive = map[string]bool{"authorization": true, "api_key": true, "apikey": true, "token": true, "secret": true, "password": true, "credential": true}

func RedactAttrs(_ []string, a slog.Attr) slog.Attr {
	if sensitive[strings.ToLower(a.Key)] {
		a.Value = slog.StringValue("[REDACTED]")
	}
	if a.Value.Kind() == slog.KindGroup {
		a.Value = slog.GroupValue(redactGroup(a.Value.Group())...)
	}
	return a
}
func redactGroup(attrs []slog.Attr) []slog.Attr {
	for i := range attrs {
		attrs[i] = RedactAttrs(nil, attrs[i])
	}
	return attrs
}
