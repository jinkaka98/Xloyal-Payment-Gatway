package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"xloyal/backend/internal/domain"
)

func TestOutboxClaimIsExclusiveAndLeaseRecovers(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	repo := NewMemory()
	repo.invoices["inv"] = domain.Invoice{ID: "inv", TenantID: "tenant", Status: domain.InvoicePending, ExpiresAt: now.Add(time.Hour)}
	session := domain.PaymentSession{ID: "session", TenantID: "tenant", InvoiceID: "inv", Status: domain.PaymentSessionOpen, ExpiresAt: now.Add(time.Hour)}
	event := domain.PaymentEvent{ID: "row", EventID: "event", TenantID: "tenant", InvoiceID: "inv", PaymentSessionID: "session", EventType: "payment.created"}
	outbox := domain.OutboxEvent{ID: "outbox", EventID: "event", TenantID: "tenant", EventType: "payment.created", AggregateType: "payment_session", AggregateID: "session", Status: "PENDING", NextAttemptAt: now, CreatedAt: now}
	if err := repo.CreatePaymentSession(ctx, session, event, outbox); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	counts := make(chan int, 2)
	for _, owner := range []string{"worker-a", "worker-b"} {
		go func(owner string) {
			defer wg.Done()
			got, err := repo.ClaimOutboxEvents(ctx, owner, now, time.Minute, 10)
			if err != nil {
				t.Error(err)
			}
			counts <- len(got)
		}(owner)
	}
	wg.Wait()
	close(counts)
	total := 0
	for n := range counts {
		total += n
	}
	if total != 1 {
		t.Fatalf("one outbox row must be claimed exactly once, got %d", total)
	}
	recovered, err := repo.ClaimOutboxEvents(ctx, "worker-c", now.Add(2*time.Minute), time.Minute, 10)
	if err != nil || len(recovered) != 1 || recovered[0].LockedBy != "worker-c" {
		t.Fatalf("expired lease was not recovered: %#v %v", recovered, err)
	}
}

func TestPaymentSessionRepositoryRevalidatesThemeAndRedirect(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	repo := NewMemory()
	repo.invoices["inv"] = domain.Invoice{ID: "inv", TenantID: "tenant", Status: domain.InvoicePending, ExpiresAt: now.Add(time.Hour)}
	repo.PutPaymentThemeVersion(
		domain.PaymentTheme{ID: "theme", TenantID: "tenant", Status: domain.ThemePublished},
		domain.PaymentThemeVersion{ID: "theme-v1", ThemeID: "theme", Version: 1, Status: domain.ThemePublished},
	)
	session := domain.PaymentSession{ID: "session", TenantID: "tenant", InvoiceID: "inv", PublicTokenHash: "hash", Status: domain.PaymentSessionOpen, ThemeID: "theme", ThemeVersion: 1, SuccessURL: "https://client.example/success", ExpiresAt: now.Add(time.Hour)}
	event := domain.PaymentEvent{ID: "event-row", EventID: "event", EventType: domain.PaymentEventCreated}
	outbox := domain.OutboxEvent{ID: "outbox", NextAttemptAt: now}
	if err := repo.CreatePaymentSession(ctx, session, event, outbox); !errors.Is(err, ErrConflict) {
		t.Fatalf("unregistered redirect must be rejected in repository transaction, got %v", err)
	}
	repo.PutAllowedRedirectURL(domain.AllowedRedirectURL{ID: "redirect", TenantID: "tenant", URL: session.SuccessURL, Type: domain.RedirectSuccess, Active: true})
	if err := repo.CreatePaymentSession(ctx, session, event, outbox); err != nil {
		t.Fatalf("valid theme and redirect were rejected: %v", err)
	}

	if err := repo.CreatePaymentEvent(ctx, domain.PaymentEvent{ID: "wrong-tenant-row", EventID: "wrong-tenant", TenantID: "other", PaymentSessionID: session.ID}); !errors.Is(err, ErrConflict) {
		t.Fatalf("cross-tenant event must be rejected, got %v", err)
	}
	if err := repo.CreatePaymentEvent(ctx, domain.PaymentEvent{ID: "next-row", EventID: "next", EventType: domain.PaymentEventPending, PaymentSessionID: session.ID}); err != nil {
		t.Fatalf("valid event append failed: %v", err)
	}
	events, err := repo.PaymentEvents(ctx, "tenant", session.ID)
	if err != nil || len(events) != 2 || events[0].SequenceNumber != 1 || events[1].SequenceNumber != 2 {
		t.Fatalf("event sequence is not monotonic: %#v %v", events, err)
	}
	if err := repo.CreateOutboxEvent(ctx, domain.OutboxEvent{ID: "wrong-outbox", EventID: "next", TenantID: "other"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("cross-tenant outbox must be rejected, got %v", err)
	}
}

func TestFailedOutboxEventIsNotClaimedAgain(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	repo := NewMemory()
	repo.outboxEvents["outbox"] = domain.OutboxEvent{ID: "outbox", Status: domain.OutboxPending, NextAttemptAt: now}
	claimed, err := repo.ClaimOutboxEvents(ctx, "worker", now, time.Minute, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("initial claim failed: %#v %v", claimed, err)
	}
	if err = repo.MarkOutboxFailed(ctx, "outbox", "worker", now, "permanent"); err != nil {
		t.Fatal(err)
	}
	claimed, err = repo.ClaimOutboxEvents(ctx, "other-worker", now.Add(time.Hour), time.Minute, 1)
	if err != nil || len(claimed) != 0 {
		t.Fatalf("permanently failed event was claimed again: %#v %v", claimed, err)
	}
}

func TestPaymentSessionTransitionPersistsEventAndOutboxAtomically(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	repo := NewMemory()
	repo.invoices["inv"] = domain.Invoice{ID: "inv", TenantID: "tenant", Status: domain.InvoicePending, ExpiresAt: now.Add(time.Hour)}
	session := domain.PaymentSession{ID: "session", TenantID: "tenant", InvoiceID: "inv", Status: domain.PaymentSessionOpen, ExpiresAt: now.Add(time.Hour)}
	created := domain.PaymentEvent{ID: "created-row", EventID: "created", TenantID: "tenant", InvoiceID: "inv", PaymentSessionID: "session", EventType: "payment.created"}
	if err := repo.CreatePaymentSession(ctx, session, created, domain.OutboxEvent{ID: "created-outbox", EventID: "created", Status: "PENDING"}); err != nil {
		t.Fatal(err)
	}
	pending := domain.PaymentEvent{ID: "pending-row", EventID: "pending", EventType: "payment.pending"}
	updated, err := repo.TransitionPaymentSession(ctx, "tenant", "session", domain.PaymentSessionPaymentPending, now, pending, domain.OutboxEvent{ID: "pending-outbox", EventID: "pending", Status: "PENDING"})
	if err != nil || updated.Status != domain.PaymentSessionPaymentPending {
		t.Fatalf("transition failed: %#v %v", updated, err)
	}
	if len(repo.paymentEvents) != 2 || len(repo.outboxEvents) != 2 {
		t.Fatalf("event and outbox were not inserted with transition")
	}
	if _, err = repo.TransitionPaymentSession(ctx, "tenant", "session", domain.PaymentSessionClosed, now, pending, domain.OutboxEvent{}); err == nil {
		t.Fatal("invalid transition accepted")
	}
	if len(repo.paymentEvents) != 2 || len(repo.outboxEvents) != 2 {
		t.Fatal("invalid transition partially wrote event/outbox")
	}
}

func TestTerminalPaymentSessionTransitionUpdatesInvoiceAtomically(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	repo := NewMemory()
	repo.invoices["inv"] = domain.Invoice{ID: "inv", TenantID: "tenant", Status: domain.InvoicePending, ExpiresAt: now.Add(time.Hour)}
	repo.paymentSessions["session"] = domain.PaymentSession{ID: "session", TenantID: "tenant", InvoiceID: "inv", Status: domain.PaymentSessionPaymentPending}
	_, err := repo.TransitionPaymentSession(ctx, "tenant", "session", domain.PaymentSessionPaid, now, domain.PaymentEvent{ID: "paid-row", EventID: "paid-event", PaymentSessionID: "session", EventType: domain.PaymentEventPaid}, domain.OutboxEvent{ID: "paid-outbox", EventID: "paid-event"})
	if err != nil {
		t.Fatal(err)
	}
	if repo.invoices["inv"].Status != domain.InvoicePaid {
		t.Fatal("invoice and session terminal state did not commit together")
	}
	repo.invoices["inv2"] = domain.Invoice{ID: "inv2", TenantID: "tenant", Status: domain.InvoiceExpired, ExpiresAt: now.Add(time.Hour)}
	repo.paymentSessions["session2"] = domain.PaymentSession{ID: "session2", TenantID: "tenant", InvoiceID: "inv2", Status: domain.PaymentSessionPaymentPending}
	beforeEvents, beforeOutbox := len(repo.paymentEvents), len(repo.outboxEvents)
	if _, err = repo.TransitionPaymentSession(ctx, "tenant", "session2", domain.PaymentSessionPaid, now, domain.PaymentEvent{ID: "bad-row", EventID: "bad-event", EventType: domain.PaymentEventPaid}, domain.OutboxEvent{ID: "bad-outbox"}); err == nil {
		t.Fatal("expected invoice state conflict")
	}
	if repo.paymentSessions["session2"].Status != domain.PaymentSessionPaymentPending || len(repo.paymentEvents) != beforeEvents || len(repo.outboxEvents) != beforeOutbox {
		t.Fatal("failed transition partially committed")
	}
}
