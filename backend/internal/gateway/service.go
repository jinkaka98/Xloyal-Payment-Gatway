package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"xloyal/backend/internal/domain"
	"xloyal/backend/internal/qris"
	"xloyal/backend/internal/store"
)

type ProviderResolver func(context.Context, domain.MerchantAccount) (domain.PaymentProvider, error)
type Service struct {
	Repo                  store.Repository
	Provider              ProviderResolver
	Now                   func() time.Time
	UniqueAmountCodeOrder func() ([]int64, error)
}
type CreateInvoiceInput struct {
	TenantID, MerchantAccountID, IdempotencyKey, Currency, Description string
	Amount                                                             int64
	SandboxMode                                                        bool
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
	if in.TenantID == "" || in.IdempotencyKey == "" || in.Amount <= 0 {
		return domain.Invoice{}, false, errors.New("tenant, idempotency key and positive amount are required")
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
	// Sandbox hosted invoices need tenant QRIS configuration. Keep the
	// production provider path compatible with older service callers that only
	// persisted a merchant account; the authenticated HTTP handler still
	// resolves and validates the tenant before calling this service.
	tenant, tenantErr := s.Repo.Tenant(ctx, in.TenantID)
	if tenantErr != nil {
		if in.SandboxMode || !errors.Is(tenantErr, store.ErrNotFound) {
			return domain.Invoice{}, false, tenantErr
		}
		tenant = domain.Tenant{ID: in.TenantID}
	}
	if in.SandboxMode && tenant.UseUniqueAmountCode && in.Amount > maxInvoiceAmount-99 {
		return domain.Invoice{}, false, fmt.Errorf("amount must not exceed %d when unique amount codes are enabled", maxInvoiceAmount-99)
	}
	var merchant domain.MerchantAccount
	err := error(nil)
	if in.MerchantAccountID != "" {
		merchant, err = s.Repo.MerchantAccount(ctx, in.TenantID, in.MerchantAccountID)
	} else {
		accounts, listErr := s.Repo.ListMerchantAccounts(ctx, in.TenantID)
		if listErr != nil {
			return domain.Invoice{}, false, listErr
		}
		active := make([]domain.MerchantAccount, 0, len(accounts))
		for _, account := range accounts {
			if account.Active {
				active = append(active, account)
			}
		}
		sort.Slice(active, func(i, j int) bool { return active[i].ID < active[j].ID })
		if len(active) == 0 {
			return domain.Invoice{}, false, errors.New("no active merchant account configured for tenant")
		}
		merchant = active[0]
		in.MerchantAccountID = merchant.ID
	}
	if err != nil {
		return domain.Invoice{}, false, err
	}
	if !merchant.Active {
		return domain.Invoice{}, false, errors.New("merchant account inactive")
	}
	now := s.now()
	inv := domain.Invoice{ID: id(), TenantID: in.TenantID, MerchantAccountID: in.MerchantAccountID, IdempotencyKey: in.IdempotencyKey, RequestedAmount: in.Amount, Amount: in.Amount, Currency: in.Currency, Description: in.Description, Status: domain.InvoiceCreating, SandboxMode: in.SandboxMode, CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(30 * time.Minute)}
	inv, created, err := s.Repo.CreateInvoice(ctx, inv)
	if err != nil {
		return inv, created, err
	}
	if !created {
		requestedAmount := inv.RequestedAmount
		if requestedAmount == 0 {
			requestedAmount = inv.Amount
		}
		if inv.MerchantAccountID != in.MerchantAccountID || requestedAmount != in.Amount || inv.Currency != in.Currency || inv.Description != in.Description {
			return inv, false, ErrIdempotencyConflict
		}
		return inv, false, nil
	}
	if in.SandboxMode {
		template, templateErr := s.sandboxQRISTemplate(ctx, in.TenantID)
		if templateErr != nil {
			return domain.Invoice{}, true, s.failCreate(ctx, inv, templateErr)
		}
		if tenant.MerchantID == "" {
			return domain.Invoice{}, true, s.failCreate(ctx, inv, errors.New("tenant is not linked to a Merchant ID"))
		}
		codes := []int64{0}
		if tenant.UseUniqueAmountCode {
			order := s.UniqueAmountCodeOrder
			if order == nil {
				order = secureUniqueAmountCodeOrder
			}
			codes, err = order()
			if err != nil {
				return domain.Invoice{}, true, s.failCreate(ctx, inv, err)
			}
		}
		for _, amountCode := range codes {
			candidate := inv
			candidate.Amount = in.Amount + amountCode
			candidate.UniqueAmountCode = amountCode
			candidate.QRISTemplateID = template.ID
			candidate.QRISMerchantID = tenant.MerchantID
			candidate.QRISMerchantName = strings.TrimSpace(template.MerchantName)
			candidate.QRISMerchantCity = strings.TrimSpace(template.MerchantCity)
			candidate.ProviderReference = "sandbox-" + candidate.ID
			candidate.ProviderRequestDate = now.Format(time.RFC3339)
			candidate.QRPayload, err = qris.Convert(template.StaticPayload, candidate.Amount)
			if err != nil {
				return domain.Invoice{}, true, s.failCreate(ctx, inv, err)
			}
			if err = candidate.Transition(domain.InvoicePending, s.now()); err != nil {
				return domain.Invoice{}, true, err
			}
			if err = s.Repo.ActivateHostedInvoice(ctx, candidate); errors.Is(err, store.ErrUniqueAmountUnavailable) {
				continue
			} else if err != nil {
				return domain.Invoice{}, true, err
			}
			return candidate, true, nil
		}
		return domain.Invoice{}, true, s.failCreate(ctx, inv, store.ErrUniqueAmountUnavailable)
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

func secureUniqueAmountCodeOrder() ([]int64, error) {
	codes := make([]int64, 99)
	for i := range codes {
		codes[i] = int64(i + 1)
	}
	for i := len(codes) - 1; i > 0; i-- {
		index, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return nil, err
		}
		j := int(index.Int64())
		codes[i], codes[j] = codes[j], codes[i]
	}
	return codes, nil
}

func (s Service) sandboxQRISTemplate(ctx context.Context, tenantID string) (domain.QRISTemplate, error) {
	templates, err := s.Repo.ListQRISTemplates(ctx)
	if err != nil {
		return domain.QRISTemplate{}, err
	}
	eligible := make([]domain.QRISTemplate, 0, len(templates))
	for _, template := range templates {
		if template.Active && template.StaticToDynamic && qrisTemplateAccessible(template, tenantID) {
			eligible = append(eligible, template)
		}
	}
	if len(eligible) == 0 {
		return domain.QRISTemplate{}, errors.New("no active stored QRIS template is available for tenant")
	}
	sort.Slice(eligible, func(i, j int) bool {
		iOwned := eligible[i].TenantID == tenantID
		jOwned := eligible[j].TenantID == tenantID
		if iOwned != jOwned {
			return iOwned
		}
		if !eligible[i].CreatedAt.Equal(eligible[j].CreatedAt) {
			return eligible[i].CreatedAt.After(eligible[j].CreatedAt)
		}
		return eligible[i].ID < eligible[j].ID
	})
	return eligible[0], nil
}

func qrisTemplateAccessible(template domain.QRISTemplate, tenantID string) bool {
	scope := template.AccessScope
	if scope == "" {
		if template.TenantID == "" {
			scope = "all_tenants"
		} else {
			scope = "selected_tenant"
		}
	}
	return scope == "all_tenants" || (scope == "selected_tenant" && template.TenantID == tenantID)
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
