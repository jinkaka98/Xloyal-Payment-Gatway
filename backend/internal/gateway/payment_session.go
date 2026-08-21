package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"xloyal/backend/internal/domain"
	"xloyal/backend/internal/security"
	"xloyal/backend/internal/store"
)

type CreatePaymentSessionInput struct {
	TenantID     string
	InvoiceID    string
	ThemeID      string
	ThemeVersion int
	ReturnURL    string
	SuccessURL   string
	CancelURL    string
	FailedURL    string
	ExpiredURL   string
	ExpiresAt    time.Time
}

type CreatePaymentSessionResult struct {
	Session     domain.PaymentSession
	PublicToken string
}

var ErrPaymentSessionStateConflict = errors.New("payment session state conflict")

type PaymentSessionSnapshot struct {
	Session domain.PaymentSession
	Invoice domain.Invoice
	Theme   *domain.PaymentThemeVersion
}

type PaymentSessionService struct {
	Repo store.Repository
	Now  func() time.Time
}

func (s PaymentSessionService) CreatePaymentSession(ctx context.Context, in CreatePaymentSessionInput) (CreatePaymentSessionResult, error) {
	in.TenantID, in.InvoiceID, in.ThemeID = strings.TrimSpace(in.TenantID), strings.TrimSpace(in.InvoiceID), strings.TrimSpace(in.ThemeID)
	in.ReturnURL, in.SuccessURL, in.CancelURL = strings.TrimSpace(in.ReturnURL), strings.TrimSpace(in.SuccessURL), strings.TrimSpace(in.CancelURL)
	in.FailedURL, in.ExpiredURL = strings.TrimSpace(in.FailedURL), strings.TrimSpace(in.ExpiredURL)
	if in.TenantID == "" || in.InvoiceID == "" {
		return CreatePaymentSessionResult{}, errors.New("tenant and invoice are required")
	}
	invoice, err := s.Repo.Invoice(ctx, in.TenantID, in.InvoiceID)
	if err != nil {
		return CreatePaymentSessionResult{}, err
	}
	if invoice.Status != domain.InvoicePending {
		return CreatePaymentSessionResult{}, store.ErrConflict
	}
	if invoice.Amount <= 0 || strings.ToUpper(strings.TrimSpace(invoice.Currency)) != "IDR" {
		return CreatePaymentSessionResult{}, store.ErrConflict
	}
	if in.ExpiresAt.IsZero() || in.ExpiresAt.After(invoice.ExpiresAt) {
		in.ExpiresAt = invoice.ExpiresAt
	}
	now := s.now()
	if !in.ExpiresAt.After(now) {
		return CreatePaymentSessionResult{}, store.ErrConflict
	}
	if in.ThemeID == "" && in.ThemeVersion != 0 {
		return CreatePaymentSessionResult{}, errors.New("theme id and version must be provided together")
	}
	if in.ThemeID != "" {
		version, themeErr := s.Repo.PublishedPaymentThemeVersion(ctx, in.TenantID, in.ThemeID, in.ThemeVersion)
		if themeErr != nil {
			return CreatePaymentSessionResult{}, themeErr
		}
		in.ThemeVersion = version.Version
	} else {
		version, themeErr := s.Repo.DefaultPublishedPaymentThemeVersion(ctx, in.TenantID)
		if themeErr == nil {
			in.ThemeID, in.ThemeVersion = version.ThemeID, version.Version
		} else if !errors.Is(themeErr, store.ErrNotFound) {
			return CreatePaymentSessionResult{}, themeErr
		}
	}
	for kind, url := range map[string]string{"SUCCESS": in.SuccessURL, "RETURN": in.ReturnURL, "CANCEL": in.CancelURL, "FAILED": in.FailedURL, "EXPIRED": in.ExpiredURL} {
		if strings.TrimSpace(url) == "" {
			continue
		}
		if kind == "RETURN" {
			kind = domain.RedirectSuccess
		}
		allowed, allowErr := s.Repo.RedirectURLAllowed(ctx, in.TenantID, url, kind)
		if allowErr != nil {
			return CreatePaymentSessionResult{}, allowErr
		}
		if !allowed {
			return CreatePaymentSessionResult{}, errors.New("redirect URL is not allowed")
		}
	}
	token, err := security.GeneratePublicPaymentToken()
	if err != nil {
		return CreatePaymentSessionResult{}, err
	}
	sessionID, eventID, eventRowID, outboxID := id(), id(), id(), id()
	session := domain.PaymentSession{ID: sessionID, TenantID: in.TenantID, InvoiceID: in.InvoiceID, PublicTokenHash: security.HashPublicPaymentToken(token), Status: domain.PaymentSessionOpen, ThemeID: in.ThemeID, ThemeVersion: in.ThemeVersion, ReturnURL: in.ReturnURL, SuccessURL: in.SuccessURL, CancelURL: in.CancelURL, FailedURL: in.FailedURL, ExpiredURL: in.ExpiredURL, ExpiresAt: in.ExpiresAt, CreatedAt: now, UpdatedAt: now}
	payload, _ := json.Marshal(map[string]any{"payment_session_id": sessionID, "invoice_id": in.InvoiceID, "status": session.Status})
	event := domain.PaymentEvent{ID: eventRowID, EventID: eventID, TenantID: in.TenantID, InvoiceID: in.InvoiceID, PaymentSessionID: sessionID, SequenceNumber: 1, EventType: "payment.created", Payload: payload, OccurredAt: now, CreatedAt: now}
	outbox := domain.OutboxEvent{ID: outboxID, EventID: eventID, TenantID: in.TenantID, EventType: event.EventType, AggregateType: "payment_session", AggregateID: sessionID, Payload: payload, Status: "PENDING", NextAttemptAt: now, CreatedAt: now}
	if err = s.Repo.CreatePaymentSession(ctx, session, event, outbox); err != nil {
		return CreatePaymentSessionResult{}, err
	}
	return CreatePaymentSessionResult{Session: session, PublicToken: token}, nil
}

func (s PaymentSessionService) LookupPaymentSessionByTokenHash(ctx context.Context, hash string) (domain.PaymentSession, error) {
	if strings.TrimSpace(hash) == "" {
		return domain.PaymentSession{}, store.ErrNotFound
	}
	return s.Repo.PaymentSessionByTokenHash(ctx, hash)
}

func (s PaymentSessionService) LookupPaymentSessionByPublicToken(ctx context.Context, token string) (domain.PaymentSession, error) {
	if strings.TrimSpace(token) == "" {
		return domain.PaymentSession{}, store.ErrNotFound
	}
	return s.LookupPaymentSessionByTokenHash(ctx, security.HashPublicPaymentToken(token))
}

func (s PaymentSessionService) LookupPaymentSessionByToken(ctx context.Context, token string) (domain.PaymentSession, error) {
	return s.LookupPaymentSessionByPublicToken(ctx, token)
}

// Snapshot resolves externally committed invoice state and request-time expiry
// before returning the public-safe session aggregate.
func (s PaymentSessionService) Snapshot(ctx context.Context, token string) (PaymentSessionSnapshot, error) {
	session, err := s.LookupPaymentSessionByPublicToken(ctx, token)
	if err != nil {
		return PaymentSessionSnapshot{}, err
	}
	now := s.now()
	for attempt := 0; attempt < 4; attempt++ {
		invoice, invoiceErr := s.Repo.Invoice(ctx, session.TenantID, session.InvoiceID)
		if invoiceErr != nil {
			return PaymentSessionSnapshot{}, invoiceErr
		}
		if session.Status == domain.PaymentSessionOpen {
			if _, transitionErr := s.transition(ctx, session, domain.PaymentSessionPaymentPending, now); transitionErr != nil && !errors.Is(transitionErr, store.ErrConflict) {
				return PaymentSessionSnapshot{}, transitionErr
			}
			session, err = s.Repo.PaymentSession(ctx, session.TenantID, session.ID)
			if err != nil {
				return PaymentSessionSnapshot{}, err
			}
			continue
		}
		if session.Status == domain.PaymentSessionPaymentPending {
			if terminal, ok := paymentSessionStatusForInvoice(invoice.Status); ok {
				if _, transitionErr := s.transition(ctx, session, terminal, now); transitionErr != nil && !errors.Is(transitionErr, store.ErrConflict) {
					return PaymentSessionSnapshot{}, transitionErr
				}
				session, err = s.Repo.PaymentSession(ctx, session.TenantID, session.ID)
				if err != nil {
					return PaymentSessionSnapshot{}, err
				}
				continue
			}
			if !now.Before(session.ExpiresAt) {
				if _, transitionErr := s.transition(ctx, session, domain.PaymentSessionExpired, now); transitionErr != nil && !errors.Is(transitionErr, store.ErrConflict) {
					return PaymentSessionSnapshot{}, transitionErr
				}
				session, err = s.Repo.PaymentSession(ctx, session.TenantID, session.ID)
				if err != nil {
					return PaymentSessionSnapshot{}, err
				}
				continue
			}
		}
		// Checkout presentation is intentionally live: a default-theme update
		// applies when any existing public checkout URL is loaded again. The
		// payment session still owns all financial and lifecycle fields.
		var theme *domain.PaymentThemeVersion
		resolved, themeErr := s.Repo.DefaultPublishedPaymentThemeVersion(ctx, session.TenantID)
		if themeErr == nil {
			theme = &resolved
		} else if !errors.Is(themeErr, store.ErrNotFound) {
			return PaymentSessionSnapshot{}, themeErr
		} else if session.ThemeID != "" {
			resolved, themeErr = s.Repo.PaymentThemeVersion(ctx, session.ThemeID, session.ThemeVersion)
			if themeErr != nil {
				return PaymentSessionSnapshot{}, themeErr
			}
			theme = &resolved
		}
		return PaymentSessionSnapshot{Session: session, Invoice: invoice, Theme: theme}, nil
	}
	return PaymentSessionSnapshot{}, ErrPaymentSessionStateConflict
}

func (s PaymentSessionService) Cancel(ctx context.Context, token string) (PaymentSessionSnapshot, error) {
	snapshot, err := s.Snapshot(ctx, token)
	if err != nil {
		return PaymentSessionSnapshot{}, err
	}
	if snapshot.Session.Status == domain.PaymentSessionCancelled {
		return snapshot, nil
	}
	if snapshot.Session.Status != domain.PaymentSessionPaymentPending {
		return snapshot, ErrPaymentSessionStateConflict
	}
	now := s.now()
	// Cancel the invoice with a conditional pending-only update first. This
	// prevents a worker that already won the pending->paid race from being
	// overwritten by a checkout cancel. A concurrent cancel may already have
	// committed the same terminal state; that case is idempotent.
	if snapshot.Invoice.Status == domain.InvoicePending {
		invoice := snapshot.Invoice
		if err := invoice.Transition(domain.InvoiceCancelled, now); err != nil {
			return PaymentSessionSnapshot{}, err
		}
		changed, cancelErr := s.Repo.CancelPendingInvoice(ctx, invoice)
		if cancelErr != nil {
			return PaymentSessionSnapshot{}, cancelErr
		}
		if !changed {
			latest, latestErr := s.Snapshot(ctx, token)
			if latestErr != nil {
				return PaymentSessionSnapshot{}, latestErr
			}
			if latest.Session.Status == domain.PaymentSessionCancelled {
				return latest, nil
			}
			return latest, ErrPaymentSessionStateConflict
		}
	} else if snapshot.Invoice.Status != domain.InvoiceCancelled {
		latest, latestErr := s.Snapshot(ctx, token)
		if latestErr != nil {
			return PaymentSessionSnapshot{}, latestErr
		}
		return latest, ErrPaymentSessionStateConflict
	}
	if _, transitionErr := s.transition(ctx, snapshot.Session, domain.PaymentSessionCancelled, now); transitionErr != nil {
		if !errors.Is(transitionErr, store.ErrConflict) {
			return PaymentSessionSnapshot{}, transitionErr
		}
	}
	latest, snapshotErr := s.Snapshot(ctx, token)
	if snapshotErr != nil {
		return PaymentSessionSnapshot{}, snapshotErr
	}
	// A concurrent terminal transition may have won after the initial snapshot.
	// Do not turn that race into a successful cancel response; the latest
	// persisted payment state remains the source of truth for the client.
	if latest.Session.Status != domain.PaymentSessionCancelled {
		return latest, ErrPaymentSessionStateConflict
	}
	return latest, nil
}

func (s PaymentSessionService) transition(ctx context.Context, current domain.PaymentSession, next domain.PaymentSessionStatus, now time.Time) (domain.PaymentSession, error) {
	eventType, ok := current.Status.PaymentEventTypeForTransition(next)
	if !ok {
		return current, ErrPaymentSessionStateConflict
	}
	payload, _ := json.Marshal(map[string]any{"payment_session_id": current.ID, "invoice_id": current.InvoiceID, "status": next})
	eventID, rowID, outboxID := id(), id(), id()
	event := domain.PaymentEvent{ID: rowID, EventID: eventID, TenantID: current.TenantID, InvoiceID: current.InvoiceID, PaymentSessionID: current.ID, EventType: eventType, Payload: payload, OccurredAt: now, CreatedAt: now}
	outbox := domain.OutboxEvent{ID: outboxID, EventID: eventID, TenantID: current.TenantID, EventType: eventType, AggregateType: "payment_session", AggregateID: current.ID, Payload: payload, Status: domain.OutboxPending, NextAttemptAt: now, CreatedAt: now}
	return s.Repo.TransitionPaymentSession(ctx, current.TenantID, current.ID, next, now, event, outbox)
}

func paymentSessionStatusForInvoice(status domain.InvoiceStatus) (domain.PaymentSessionStatus, bool) {
	switch status {
	case domain.InvoicePaid:
		return domain.PaymentSessionPaid, true
	case domain.InvoiceExpired:
		return domain.PaymentSessionExpired, true
	case domain.InvoiceFailed:
		return domain.PaymentSessionFailed, true
	default:
		return "", false
	}
}

func (s PaymentSessionService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
