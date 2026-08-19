package domain

import (
	"testing"
	"time"
)

func TestPaymentSessionTransitions(t *testing.T) {
	now := time.Now()
	valid := []struct{ from, to PaymentSessionStatus }{
		{PaymentSessionOpen, PaymentSessionPaymentPending},
		{PaymentSessionPaymentPending, PaymentSessionPaid},
		{PaymentSessionPaymentPending, PaymentSessionCancelled},
		{PaymentSessionPaymentPending, PaymentSessionExpired},
		{PaymentSessionPaymentPending, PaymentSessionFailed},
		{PaymentSessionPaid, PaymentSessionRedirecting},
		{PaymentSessionRedirecting, PaymentSessionClosed},
	}
	for _, tc := range valid {
		s := PaymentSession{Status: tc.from}
		if err := s.Transition(tc.to, now); err != nil {
			t.Fatalf("%s -> %s: %v", tc.from, tc.to, err)
		}
	}
	invalid := []struct{ from, to PaymentSessionStatus }{
		{PaymentSessionPaid, PaymentSessionPaymentPending}, {PaymentSessionPaid, PaymentSessionCancelled},
		{PaymentSessionCancelled, PaymentSessionPaid}, {PaymentSessionExpired, PaymentSessionPaymentPending}, {PaymentSessionFailed, PaymentSessionPaid},
	}
	for _, tc := range invalid {
		s := PaymentSession{Status: tc.from}
		if err := s.Transition(tc.to, now); err == nil {
			t.Fatalf("expected %s -> %s to fail", tc.from, tc.to)
		}
	}
}
