package security

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestEncryptionAndLogRedaction(t *testing.T) {
	c, err := NewCipher(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	ct, err := c.Encrypt([]byte("fixture-secret"))
	if err != nil {
		t.Fatal(err)
	}
	pt, err := c.Decrypt(ct)
	if err != nil || string(pt) != "fixture-secret" || strings.Contains(ct, "fixture-secret") {
		t.Fatal("cipher round trip failed")
	}
	var out bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&out, &slog.HandlerOptions{ReplaceAttr: RedactAttrs}))
	log.Info("request", "authorization", "fixture-secret")
	if strings.Contains(out.String(), "fixture-secret") || !strings.Contains(out.String(), "[REDACTED]") {
		t.Fatalf("not redacted: %s", out.String())
	}
}
