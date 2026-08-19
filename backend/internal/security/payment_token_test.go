package security

import "testing"

func TestPublicPaymentTokenIsOpaqueAndRandom(t *testing.T) {
	a, err := GeneratePublicPaymentToken()
	if err != nil {
		t.Fatal(err)
	}
	b, err := GeneratePublicPaymentToken()
	if err != nil {
		t.Fatal(err)
	}
	if a == b || len(a) < 40 {
		t.Fatalf("tokens must be high entropy and unique")
	}
	if HashPublicPaymentToken(a) == a {
		t.Fatal("stored token hash must differ from plaintext")
	}
	if HashPublicPaymentToken(a) != HashPublicPaymentToken(a) {
		t.Fatal("token hashing must be deterministic")
	}
}
