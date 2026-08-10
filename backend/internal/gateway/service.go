package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"xloyal/backend/internal/domain"
	"xloyal/backend/internal/store"
)

type ProviderResolver func(context.Context, domain.MerchantAccount) (domain.PaymentProvider, error)
type Service struct {
	Repo     store.Repository
	Provider ProviderResolver
	Now      func() time.Time
}
type CreateInvoiceInput struct {
	TenantID, MerchantAccountID, IdempotencyKey, Currency, Description string
	Amount                                                             int64
}

var ErrIdempotencyConflict = errors.New("idempotency key reused with different request")
var ErrCheckCooldown = errors.New("invoice status was checked recently")

const (
	maxInvoiceAmount = int64(100_000_000)
	checkCooldown    = time.Minute
)

func (s Service) CreateInvoice(ctx context.Context, in CreateInvoiceInput) (domain.Invoice, bool, error) {
	in.TenantID = strings.TrimSpace(in.TenantID)
	in.MerchantAccountID = strings.TrimSpace(in.MerchantAccountID)
	in.IdempotencyKey = strings.TrimSpace(in.IdempotencyKey)
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	in.Description = strings.TrimSpace(in.Description)
	if in.TenantID == "" || in.MerchantAccountID == "" || in.IdempotencyKey == "" || in.Amount <= 0 {
		return domain.Invoice{}, false, errors.New("tenant, merchant account, idempotency key and positive amount are required")
	}
	if in.Currency == "" {
		in.Currency = "IDR"
	}
	if in.Currency != "IDR" {
		return domain.Invoice{}, false, errors.New("currency must be IDR")
	}
	if in.Amount > maxInvoiceAmount {
		return domain.Invoice{}, false, fmt.Errorf("amount must not exceed %d", maxInvoiceAmount)
	}
	if len(in.IdempotencyKey) > 128 || len(in.Description) > 200 {
		return domain.Invoice{}, false, errors.New("idempotency key or description is too long")
	}
	merchant, err := s.Repo.MerchantAccount(ctx, in.TenantID, in.MerchantAccountID)
	if err != nil {
		return domain.Invoice{}, false, err
	}
	if !merchant.Active {
		return domain.Invoice{}, false, errors.New("merchant account inactive")
	}
	now := s.now()
	inv := domain.Invoice{ID: id(), TenantID: in.TenantID, MerchantAccountID: in.MerchantAccountID, IdempotencyKey: in.IdempotencyKey, Amount: in.Amount, Currency: in.Currency, Description: in.Description, Status: domain.InvoiceCreating, CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(30 * time.Minute)}
	inv, created, err := s.Repo.CreateInvoice(ctx, inv)
	if err != nil {
		return inv, created, err
	}
	if !created {
		if inv.MerchantAccountID != in.MerchantAccountID || inv.Amount != in.Amount || inv.Currency != in.Currency || inv.Description != in.Description {
			return inv, false, ErrIdempotencyConflict
		}
		return inv, false, nil
	}
	p, err := s.Provider(ctx, merchant)
	if err != nil {
		return domain.Invoice{}, true, s.failCreate(ctx, inv, err)
	}
	result, err := p.CreatePayment(ctx, domain.CreatePaymentRequest{InvoiceID: inv.ID, Amount: inv.Amount, Currency: inv.Currency, Description: inv.Description, ExpiresAt: inv.ExpiresAt})
	if err != nil {
		return domain.Invoice{}, true, s.failCreate(ctx, inv, err)
	}
	inv.ProviderReference, inv.QRPayload, inv.ProviderRequestDate = result.ProviderReference, result.QRPayload, result.ProviderRequestDate
	if err = inv.Transition(domain.InvoicePending, s.now()); err != nil {
		return domain.Invoice{}, true, err
	}
	if err = s.Repo.UpdateInvoice(ctx, inv); err != nil {
		return domain.Invoice{}, true, err
	}
	s.audit(ctx, inv.TenantID, "api_key", "invoice.created", "invoice", inv.ID)
	return inv, true, nil
}
func (s Service) failCreate(ctx context.Context, inv domain.Invoice, cause error) error {
	_ = inv.Transition(domain.InvoiceFailed, s.now())
	_ = s.Repo.UpdateInvoice(ctx, inv)
	return fmt.Errorf("create provider payment: %w", cause)
}
func (s Service) Invoice(ctx context.Context, tenant, id string) (domain.Invoice, error) {
	return s.Repo.Invoice(ctx, tenant, id)
}
func (s Service) Check(ctx context.Context, tenant, id string) (domain.Invoice, error) {
	inv, err := s.Repo.Invoice(ctx, tenant, id)
	if err != nil {
		return inv, err
	}
	if inv.Status != domain.InvoicePending {
		return inv, nil
	}
	now := s.now()
	if inv.LastCheckedAt != nil && now.Sub(*inv.LastCheckedAt) < checkCooldown {
		return inv, ErrCheckCooldown
	}
	merchant, err := s.Repo.MerchantAccount(ctx, inv.TenantID, inv.MerchantAccountID)
	if err != nil {
		return inv, err
	}
	p, err := s.Provider(ctx, merchant)
	if err != nil {
		return inv, err
	}
	inv.LastCheckedAt = &now
	inv.CheckCount++
	result, err := p.CheckPayment(ctx, domain.CheckPaymentRequest{ProviderInvoiceID: inv.ProviderReference, Amount: inv.Amount, RequestDate: inv.ProviderRequestDate})
	if err != nil {
		updated, updateErr := s.Repo.UpdatePendingInvoice(ctx, inv)
		if updateErr != nil {
			return inv, updateErr
		}
		if !updated {
			return s.Repo.Invoice(ctx, tenant, id)
		}
		return inv, err
	}
	if result.Status != domain.InvoicePending {
		if err := inv.Transition(result.Status, now); err != nil {
			return inv, err
		}
	}
	updated, err := s.Repo.UpdatePendingInvoice(ctx, inv)
	if err != nil {
		return inv, err
	}
	if !updated {
		return s.Repo.Invoice(ctx, tenant, id)
	}
	if inv.Status != domain.InvoicePending {
		s.audit(ctx, inv.TenantID, "worker", "invoice."+string(inv.Status), "invoice", inv.ID)
	}
	return inv, nil
}
func (s Service) Expire(ctx context.Context, inv domain.Invoice) error {
	if inv.Status != domain.InvoicePending {
		return nil
	}
	if err := inv.Transition(domain.InvoiceExpired, s.now()); err != nil {
		return err
	}
	_, err := s.Repo.UpdatePendingInvoice(ctx, inv)
	return err
}
func (s Service) audit(ctx context.Context, tenant, actor, action, kind, rid string) {
	_ = s.Repo.AppendAudit(ctx, domain.AuditEvent{ID: id(), TenantID: tenant, Actor: actor, Action: action, ResourceType: kind, ResourceID: rid, CreatedAt: s.now()})
}
func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func id() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
