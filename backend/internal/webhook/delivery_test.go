package webhook

import (
	"context"
	"testing"
	"time"
)

func TestSignatureUsesExactRawBodyAndReplayWindow(t *testing.T) {
	secret := []byte("secret")
	now := time.Unix(1_000, 0).UTC()
	body := []byte(`{"event":"payment.paid","data":{"amount":1000}}`)
	timestamp := "1000"
	signature := "sha256=" + Sign(secret, timestamp, body)
	if !VerifySignature(secret, timestamp, body, signature, now, 5*time.Minute) {
		t.Fatal("valid signature rejected")
	}
	if VerifySignature(secret, timestamp, []byte(`{"data":{"amount":1000},"event":"payment.paid"}`), signature, now, 5*time.Minute) {
		t.Fatal("reformatted payload accepted")
	}
	if VerifySignature([]byte("wrong"), timestamp, body, signature, now, 5*time.Minute) {
		t.Fatal("wrong secret accepted")
	}
	if VerifySignature(secret, "600", body, "sha256="+Sign(secret, "600", body), now, 5*time.Minute) {
		t.Fatal("old timestamp accepted")
	}
}

func TestWebhookBackoffIsBounded(t *testing.T) {
	if Backoff(1) != 30*time.Second || Backoff(6) != 30*time.Minute || Backoff(99) != 30*time.Minute {
		t.Fatalf("unexpected backoff values")
	}
}

func TestValidateEndpointRejectsPrivateTargets(t *testing.T) {
	for _, endpoint := range []string{"http://localhost/hook", "http://127.0.0.1/hook", "https://10.0.0.1/hook", "https://169.254.169.254/latest"} {
		if err := ValidateEndpoint(context.Background(), endpoint, true); err == nil {
			t.Fatalf("private endpoint accepted: %s", endpoint)
		}
	}
	if err := ValidateEndpoint(context.Background(), "http://example.com/hook", false); err == nil {
		t.Fatal("HTTP production endpoint accepted")
	}
}
