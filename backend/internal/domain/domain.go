package domain

import (
	"context"
	"errors"
	"time"
)

type InvoiceStatus string

const (
	InvoiceCreating InvoiceStatus = "creating"
	InvoicePending  InvoiceStatus = "pending"
	InvoicePaid     InvoiceStatus = "paid"
	InvoiceExpired  InvoiceStatus = "expired"
	InvoiceFailed   InvoiceStatus = "failed"
)

var ErrInvalidTransition = errors.New("invalid invoice status transition")

type Invoice struct {
	ID                  string        `json:"id"`
	TenantID            string        `json:"tenant_id"`
	MerchantAccountID   string        `json:"merchant_account_id"`
	IdempotencyKey      string        `json:"idempotency_key"`
	Amount              int64         `json:"amount"`
	Currency            string        `json:"currency"`
	Description         string        `json:"description"`
	ProviderReference   string        `json:"provider_reference"`
	ProviderRequestDate string        `json:"provider_request_date"`
	QRPayload           string        `json:"qr_payload"`
	Status              InvoiceStatus `json:"status"`
	CreatedAt           time.Time     `json:"created_at"`
	UpdatedAt           time.Time     `json:"updated_at"`
	ExpiresAt           time.Time     `json:"expires_at"`
	LastCheckedAt       *time.Time    `json:"last_checked_at"`
	CheckCount          int           `json:"check_count"`
}

func (i *Invoice) Transition(next InvoiceStatus, now time.Time) error {
	allowed := (i.Status == InvoiceCreating && (next == InvoicePending || next == InvoiceFailed)) || (i.Status == InvoicePending && (next == InvoicePaid || next == InvoiceExpired || next == InvoiceFailed))
	if !allowed {
		return ErrInvalidTransition
	}
	i.Status, i.UpdatedAt = next, now
	return nil
}

type Tenant struct {
	ID          string    `json:"id"`
	MerchantID  string    `json:"merchant_id,omitempty"`
	Name        string    `json:"name"`
	SiteURL     string    `json:"site_url,omitempty"`
	CallbackURL string    `json:"callback_url,omitempty"`
	WebhookURL  string    `json:"webhook_url,omitempty"`
	APIKeyHash  string    `json:"-"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
}
type MerchantID struct {
	ID                    string    `json:"id"`
	InteractiveMerchantID string    `json:"interactive_merchant_id"`
	Name                  string    `json:"name"`
	CredentialCiphertext  string    `json:"-"`
	Active                bool      `json:"active"`
	CreatedAt             time.Time `json:"created_at"`
}

type ConnectionStatus string

const (
	ConnectionDisconnected      ConnectionStatus = "disconnected"
	ConnectionConnected         ConnectionStatus = "connected"
	ConnectionExpired           ConnectionStatus = "expired"
	ConnectionReconnectRequired ConnectionStatus = "reconnect_required"
)

type MerchantConnection struct {
	MerchantID                  string           `json:"merchant_id"`
	SessionCiphertext           string           `json:"-"`
	BrowserCredentialCiphertext string           `json:"-"`
	Status                      ConnectionStatus `json:"status"`
	LastSyncedAt                *time.Time       `json:"last_synced_at,omitempty"`
	HistoryBackfilledAt         *time.Time       `json:"history_backfilled_at,omitempty"`
	LastError                   string           `json:"last_error,omitempty"`
	UpdatedAt                   time.Time        `json:"updated_at"`
}

type PortalTransaction struct {
	ID              string    `json:"id"`
	MerchantID      string    `json:"merchant_id"`
	TenantID        string    `json:"tenant_id,omitempty"`
	Reference       string    `json:"reference"`
	Amount          int64     `json:"amount"`
	Status          string    `json:"status"`
	PaidAt          time.Time `json:"paid_at"`
	Source          string    `json:"source"`
	MatchConfidence string    `json:"match_confidence"`
	InvoiceID       string    `json:"invoice_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type Tariff struct {
	MerchantID  string    `json:"merchant_id"`
	BasisPoints int64     `json:"basis_points"`
	FixedFee    int64     `json:"fixed_fee"`
	Active      bool      `json:"active"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type MerchantAccount struct {
	ID                   string    `json:"id"`
	TenantID             string    `json:"tenant_id"`
	Provider             string    `json:"provider"`
	Name                 string    `json:"name"`
	CredentialCiphertext string    `json:"-"`
	Active               bool      `json:"active"`
	CreatedAt            time.Time `json:"created_at"`
}
type AuditEvent struct {
	ID           string         `json:"id"`
	TenantID     string         `json:"tenant_id,omitempty"`
	Actor        string         `json:"actor"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    time.Time      `json:"created_at"`
}

type QRISTemplate struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id,omitempty"`
	Name            string    `json:"name"`
	StaticPayload   string    `json:"-"`
	ImageMIME       string    `json:"image_mime"`
	ImageData       []byte    `json:"-"`
	MerchantName    string    `json:"merchant_name"`
	MerchantCity    string    `json:"merchant_city"`
	AccessScope     string    `json:"access_scope"`
	StaticToDynamic bool      `json:"static_to_dynamic"`
	MaxRequestsPM   int       `json:"max_requests_per_minute"`
	Active          bool      `json:"active"`
	CreatedAt       time.Time `json:"created_at"`
}

type TestPayment struct {
	ID                   string        `json:"id"`
	QRISTemplateID       string        `json:"qris_template_id"`
	MerchantID           string        `json:"merchant_id,omitempty"`
	TenantID             string        `json:"tenant_id,omitempty"`
	Amount               int64         `json:"amount"`
	DynamicPayload       string        `json:"dynamic_payload"`
	Status               InvoiceStatus `json:"status"`
	RequestSource        string        `json:"request_source"`
	MatchConfidence      string        `json:"match_confidence"`
	MatchedTransactionID string        `json:"matched_transaction_id,omitempty"`
	CreatedAt            time.Time     `json:"created_at"`
	UpdatedAt            time.Time     `json:"updated_at"`
	ExpiresAt            time.Time     `json:"expires_at"`
	LastCheckedAt        *time.Time    `json:"last_checked_at,omitempty"`
	NextCheckAt          *time.Time    `json:"next_check_at,omitempty"`
	CheckCount           int           `json:"check_count"`
}

type GlobalTransactionLog struct {
	ID                   string     `json:"id"`
	EventType            string     `json:"event_type"`
	MerchantID           string     `json:"merchant_id,omitempty"`
	TenantID             string     `json:"tenant_id,omitempty"`
	Reference            string     `json:"reference"`
	Amount               int64      `json:"amount"`
	Status               string     `json:"status"`
	EventAt              time.Time  `json:"event_at"`
	Source               string     `json:"source"`
	RequestSource        string     `json:"request_source"`
	Validation           string     `json:"validation"`
	ExpiresAt            *time.Time `json:"expires_at,omitempty"`
	LastCheckedAt        *time.Time `json:"last_checked_at,omitempty"`
	NextCheckAt          *time.Time `json:"next_check_at,omitempty"`
	CheckCount           int        `json:"check_count"`
	InvoiceID            string     `json:"invoice_id,omitempty"`
	TestPaymentID        string     `json:"test_payment_id,omitempty"`
	MatchedTransactionID string     `json:"matched_transaction_id,omitempty"`
}

type CreatePaymentRequest struct {
	InvoiceID             string
	Amount                int64
	Currency, Description string
	ExpiresAt             time.Time
}
type CreatePaymentResult struct{ ProviderReference, QRPayload, ProviderRequestDate string }
type CheckPaymentRequest struct {
	ProviderInvoiceID, RequestDate string
	Amount                         int64
}
type CheckPaymentResult struct{ Status InvoiceStatus }

type PaymentProvider interface {
	CreatePayment(ctx context.Context, req CreatePaymentRequest) (CreatePaymentResult, error)
	CheckPayment(ctx context.Context, req CheckPaymentRequest) (CheckPaymentResult, error)
	Health(ctx context.Context) error
}
