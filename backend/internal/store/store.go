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
	Tenant(context.Context, string) (domain.Tenant, error)
	CreateTenant(context.Context, domain.Tenant) error
	UpdateTenant(context.Context, domain.Tenant) error
	ListTenants(context.Context) ([]domain.Tenant, error)
	AssignTenantMerchant(context.Context, string, string) error
	CreateMerchantID(context.Context, domain.MerchantID) error
	MerchantID(context.Context, string) (domain.MerchantID, error)
	ListMerchantIDs(context.Context) ([]domain.MerchantID, error)
	UpsertMerchantConnection(context.Context, domain.MerchantConnection) error
	MerchantConnection(context.Context, string) (domain.MerchantConnection, error)
	ListDueMerchantConnections(context.Context, time.Time, int) ([]domain.MerchantConnection, error)
	CreatePortalTransaction(context.Context, domain.PortalTransaction) error
	ListPortalTransactions(context.Context, string, string, int) ([]domain.PortalTransaction, error)
	UpsertTariff(context.Context, domain.Tariff) error
	Tariff(context.Context, string) (domain.Tariff, error)
	CreateMerchantAccount(context.Context, domain.MerchantAccount) error
	MerchantAccount(context.Context, string, string) (domain.MerchantAccount, error)
	UpdateMerchantAccount(context.Context, domain.MerchantAccount) error
	ListMerchantAccounts(context.Context, string) ([]domain.MerchantAccount, error)
	CreateInvoice(context.Context, domain.Invoice) (domain.Invoice, bool, error)
	Invoice(context.Context, string, string) (domain.Invoice, error)
	UpdateInvoice(context.Context, domain.Invoice) error
	UpdatePendingInvoice(context.Context, domain.Invoice) (bool, error)
	PendingInvoices(context.Context, time.Time, int) ([]domain.Invoice, error)
	ListInvoices(context.Context, string, int) ([]domain.Invoice, error)
	CreateQRISTemplate(context.Context, domain.QRISTemplate) error
	UpdateQRISTemplate(context.Context, domain.QRISTemplate) error
	QRISTemplate(context.Context, string) (domain.QRISTemplate, error)
	ListQRISTemplates(context.Context) ([]domain.QRISTemplate, error)
	AllowQRISRequest(context.Context, string, string, time.Time, int) (bool, int, error)
	CreateTestPayment(context.Context, domain.TestPayment) error
	TestPayment(context.Context, string) (domain.TestPayment, error)
	UpdatePendingTestPayment(context.Context, domain.TestPayment) (bool, error)
	PendingTestPayments(context.Context, time.Time, int) ([]domain.TestPayment, error)
	ExpirePendingTestPayments(context.Context, time.Time) (int64, error)
	ListTestPayments(context.Context, int) ([]domain.TestPayment, error)
	AppendAudit(context.Context, domain.AuditEvent) error
	ListAudit(context.Context, string, int) ([]domain.AuditEvent, error)
}

type Memory struct {
	mu                 sync.Mutex
	tenants            map[string]domain.Tenant
	merchantIDs        map[string]domain.MerchantID
	connections        map[string]domain.MerchantConnection
	portalTransactions map[string]domain.PortalTransaction
	tariffs            map[string]domain.Tariff
	merchants          map[string]domain.MerchantAccount
	invoices           map[string]domain.Invoice
	idem               map[string]string
	audits             []domain.AuditEvent
	templates          map[string]domain.QRISTemplate
	payments           map[string]domain.TestPayment
	qrisRateWindows    map[string]qrisRateWindow
}

type qrisRateWindow struct {
	StartedAt time.Time
	Count     int
}

func NewMemory() *Memory {
	return &Memory{
		tenants: map[string]domain.Tenant{}, merchants: map[string]domain.MerchantAccount{},
		merchantIDs: map[string]domain.MerchantID{}, connections: map[string]domain.MerchantConnection{}, portalTransactions: map[string]domain.PortalTransaction{}, tariffs: map[string]domain.Tariff{},
		invoices: map[string]domain.Invoice{}, idem: map[string]string{},
		templates: map[string]domain.QRISTemplate{}, payments: map[string]domain.TestPayment{}, qrisRateWindows: map[string]qrisRateWindow{},
	}
}
func (m *Memory) AssignTenantMerchant(_ context.Context, tenantID, merchantID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.tenants[tenantID]
	if !ok {
		return ErrNotFound
	}
	v.MerchantID = merchantID
	m.tenants[tenantID] = v
	return nil
}
func (m *Memory) CreateMerchantID(_ context.Context, v domain.MerchantID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.merchantIDs[v.ID] = v
	return nil
}
func (m *Memory) MerchantID(_ context.Context, id string) (domain.MerchantID, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.merchantIDs[id]
	if !ok {
		return domain.MerchantID{}, ErrNotFound
	}
	return v, nil
}
func (m *Memory) ListMerchantIDs(context.Context) ([]domain.MerchantID, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.MerchantID, 0, len(m.merchantIDs))
	for _, v := range m.merchantIDs {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
func (m *Memory) UpsertMerchantConnection(_ context.Context, v domain.MerchantConnection) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connections[v.MerchantID] = v
	return nil
}
func (m *Memory) MerchantConnection(_ context.Context, id string) (domain.MerchantConnection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.connections[id]
	if !ok {
		return domain.MerchantConnection{}, ErrNotFound
	}
	return v, nil
}
func (m *Memory) ListDueMerchantConnections(_ context.Context, due time.Time, limit int) ([]domain.MerchantConnection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []domain.MerchantConnection{}
	for _, v := range m.connections {
		eligible := v.Status == domain.ConnectionConnected || v.Status == domain.ConnectionReconnectRequired
		lastAttempt := v.UpdatedAt
		if v.LastSyncedAt != nil && v.LastSyncedAt.After(lastAttempt) {
			lastAttempt = *v.LastSyncedAt
		}
		if eligible && !lastAttempt.After(due) {
			out = append(out, v)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (m *Memory) CreatePortalTransaction(_ context.Context, v domain.PortalTransaction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, existing := range m.portalTransactions {
		if existing.MerchantID != v.MerchantID || existing.Reference != v.Reference {
			continue
		}
		v.ID, v.CreatedAt = existing.ID, existing.CreatedAt
		if existing.TenantID != "" {
			v.TenantID, v.InvoiceID, v.MatchConfidence = existing.TenantID, existing.InvoiceID, existing.MatchConfidence
		}
		m.portalTransactions[id] = v
		return nil
	}
	m.portalTransactions[v.ID] = v
	return nil
}
func (m *Memory) ListPortalTransactions(_ context.Context, merchantID, tenantID string, limit int) ([]domain.PortalTransaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []domain.PortalTransaction{}
	for _, v := range m.portalTransactions {
		if (merchantID == "" || v.MerchantID == merchantID) && (tenantID == "" || v.TenantID == tenantID) {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PaidAt.After(out[j].PaidAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (m *Memory) UpsertTariff(_ context.Context, v domain.Tariff) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tariffs[v.MerchantID] = v
	return nil
}
func (m *Memory) Tariff(_ context.Context, merchantID string) (domain.Tariff, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.tariffs[merchantID]
	if !ok {
		return domain.Tariff{}, ErrNotFound
	}
	return v, nil
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
func (m *Memory) Tenant(_ context.Context, id string) (domain.Tenant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.tenants[id]
	if !ok {
		return domain.Tenant{}, ErrNotFound
	}
	return v, nil
}
func (m *Memory) CreateTenant(_ context.Context, v domain.Tenant) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tenants[v.ID] = v
	return nil
}
func (m *Memory) UpdateTenant(_ context.Context, v domain.Tenant) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.tenants[v.ID]; !ok {
		return ErrNotFound
	}
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
func (m *Memory) UpdateMerchantAccount(_ context.Context, v domain.MerchantAccount) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.merchants[v.ID]; !ok {
		return ErrNotFound
	}
	m.merchants[v.ID] = v
	return nil
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
		if v.Status == domain.InvoicePending && (v.LastCheckedAt == nil || v.LastCheckedAt.Before(due)) {
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
func (m *Memory) UpdateQRISTemplate(_ context.Context, v domain.QRISTemplate) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.templates[v.ID]; !ok {
		return ErrNotFound
	}
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
func (m *Memory) AllowQRISRequest(_ context.Context, templateID, tenantID string, now time.Time, max int) (bool, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	windowStart := now.UTC().Truncate(time.Minute)
	key := templateID + "\x00" + tenantID
	window := m.qrisRateWindows[key]
	if window.StartedAt.Before(windowStart) {
		window = qrisRateWindow{StartedAt: windowStart}
	}
	window.Count++
	m.qrisRateWindows[key] = window
	retry := int(windowStart.Add(time.Minute).Sub(now.UTC()).Seconds())
	if retry < 1 {
		retry = 1
	}
	return window.Count <= max, retry, nil
}
func (m *Memory) CreateTestPayment(_ context.Context, v domain.TestPayment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.templates[v.QRISTemplateID]; !ok {
		return ErrNotFound
	}
	if v.NextCheckAt == nil && v.Status == domain.InvoicePending {
		next := v.CreatedAt.Add(15 * time.Second)
		v.NextCheckAt = &next
	}
	m.payments[v.ID] = v
	return nil
}
func (m *Memory) TestPayment(_ context.Context, id string) (domain.TestPayment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.payments[id]
	if !ok {
		return domain.TestPayment{}, ErrNotFound
	}
	return v, nil
}
func (m *Memory) UpdatePendingTestPayment(_ context.Context, v domain.TestPayment) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	old, ok := m.payments[v.ID]
	if !ok {
		return false, ErrNotFound
	}
	if old.Status != domain.InvoicePending {
		return false, nil
	}
	m.payments[v.ID] = v
	return true, nil
}
func (m *Memory) PendingTestPayments(_ context.Context, due time.Time, limit int) ([]domain.TestPayment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.TestPayment, 0)
	for _, v := range m.payments {
		if v.Status == domain.InvoicePending && v.NextCheckAt != nil && !v.NextCheckAt.After(due) {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (m *Memory) ExpirePendingTestPayments(_ context.Context, now time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var count int64
	for id, v := range m.payments {
		if v.Status != domain.InvoicePending || v.ExpiresAt.After(now) {
			continue
		}
		v.Status = domain.InvoiceExpired
		v.UpdatedAt = now
		v.NextCheckAt = nil
		v.MatchConfidence = "expired_no_match"
		m.payments[id] = v
		count++
	}
	return count, nil
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
