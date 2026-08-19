package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"xloyal/backend/internal/domain"
	"xloyal/backend/internal/store"
)

// PaymentSSEEvent is the public, deliberately reduced representation of a
// persisted payment event. Database rows and internal payloads are never
// serialized directly to a public stream.
type PaymentSSEEvent struct {
	EventID          string    `json:"event_id"`
	PaymentSessionID string    `json:"payment_session_id"`
	InvoiceID        string    `json:"invoice_id"`
	Status           string    `json:"status"`
	Sequence         int64     `json:"sequence"`
	OccurredAt       time.Time `json:"occurred_at"`
}

type paymentSSESubscriber struct {
	ch chan PaymentSSEEvent
}

// PaymentSSEHub is transport-only state. Persisted events remain authoritative
// and are replayed by the endpoint after reconnect or process restart.
type PaymentSSEHub struct {
	mu          sync.Mutex
	subscribers map[string]map[*paymentSSESubscriber]struct{}
	BufferSize  int
}

func (h *PaymentSSEHub) ensure() {
	if h.subscribers == nil {
		h.subscribers = make(map[string]map[*paymentSSESubscriber]struct{})
	}
	if h.BufferSize <= 0 {
		h.BufferSize = 16
	}
}

func (h *PaymentSSEHub) subscribe(sessionID string) *paymentSSESubscriber {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ensure()
	s := &paymentSSESubscriber{ch: make(chan PaymentSSEEvent, h.BufferSize)}
	if h.subscribers[sessionID] == nil {
		h.subscribers[sessionID] = make(map[*paymentSSESubscriber]struct{})
	}
	h.subscribers[sessionID][s] = struct{}{}
	return s
}

func (h *PaymentSSEHub) unsubscribe(sessionID string, s *paymentSSESubscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if group := h.subscribers[sessionID]; group != nil {
		delete(group, s)
		if len(group) == 0 {
			delete(h.subscribers, sessionID)
		}
	}
}

// Publish never blocks the dispatcher. A full subscriber is removed; the
// reconnecting client replays the missing persisted sequence.
func (h *PaymentSSEHub) Publish(sessionID string, event PaymentSSEEvent) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	count := 0
	for sub := range h.subscribers[sessionID] {
		select {
		case sub.ch <- event:
			count++
		default:
			delete(h.subscribers[sessionID], sub)
			close(sub.ch)
		}
	}
	return count
}

func (s Server) paymentSSEHub() *PaymentSSEHub {
	if s.SSE != nil {
		return s.SSE
	}
	return &PaymentSSEHub{BufferSize: 16}
}

func paymentSSEStatus(eventType string) string {
	switch eventType {
	case domain.PaymentEventPending:
		return "payment_pending"
	case domain.PaymentEventVerifying:
		return "verifying"
	case domain.PaymentEventPaid:
		return "paid"
	case domain.PaymentEventFailed:
		return "failed"
	case domain.PaymentEventExpired:
		return "expired"
	case domain.PaymentEventCancelled:
		return "cancelled"
	case domain.PaymentEventRedirecting:
		return "redirecting"
	case domain.PaymentEventClosed:
		return "closed"
	default:
		return "payment_pending"
	}
}

func publicPaymentSSEEvent(event domain.PaymentEvent) PaymentSSEEvent {
	return PaymentSSEEvent{EventID: event.EventID, PaymentSessionID: event.PaymentSessionID, InvoiceID: event.InvoiceID, Status: paymentSSEStatus(event.EventType), Sequence: event.SequenceNumber, OccurredAt: event.OccurredAt}
}

func (s Server) paymentSessionEvents(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.PathValue("token"))
	if token == "" || len(token) > 512 {
		paymentSessionProblem(w, http.StatusNotFound, "not_found", "payment session not found")
		return
	}
	after := int64(0)
	if raw := r.URL.Query().Get("after_sequence"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			paymentSessionProblem(w, http.StatusBadRequest, "invalid_cursor", "after_sequence must be a non-negative integer")
			return
		}
		after = parsed
	}
	service := s.paymentSessionService()
	snapshot, err := service.Snapshot(r.Context(), token)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			paymentSessionProblem(w, http.StatusNotFound, "not_found", "payment session not found")
		} else {
			paymentSessionProblem(w, http.StatusConflict, "conflict", "payment session state could not be resolved")
		}
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		paymentSessionProblem(w, http.StatusInternalServerError, "stream_unavailable", "SSE is not supported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	hub := s.paymentSSEHub()
	subscriber := hub.subscribe(snapshot.Session.ID)
	defer hub.unsubscribe(snapshot.Session.ID, subscriber)
	ctx := r.Context()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	writeEvent := func(event domain.PaymentEvent) error {
		if event.SequenceNumber <= after {
			return nil
		}
		public := publicPaymentSSEEvent(event)
		data, err := json.Marshal(public)
		if err != nil {
			return err
		}
		if _, err = fmt.Fprintf(w, "event: %s\nid: %d\ndata: %s\n\n", event.EventType, event.SequenceNumber, data); err != nil {
			return err
		}
		after = event.SequenceNumber
		flusher.Flush()
		return nil
	}

	// Snapshot-first is followed by a persisted replay, closing the race where
	// the invoice transitions after the snapshot but before registration.
	if events, eventErr := s.Repo.PaymentEvents(ctx, snapshot.Session.TenantID, snapshot.Session.ID); eventErr == nil {
		for _, event := range events {
			if err := writeEvent(event); err != nil {
				return
			}
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case event, open := <-subscriber.ch:
			if !open {
				return
			}
			if event.Sequence > after {
				data, marshalErr := json.Marshal(event)
				if marshalErr != nil {
					return
				}
				if _, writeErr := fmt.Fprintf(w, "event: %s\nid: %d\ndata: %s\n\n", eventTypeForStatus(event.Status), event.Sequence, data); writeErr != nil {
					return
				}
				after = event.Sequence
				flusher.Flush()
			}
		case <-ticker.C:
			events, eventErr := s.Repo.PaymentEvents(ctx, snapshot.Session.TenantID, snapshot.Session.ID)
			if eventErr != nil {
				continue
			}
			for _, event := range events {
				if err := writeEvent(event); err != nil {
					return
				}
			}
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func eventTypeForStatus(status string) string {
	switch status {
	case "payment_pending":
		return domain.PaymentEventPending
	case "verifying":
		return domain.PaymentEventVerifying
	case "paid":
		return domain.PaymentEventPaid
	case "failed":
		return domain.PaymentEventFailed
	case "expired":
		return domain.PaymentEventExpired
	case "cancelled":
		return domain.PaymentEventCancelled
	case "redirecting":
		return domain.PaymentEventRedirecting
	case "closed":
		return domain.PaymentEventClosed
	default:
		return domain.PaymentEventPending
	}
}
