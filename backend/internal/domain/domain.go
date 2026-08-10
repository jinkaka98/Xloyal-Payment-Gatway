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
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	APIKeyHash string    `json:"-"`
	Active     bool      `json:"active"`
	CreatedAt  time.Time `json:"created_at"`
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
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	StaticPayload string    `json:"-"`
	ImageMIME     string    `json:"image_mime"`
	ImageData     []byte    `json:"-"`
	MerchantName  string    `json:"merchant_name"`
	MerchantCity  string    `json:"merchant_city"`
	CreatedAt     time.Time `json:"created_at"`
}

type TestPayment struct {
	ID             string        `json:"id"`
	QRISTemplateID string        `json:"qris_template_id"`
	Amount         int64         `json:"amount"`
	DynamicPayload string        `json:"dynamic_payload"`
	Status         InvoiceStatus `json:"status"`
	CreatedAt      time.Time     `json:"created_at"`
	ExpiresAt      time.Time     `json:"expires_at"`
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
