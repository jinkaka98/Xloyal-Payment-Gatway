package httpapi

import (
	"testing"
	"time"

	"xloyal/backend/internal/domain"
)

func TestPublicPaymentSSEEventDoesNotExposePersistedPayload(t *testing.T) {
	event := domain.PaymentEvent{EventID: "event-1", PaymentSessionID: "session-1", InvoiceID: "invoice-1", EventType: domain.PaymentEventPaid, SequenceNumber: 3, Payload: []byte(`{"api_key":"secret","status":"paid"}`), OccurredAt: time.Unix(10, 0).UTC()}
	public := publicPaymentSSEEvent(event)
	if public.Status != "paid" || public.Sequence != 3 || public.EventID != "event-1" {
		t.Fatalf("unexpected public event: %#v", public)
	}
	if publicPaymentSSEEvent(event).PaymentSessionID != event.PaymentSessionID {
		t.Fatal("session identity was not preserved")
	}
}

func TestPaymentSSEHubDisconnectsSlowSubscriber(t *testing.T) {
	hub := &PaymentSSEHub{BufferSize: 1}
	subscriber := hub.subscribe("session-1")
	event := PaymentSSEEvent{EventID: "event-1", PaymentSessionID: "session-1", Sequence: 1}
	if got := hub.Publish("session-1", event); got != 1 {
		t.Fatalf("first publish subscribers=%d", got)
	}
	if got := hub.Publish("session-1", event); got != 0 {
		t.Fatalf("slow subscriber should be removed, subscribers=%d", got)
	}
	select {
	case _, open := <-subscriber.ch:
		if !open {
			return
		}
		if _, open = <-subscriber.ch; open {
			t.Fatal("slow subscriber channel remained open")
		}
	default:
		t.Fatal("subscriber channel was not closed")
	}
}
