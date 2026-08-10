package store

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"xloyal/backend/internal/domain"
)

var ErrNotFound = errors.New("not found")

type Repository interface {
	Health(context.Context) error
	TenantByAPIKey(context.Context, string) (domain.Tenant, error)
	CreateTenant(context.Context, domain.Tenant) error
	ListTenants(context.Context) ([]domain.Tenant, error)
	CreateMerchantAccount(context.Context, domain.MerchantAccount) error
	MerchantAccount(context.Context, string, string) (domain.MerchantAccount, error)
	ListMerchantAccounts(context.Context, string) ([]domain.MerchantAccount, error)
	CreateInvoice(context.Context, domain.Invoice) (domain.Invoice, bool, error)
	Invoice(context.Context, string, string) (domain.Invoice, error)
	UpdateInvoice(context.Context, domain.Invoice) error
	UpdatePendingInvoice(context.Context, domain.Invoice) (bool, error)
	PendingInvoices(context.Context, time.Time, int) ([]domain.Invoice, error)
	ListInvoices(context.Context, string, int) ([]domain.Invoice, error)
	CreateQRISTemplate(context.Context, domain.QRISTemplate) error
	QRISTemplate(context.Context, string) (domain.QRISTemplate, error)
	ListQRISTemplates(context.Context) ([]domain.QRISTemplate, error)
	CreateTestPayment(context.Context, domain.TestPayment) error
	ListTestPayments(context.Context, int) ([]domain.TestPayment, error)
	AppendAudit(context.Context, domain.AuditEvent) error
	ListAudit(context.Context, string, int) ([]domain.AuditEvent, error)
}

type Memory struct {
	mu        sync.Mutex
	tenants   map[string]domain.Tenant
	merchants map[string]domain.MerchantAccount
	invoices  map[string]domain.Invoice
	idem      map[string]string
	audits    []domain.AuditEvent
	templates map[string]domain.QRISTemplate
	payments  map[string]domain.TestPayment
}

func NewMemory() *Memory {
	return &Memory{
		tenants: map[string]domain.Tenant{}, merchants: map[string]domain.MerchantAccount{},
		invoices: map[string]domain.Invoice{}, idem: map[string]string{},
		templates: map[string]domain.QRISTemplate{}, payments: map[string]domain.TestPayment{},
	}
}
func (m *Memory) Health(context.Context) error { return nil }
func (m *Memory) TenantByAPIKey(_ context.Context, hash string) (domain.Tenant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.tenants {
		if v.APIKeyHash == hash && v.Active {
			return v, nil
		}
	}
	return domain.Tenant{}, ErrNotFound
}
func (m *Memory) CreateTenant(_ context.Context, v domain.Tenant) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tenants[v.ID] = v
	return nil
}
func (m *Memory) ListTenants(context.Context) ([]domain.Tenant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := make([]domain.Tenant, 0, len(m.tenants))
	for _, v := range m.tenants {
		r = append(r, v)
	}
	return r, nil
}
func (m *Memory) CreateMerchantAccount(_ context.Context, v domain.MerchantAccount) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.merchants[v.ID] = v
	return nil
}
func (m *Memory) MerchantAccount(_ context.Context, tenant, id string) (domain.MerchantAccount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.merchants[id]
	if !ok || v.TenantID != tenant {
		return domain.MerchantAccount{}, ErrNotFound
	}
	return v, nil
}
func (m *Memory) ListMerchantAccounts(_ context.Context, tenant string) ([]domain.MerchantAccount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var r []domain.MerchantAccount
	for _, v := range m.merchants {
		if tenant == "" || v.TenantID == tenant {
			r = append(r, v)
		}
	}
	return r, nil
}
func (m *Memory) CreateInvoice(_ context.Context, v domain.Invoice) (domain.Invoice, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := v.TenantID + "\x00" + v.IdempotencyKey
	if id, ok := m.idem[k]; ok {
		return m.invoices[id], false, nil
	}
	m.invoices[v.ID] = v
	m.idem[k] = v.ID
	return v, true, nil
}
func (m *Memory) Invoice(_ context.Context, tenant, id string) (domain.Invoice, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.invoices[id]
	if !ok || (tenant != "" && v.TenantID != tenant) {
		return domain.Invoice{}, ErrNotFound
	}
	return v, nil
}
func (m *Memory) UpdateInvoice(_ context.Context, v domain.Invoice) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	old, ok := m.invoices[v.ID]
	if !ok || old.TenantID != v.TenantID {
		return ErrNotFound
	}
	m.invoices[v.ID] = v
	return nil
}
func (m *Memory) UpdatePendingInvoice(_ context.Context, v domain.Invoice) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	old, ok := m.invoices[v.ID]
	if !ok || old.TenantID != v.TenantID {
		return false, ErrNotFound
	}
	if old.Status != domain.InvoicePending {
		return false, nil
	}
	m.invoices[v.ID] = v
	return true, nil
}
func (m *Memory) PendingInvoices(_ context.Context, due time.Time, limit int) ([]domain.Invoice, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var r []domain.Invoice
	for _, v := range m.invoices {
		if v.Status == domain.InvoicePending && (v.LastCheckedAt == nil || !v.LastCheckedAt.After(due)) {
			r = append(r, v)
		}
	}
	sort.Slice(r, func(i, j int) bool { return r[i].CreatedAt.Before(r[j].CreatedAt) })
	if len(r) > limit {
		r = r[:limit]
	}
	return r, nil
}
func (m *Memory) ListInvoices(_ context.Context, tenant string, limit int) ([]domain.Invoice, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var r []domain.Invoice
	for _, v := range m.invoices {
		if tenant == "" || v.TenantID == tenant {
			r = append(r, v)
		}
	}
	sort.Slice(r, func(i, j int) bool { return r[i].CreatedAt.After(r[j].CreatedAt) })
	if len(r) > limit {
		r = r[:limit]
	}
	return r, nil
}
func (m *Memory) CreateQRISTemplate(_ context.Context, v domain.QRISTemplate) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.templates[v.ID] = v
	return nil
}
func (m *Memory) QRISTemplate(_ context.Context, id string) (domain.QRISTemplate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.templates[id]
	if !ok {
		return domain.QRISTemplate{}, ErrNotFound
	}
	return v, nil
}
func (m *Memory) ListQRISTemplates(context.Context) ([]domain.QRISTemplate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.QRISTemplate, 0, len(m.templates))
	for _, v := range m.templates {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
func (m *Memory) CreateTestPayment(_ context.Context, v domain.TestPayment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.templates[v.QRISTemplateID]; !ok {
		return ErrNotFound
	}
	m.payments[v.ID] = v
	return nil
}
func (m *Memory) ListTestPayments(_ context.Context, limit int) ([]domain.TestPayment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.TestPayment, 0, len(m.payments))
	for _, v := range m.payments {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (m *Memory) AppendAudit(_ context.Context, v domain.AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.audits = append(m.audits, v)
	return nil
}
func (m *Memory) ListAudit(_ context.Context, tenant string, limit int) ([]domain.AuditEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var r []domain.AuditEvent
	for _, v := range m.audits {
		if tenant == "" || v.TenantID == tenant {
			r = append(r, v)
		}
	}
	sort.Slice(r, func(i, j int) bool { return r[i].CreatedAt.After(r[j].CreatedAt) })
	if len(r) > limit {
		r = r[:limit]
	}
	return r, nil
}
