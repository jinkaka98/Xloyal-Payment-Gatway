package domain

import (
	"errors"
	"testing"
	"time"
)

func TestInvoiceTransitionsOnlyFromPending(t *testing.T) {
	for _, next := range []InvoiceStatus{InvoicePaid, InvoiceExpired, InvoiceFailed} {
		i := Invoice{Status: InvoicePending}
		if err := i.Transition(next, time.Now()); err != nil || i.Status != next {
			t.Fatalf("transition to %s: %v", next, err)
		}
		if err := i.Transition(InvoicePaid, time.Now()); !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("terminal state changed: %v", err)
		}
	}
	if err := (&Invoice{Status: InvoicePending}).Transition(InvoicePending, time.Now()); !errors.Is(err, ErrInvalidTransition) {
		t.Fatal("pending->pending accepted")
	}
}
