package gateway

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
	"xloyal/backend/internal/domain"
	"xloyal/backend/internal/store"
)

const xloyalStaticQRISFixture = "00020101021126570011ID.DANA.WWW011893600915303088327702090308832770303UMI51440014ID.CO.QRIS.WWW0215ID10265298200310303UMI5204504553033605802ID5915XLOYAL MERCHANT6014BANDAR LAMPUNG610565164630448AD"

type fakeProvider struct {
	creates, checks int
	status          domain.InvoiceStatus
}

func (f *fakeProvider) CreatePayment(context.Context, domain.CreatePaymentRequest) (domain.CreatePaymentResult, error) {
	f.creates++
	return domain.CreatePaymentResult{ProviderReference: "ref", QRPayload: "qr"}, nil
}
func (f *fakeProvider) CheckPayment(context.Context, domain.CheckPaymentRequest) (domain.CheckPaymentResult, error) {
	f.checks++
	return domain.CheckPaymentResult{Status: f.status}, nil
}
func (*fakeProvider) Health(context.Context) error { return nil }
func setup(t *testing.T) (Service, *store.Memory, *fakeProvider) {
	r := store.NewMemory()
	r.CreateTenant(context.Background(), domain.Tenant{ID: "t1", Active: true})
	r.CreateTenant(context.Background(), domain.Tenant{ID: "t2", Active: true})
	r.CreateMerchantAccount(context.Background(), domain.MerchantAccount{ID: "m1", TenantID: "t1", Active: true})
	p := &fakeProvider{status: domain.InvoicePaid}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return Service{Repo: r, Provider: func(context.Context, domain.MerchantAccount) (domain.PaymentProvider, error) { return p, nil }, Now: func() time.Time { return now }}, r, p
}
func TestCreateIdempotentAndTenantIsolated(t *testing.T) {
	s, _, p := setup(t)
	in := CreateInvoiceInput{TenantID: "t1", MerchantAccountID: "m1", IdempotencyKey: "key", Amount: 100}
	a, created, err := s.CreateInvoice(context.Background(), in)
	if err != nil || !created {
		t.Fatal(err)
	}
	b, created, err := s.CreateInvoice(context.Background(), in)
	if err != nil || created || a.ID != b.ID || p.creates != 1 {
		t.Fatal("idempotency failed")
	}
	if _, err = s.Invoice(context.Background(), "t2", a.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cross tenant read: %v", err)
	}
}

func TestCreateRejectsIdempotencyConflictAndNonIDR(t *testing.T) {
	s, _, _ := setup(t)
	base := CreateInvoiceInput{TenantID: "t1", MerchantAccountID: "m1", IdempotencyKey: "key", Amount: 100, Currency: "IDR", Description: "first"}
	if _, _, err := s.CreateInvoice(context.Background(), base); err != nil {
		t.Fatal(err)
	}
	changed := base
	changed.Amount = 200
	if _, _, err := s.CreateInvoice(context.Background(), changed); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("idempotency conflict error=%v", err)
	}
	changed = base
	changed.IdempotencyKey = "other"
	changed.Currency = "USD"
	if _, _, err := s.CreateInvoice(context.Background(), changed); err == nil {
		t.Fatal("non-IDR invoice accepted")
	}
}

func TestCreateSandboxInvoiceUsesStoredQRISTemplate(t *testing.T) {
	s, repo, provider := setup(t)
	ctx := context.Background()
	repo.CreateTenant(ctx, domain.Tenant{ID: "t1", MerchantID: "merchant-xloyal", UseUniqueAmountCode: true, UniqueAmountCooldownMinutes: 45, Active: true})
	s.UniqueAmountCodeOrder = func() ([]int64, error) { return []int64{1}, nil }
	createdAt := s.now().Add(-time.Hour)
	if err := repo.CreateQRISTemplate(ctx, domain.QRISTemplate{
		ID: "7ac58e464854be519b5a7e6d7a1f4519", Name: "Xloyal Merchant",
		StaticPayload: xloyalStaticQRISFixture, MerchantName: "XLOYAL MERCHANT",
		MerchantCity: "BANDAR LAMPUNG", AccessScope: "all_tenants",
		StaticToDynamic: true, Active: true, CreatedAt: createdAt,
	}); err != nil {
		t.Fatal(err)
	}

	invoice, created, err := s.CreateInvoice(ctx, CreateInvoiceInput{
		TenantID: "t1", MerchantAccountID: "m1", IdempotencyKey: "sandbox-stored-qris",
		Amount: 1000, SandboxMode: true,
	})
	if err != nil || !created {
		t.Fatalf("create sandbox invoice: created=%v err=%v", created, err)
	}
	if provider.creates != 0 {
		t.Fatalf("provider creates=%d, want 0", provider.creates)
	}
	if invoice.Status != domain.InvoicePending || !strings.HasPrefix(invoice.QRPayload, "000201010212") {
		t.Fatalf("invoice status=%s payload=%q", invoice.Status, invoice.QRPayload)
	}
	if invoice.RequestedAmount != 1000 || invoice.Amount != 1001 || invoice.UniqueAmountCode != 1 {
		t.Fatalf("hosted unique amount not applied: %+v", invoice)
	}
	if invoice.QRISTemplateID != "7ac58e464854be519b5a7e6d7a1f4519" || invoice.QRISMerchantName != "XLOYAL MERCHANT" || invoice.QRISMerchantCity != "BANDAR LAMPUNG" {
		t.Fatalf("hosted merchant metadata missing: %+v", invoice)
	}
	if !strings.Contains(invoice.QRPayload, "54041001") {
		t.Fatalf("sandbox QR does not contain payable amount 1001: %q", invoice.QRPayload)
	}
	if !strings.Contains(invoice.QRPayload, "XLOYAL MERCHANT") || !strings.Contains(invoice.QRPayload, "BANDAR LAMPUNG") {
		t.Fatalf("sandbox QR does not use stored Xloyal template: %q", invoice.QRPayload)
	}
}

func TestCreateSandboxInvoiceWithoutUniqueAmountKeepsRequestedAmount(t *testing.T) {
	s, repo, _ := setup(t)
	ctx := context.Background()
	repo.CreateTenant(ctx, domain.Tenant{ID: "t1", MerchantID: "merchant-xloyal", Active: true})
	if err := repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template", StaticPayload: xloyalStaticQRISFixture, MerchantName: "XLOYAL MERCHANT", MerchantCity: "BANDAR LAMPUNG", AccessScope: "all_tenants", StaticToDynamic: true, Active: true}); err != nil {
		t.Fatal(err)
	}
	invoice, _, err := s.CreateInvoice(ctx, CreateInvoiceInput{TenantID: "t1", MerchantAccountID: "m1", IdempotencyKey: "plain-hosted", Amount: 1000, SandboxMode: true})
	if err != nil {
		t.Fatal(err)
	}
	if invoice.RequestedAmount != 1000 || invoice.Amount != 1000 || invoice.UniqueAmountCode != 0 {
		t.Fatalf("unexpected hosted amount: %+v", invoice)
	}
}

func TestCreateSandboxInvoiceRejectsMissingOrIneligibleTemplates(t *testing.T) {
	tests := map[string]domain.QRISTemplate{
		"missing":      {},
		"inactive":     {ID: "inactive", StaticPayload: xloyalStaticQRISFixture, AccessScope: "all_tenants", StaticToDynamic: true},
		"static-only":  {ID: "static", StaticPayload: xloyalStaticQRISFixture, AccessScope: "all_tenants", Active: true},
		"other-tenant": {ID: "other", TenantID: "t2", StaticPayload: xloyalStaticQRISFixture, AccessScope: "selected_tenant", StaticToDynamic: true, Active: true},
	}
	for name, template := range tests {
		t.Run(name, func(t *testing.T) {
			s, repo, provider := setup(t)
			ctx := context.Background()
			if template.ID != "" {
				if err := repo.CreateQRISTemplate(ctx, template); err != nil {
					t.Fatal(err)
				}
			}
			_, created, err := s.CreateInvoice(ctx, CreateInvoiceInput{
				TenantID: "t1", MerchantAccountID: "m1", IdempotencyKey: "ineligible-" + name,
				Amount: 1000, SandboxMode: true,
			})
			if err == nil || !created {
				t.Fatalf("create sandbox invoice: created=%v err=%v", created, err)
			}
			if provider.creates != 0 {
				t.Fatalf("provider creates=%d, want 0", provider.creates)
			}
			invoices, listErr := repo.ListInvoices(ctx, "t1", 10)
			if listErr != nil || len(invoices) != 1 {
				t.Fatalf("list invoices: count=%d err=%v", len(invoices), listErr)
			}
			stored := invoices[0]
			if stored.Status != domain.InvoiceFailed || stored.QRPayload != "" {
				t.Fatalf("stored invoice status=%s payload=%q", stored.Status, stored.QRPayload)
			}
		})
	}
}

func TestCreateSandboxInvoicePrefersTenantTemplateDeterministically(t *testing.T) {
	s, repo, _ := setup(t)
	ctx := context.Background()
	now := s.now()
	templates := []domain.QRISTemplate{
		{ID: "global", Name: "Global", StaticPayload: xloyalStaticQRISFixture, AccessScope: "all_tenants", StaticToDynamic: true, Active: true, CreatedAt: now},
		{ID: "tenant-z", TenantID: "t1", Name: "Tenant Z", StaticPayload: xloyalStaticQRISFixture, AccessScope: "selected_tenant", StaticToDynamic: true, Active: true, CreatedAt: now},
		{ID: "tenant-a", TenantID: "t1", Name: "Tenant A", StaticPayload: xloyalStaticQRISFixture, AccessScope: "selected_tenant", StaticToDynamic: true, Active: true, CreatedAt: now},
	}
	for _, template := range templates {
		if err := repo.CreateQRISTemplate(ctx, template); err != nil {
			t.Fatal(err)
		}
	}

	selected, err := s.sandboxQRISTemplate(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != "tenant-a" {
		t.Fatalf("selected template=%q, want tenant-a", selected.ID)
	}
}

func TestCheckCooldownAvoidsProviderAmplification(t *testing.T) {
	s, r, p := setup(t)
	now := s.now()
	lastChecked := now.Add(-10 * time.Second)
	_, _, err := r.CreateInvoice(context.Background(), domain.Invoice{
		ID: "cooldown", TenantID: "t1", MerchantAccountID: "m1", IdempotencyKey: "cooldown",
		Amount: 100, Currency: "IDR", Status: domain.InvoicePending, CreatedAt: now, UpdatedAt: now,
		ExpiresAt: now.Add(time.Minute), LastCheckedAt: &lastChecked,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Check(context.Background(), "t1", "cooldown"); !errors.Is(err, ErrCheckCooldown) {
		t.Fatalf("cooldown error=%v", err)
	}
	if p.checks != 0 {
		t.Fatalf("provider checks=%d", p.checks)
	}
}

type concurrentCreateProvider struct {
	mu      sync.Mutex
	creates int
	started chan struct{}
	release chan struct{}
}

func (p *concurrentCreateProvider) CreatePayment(context.Context, domain.CreatePaymentRequest) (domain.CreatePaymentResult, error) {
	p.mu.Lock()
	p.creates++
	p.mu.Unlock()
	close(p.started)
	<-p.release
	return domain.CreatePaymentResult{ProviderReference: "ref", QRPayload: "qr"}, nil
}
func (*concurrentCreateProvider) CheckPayment(context.Context, domain.CheckPaymentRequest) (domain.CheckPaymentResult, error) {
	return domain.CheckPaymentResult{Status: domain.InvoicePending}, nil
}
func (*concurrentCreateProvider) Health(context.Context) error { return nil }

func TestConcurrentCreateIsIdempotent(t *testing.T) {
	r := store.NewMemory()
	r.CreateMerchantAccount(context.Background(), domain.MerchantAccount{ID: "m", TenantID: "t", Active: true})
	p := &concurrentCreateProvider{started: make(chan struct{}), release: make(chan struct{})}
	s := Service{Repo: r, Provider: func(context.Context, domain.MerchantAccount) (domain.PaymentProvider, error) { return p, nil }}
	in := CreateInvoiceInput{TenantID: "t", MerchantAccountID: "m", IdempotencyKey: "same", Amount: 100}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, created, err := s.CreateInvoice(context.Background(), in); err != nil || !created {
			t.Errorf("first create: created=%v err=%v", created, err)
		}
	}()
	<-p.started
	second, created, err := s.CreateInvoice(context.Background(), in)
	if err != nil || created || second.ID == "" {
		t.Fatalf("second create: invoice=%+v created=%v err=%v", second, created, err)
	}
	close(p.release)
	<-done
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.creates != 1 {
		t.Fatalf("provider creates=%d", p.creates)
	}
}

type orderedCheckProvider struct {
	mu             sync.Mutex
	calls          int
	entered        chan int
	releasePaid    chan struct{}
	releasePending chan struct{}
}

func (*orderedCheckProvider) CreatePayment(context.Context, domain.CreatePaymentRequest) (domain.CreatePaymentResult, error) {
	return domain.CreatePaymentResult{}, nil
}
func (p *orderedCheckProvider) CheckPayment(context.Context, domain.CheckPaymentRequest) (domain.CheckPaymentResult, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()
	p.entered <- call
	if call == 2 {
		<-p.releasePending
		return domain.CheckPaymentResult{Status: domain.InvoicePending}, nil
	}
	<-p.releasePaid
	return domain.CheckPaymentResult{Status: domain.InvoicePaid}, nil
}
func (*orderedCheckProvider) Health(context.Context) error { return nil }

func TestConcurrentCheckDoesNotOverwriteTerminalState(t *testing.T) {
	ctx := context.Background()
	r := store.NewMemory()
	r.CreateMerchantAccount(ctx, domain.MerchantAccount{ID: "m", TenantID: "t", Active: true})
	r.CreateInvoice(ctx, domain.Invoice{ID: "i", TenantID: "t", MerchantAccountID: "m", IdempotencyKey: "k", Status: domain.InvoicePending})
	p := &orderedCheckProvider{entered: make(chan int, 2), releasePaid: make(chan struct{}), releasePending: make(chan struct{})}
	s := Service{Repo: r, Provider: func(context.Context, domain.MerchantAccount) (domain.PaymentProvider, error) { return p, nil }}
	first := make(chan domain.Invoice, 1)
	go func() {
		inv, _ := s.Check(ctx, "t", "i")
		first <- inv
	}()
	<-p.entered
	second := make(chan domain.Invoice, 1)
	go func() {
		inv, _ := s.Check(ctx, "t", "i")
		second <- inv
	}()
	<-p.entered
	close(p.releasePaid)
	deadline := time.Now().Add(time.Second)
	for {
		inv, _ := r.Invoice(ctx, "t", "i")
		if inv.Status == domain.InvoicePaid {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("paid check did not finish")
		}
		time.Sleep(time.Millisecond)
	}
	close(p.releasePending)
	if got := <-first; got.Status != domain.InvoicePaid {
		t.Fatalf("first status=%s", got.Status)
	}
	if got := <-second; got.Status != domain.InvoicePaid {
		t.Fatalf("second status=%s", got.Status)
	}
}
