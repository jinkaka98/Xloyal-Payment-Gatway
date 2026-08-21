package domain

import (
	"context"
	"errors"
	"time"
)

type InvoiceStatus string

const (
	InvoiceCreating  InvoiceStatus = "creating"
	InvoicePending   InvoiceStatus = "pending"
	InvoicePaid      InvoiceStatus = "paid"
	InvoiceExpired   InvoiceStatus = "expired"
	InvoiceFailed    InvoiceStatus = "failed"
	InvoiceCancelled InvoiceStatus = "cancelled"
)

func IsPaymentEventType(eventType string) bool {
	switch eventType {
	case PaymentEventCreated, PaymentEventPending, PaymentEventVerifying, PaymentEventPaid,
		PaymentEventFailed, PaymentEventExpired, PaymentEventCancelled, PaymentEventRedirecting, PaymentEventClosed:
		return true
	default:
		return false
	}
}

const (
	PaymentEventCreated     = "payment.created"
	PaymentEventPending     = "payment.pending"
	PaymentEventVerifying   = "payment.verifying"
	PaymentEventPaid        = "payment.paid"
	PaymentEventFailed      = "payment.failed"
	PaymentEventExpired     = "payment.expired"
	PaymentEventCancelled   = "payment.cancelled"
	PaymentEventRedirecting = "payment.redirecting"
	PaymentEventClosed      = "payment.closed"

	OutboxPending    = "PENDING"
	OutboxProcessing = "PROCESSING"
	OutboxDelivered  = "DELIVERED"
	OutboxFailed     = "FAILED"

	ThemeDraft     = "DRAFT"
	ThemePublished = "PUBLISHED"
	ThemeArchived  = "ARCHIVED"

	RedirectSuccess = "SUCCESS"
	RedirectCancel  = "CANCEL"
	RedirectFailed  = "FAILED"
	RedirectExpired = "EXPIRED"
)

var ErrInvalidTransition = errors.New("invalid invoice status transition")

type PaymentSessionStatus string

const (
	PaymentSessionOpen           PaymentSessionStatus = "OPEN"
	PaymentSessionPaymentPending PaymentSessionStatus = "PAYMENT_PENDING"
	PaymentSessionPaid           PaymentSessionStatus = "PAID"
	PaymentSessionCancelled      PaymentSessionStatus = "CANCELLED"
	PaymentSessionExpired        PaymentSessionStatus = "EXPIRED"
	PaymentSessionFailed         PaymentSessionStatus = "FAILED"
	PaymentSessionRedirecting    PaymentSessionStatus = "REDIRECTING"
	PaymentSessionClosed         PaymentSessionStatus = "CLOSED"
)

var ErrInvalidPaymentSessionTransition = errors.New("invalid payment session status transition")

func (s PaymentSessionStatus) CanTransition(next PaymentSessionStatus) bool {
	return (s == PaymentSessionOpen && next == PaymentSessionPaymentPending) ||
		(s == PaymentSessionPaymentPending && (next == PaymentSessionPaid || next == PaymentSessionCancelled || next == PaymentSessionExpired || next == PaymentSessionFailed)) ||
		(s == PaymentSessionPaid && next == PaymentSessionRedirecting) ||
		(s == PaymentSessionRedirecting && next == PaymentSessionClosed)
}

// PaymentEventTypeForTransition binds persisted lifecycle events to the state
// transition that produced them. Transport callers cannot choose an unrelated
// event name for a valid state change.
func (s PaymentSessionStatus) PaymentEventTypeForTransition(next PaymentSessionStatus) (string, bool) {
	if !s.CanTransition(next) {
		return "", false
	}
	switch next {
	case PaymentSessionPaymentPending:
		return PaymentEventPending, true
	case PaymentSessionPaid:
		return PaymentEventPaid, true
	case PaymentSessionCancelled:
		return PaymentEventCancelled, true
	case PaymentSessionExpired:
		return PaymentEventExpired, true
	case PaymentSessionFailed:
		return PaymentEventFailed, true
	case PaymentSessionRedirecting:
		return PaymentEventRedirecting, true
	case PaymentSessionClosed:
		return PaymentEventClosed, true
	default:
		return "", false
	}
}

func (s PaymentSessionStatus) InvoiceTerminalStatus() (InvoiceStatus, bool) {
	switch s {
	case PaymentSessionPaid:
		return InvoicePaid, true
	case PaymentSessionExpired:
		return InvoiceExpired, true
	case PaymentSessionFailed:
		return InvoiceFailed, true
	default:
		return "", false
	}
}

type PaymentSession struct {
	ID              string               `json:"id"`
	TenantID        string               `json:"tenant_id"`
	InvoiceID       string               `json:"invoice_id"`
	PublicTokenHash string               `json:"-"`
	Status          PaymentSessionStatus `json:"status"`
	ThemeID         string               `json:"theme_id,omitempty"`
	ThemeVersion    int                  `json:"theme_version"`
	ReturnURL       string               `json:"return_url,omitempty"`
	SuccessURL      string               `json:"success_url,omitempty"`
	CancelURL       string               `json:"cancel_url,omitempty"`
	FailedURL       string               `json:"failed_url,omitempty"`
	ExpiredURL      string               `json:"expired_url,omitempty"`
	ExpiresAt       time.Time            `json:"expires_at"`
	LastSeenAt      *time.Time           `json:"last_seen_at,omitempty"`
	CreatedAt       time.Time            `json:"created_at"`
	UpdatedAt       time.Time            `json:"updated_at"`
}

func (p *PaymentSession) Transition(next PaymentSessionStatus, now time.Time) error {
	if !p.Status.CanTransition(next) {
		return ErrInvalidPaymentSessionTransition
	}
	p.Status = next
	p.UpdatedAt = now
	return nil
}

type PaymentEvent struct {
	ID               string    `json:"id"`
	EventID          string    `json:"event_id"`
	TenantID         string    `json:"tenant_id"`
	InvoiceID        string    `json:"invoice_id"`
	PaymentSessionID string    `json:"payment_session_id"`
	SequenceNumber   int64     `json:"sequence_number"`
	EventType        string    `json:"event_type"`
	Payload          []byte    `json:"payload"`
	OccurredAt       time.Time `json:"occurred_at"`
	CreatedAt        time.Time `json:"created_at"`
}

type OutboxEvent struct {
	ID            string     `json:"id"`
	EventID       string     `json:"event_id"`
	TenantID      string     `json:"tenant_id"`
	EventType     string     `json:"event_type"`
	AggregateType string     `json:"aggregate_type"`
	AggregateID   string     `json:"aggregate_id"`
	Payload       []byte     `json:"payload"`
	Status        string     `json:"status"`
	AttemptCount  int        `json:"attempt_count"`
	NextAttemptAt time.Time  `json:"next_attempt_at"`
	LastError     string     `json:"last_error,omitempty"`
	LockedAt      *time.Time `json:"locked_at,omitempty"`
	LockedBy      string     `json:"locked_by,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	ProcessedAt   *time.Time `json:"processed_at,omitempty"`
}

const (
	WebhookDeliveryPending    = "PENDING"
	WebhookDeliveryDelivering = "DELIVERING"
	WebhookDeliveryRetrying   = "RETRYING"
	WebhookDeliveryDelivered  = "DELIVERED"
	WebhookDeliveryFailed     = "FAILED"
)

type WebhookDelivery struct {
	ID               string     `json:"id"`
	TenantID         string     `json:"tenant_id"`
	EventID          string     `json:"event_id"`
	EventType        string     `json:"event_type"`
	PaymentSessionID string     `json:"payment_session_id"`
	InvoiceID        string     `json:"invoice_id"`
	Endpoint         string     `json:"endpoint"`
	Payload          []byte     `json:"payload"`
	Status           string     `json:"status"`
	AttemptCount     int        `json:"attempt_count"`
	NextAttemptAt    time.Time  `json:"next_attempt_at"`
	LastError        string     `json:"last_error,omitempty"`
	LastStatusCode   int        `json:"last_status_code,omitempty"`
	LockedAt         *time.Time `json:"locked_at,omitempty"`
	LockedBy         string     `json:"locked_by,omitempty"`
	DeliveredAt      *time.Time `json:"delivered_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type PaymentTheme struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	Name           string    `json:"name"`
	Status         string    `json:"status"`
	IsDefault      bool      `json:"is_default"`
	CurrentVersion int       `json:"current_version"`
	DraftConfig    []byte    `json:"draft_config,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type PaymentThemeVersion struct {
	ID        string    `json:"id"`
	ThemeID   string    `json:"theme_id"`
	Version   int       `json:"version"`
	Status    string    `json:"status"`
	Config    []byte    `json:"config"`
	CreatedAt time.Time `json:"created_at"`
}

type AllowedRedirectURL struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	URL       string    `json:"url"`
	Type      string    `json:"type"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Invoice struct {
	ID                  string        `json:"id"`
	TenantID            string        `json:"tenant_id"`
	MerchantAccountID   string        `json:"merchant_account_id"`
	IdempotencyKey      string        `json:"idempotency_key"`
	RequestedAmount     int64         `json:"requested_amount"`
	Amount              int64         `json:"amount"`
	UniqueAmountCode    int64         `json:"unique_amount_code"`
	QRISTemplateID      string        `json:"qris_template_id,omitempty"`
	QRISMerchantID      string        `json:"qris_merchant_id,omitempty"`
	QRISMerchantName    string        `json:"qris_merchant_name,omitempty"`
	QRISMerchantCity    string        `json:"qris_merchant_city,omitempty"`
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
	SandboxMode         bool          `json:"sandbox_mode"`
}

func (i *Invoice) Transition(next InvoiceStatus, now time.Time) error {
	allowed := (i.Status == InvoiceCreating && (next == InvoicePending || next == InvoiceFailed)) || (i.Status == InvoicePending && (next == InvoicePaid || next == InvoiceExpired || next == InvoiceFailed || next == InvoiceCancelled))
	if !allowed {
		return ErrInvalidTransition
	}
	i.Status, i.UpdatedAt = next, now
	return nil
}

type Tenant struct {
	ID                          string    `json:"id"`
	MerchantID                  string    `json:"merchant_id,omitempty"`
	Name                        string    `json:"name"`
	SiteURL                     string    `json:"site_url,omitempty"`
	CallbackURL                 string    `json:"callback_url,omitempty"`
	WebhookURL                  string    `json:"webhook_url,omitempty"`
	SandboxMode                 bool      `json:"sandbox_mode"`
	UseUniqueAmountCode         bool      `json:"use_unique_amount_code"`
	UniqueAmountCooldownMinutes int       `json:"unique_amount_cooldown_minutes"`
	APIKeyHash                  string    `json:"-"`
	APIKeyCiphertext            string    `json:"-"`
	WebhookSecretCiphertext     string    `json:"-"`
	WebhookSecretConfigured     bool      `json:"webhook_secret_configured"`
	WebhookReplayWindowSeconds  int       `json:"webhook_replay_window_seconds"`
	APIKeyRecoverable           bool      `json:"api_key_recoverable"`
	Active                      bool      `json:"active"`
	CreatedAt                   time.Time `json:"created_at"`
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

type BrowserJob struct {
	ID           string     `json:"id"`
	ResourceKey  string     `json:"resource_key"`
	MerchantID   string     `json:"merchant_id,omitempty"`
	Kind         string     `json:"kind"`
	Priority     int        `json:"priority"`
	State        string     `json:"state"`
	NotBefore    time.Time  `json:"not_before"`
	RequestedAt  time.Time  `json:"requested_at"`
	RequestCount int        `json:"request_count"`
	Attempt      int        `json:"attempt"`
	LeaseOwner   string     `json:"lease_owner,omitempty"`
	LeaseUntil   *time.Time `json:"lease_until,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	LastError    string     `json:"last_error,omitempty"`
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
	IdempotencyKey       string        `json:"idempotency_key,omitempty"`
	QRISTemplateID       string        `json:"qris_template_id"`
	MerchantID           string        `json:"merchant_id,omitempty"`
	TenantID             string        `json:"tenant_id,omitempty"`
	Amount               int64         `json:"amount"`
	PayableAmount        int64         `json:"payable_amount"`
	UniqueAmountCode     int64         `json:"unique_amount_code"`
	DynamicPayload       string        `json:"dynamic_payload"`
	UniqueCode           string        `json:"unique_code"`
	Status               InvoiceStatus `json:"status"`
	RequestSource        string        `json:"request_source"`
	MatchConfidence      string        `json:"match_confidence"`
	MatchedTransactionID string        `json:"matched_transaction_id,omitempty"`
	CreatedAt            time.Time     `json:"created_at"`
	UpdatedAt            time.Time     `json:"updated_at"`
	ExpiresAt            time.Time     `json:"expires_at"`
	LastCheckedAt        *time.Time    `json:"last_checked_at,omitempty"`
	NextCheckAt          *time.Time    `json:"next_check_at"`
	CheckCount           int           `json:"check_count"`
	SandboxMode          bool          `json:"sandbox_mode"`
}

type TenantTransaction struct {
	ID                   string        `json:"id"`
	TenantID             string        `json:"tenant_id"`
	MerchantID           string        `json:"merchant_id,omitempty"`
	Kind                 string        `json:"kind"`
	Mode                 string        `json:"mode"`
	RequestSource        string        `json:"request_source"`
	IdempotencyKey       string        `json:"idempotency_key,omitempty"`
	Amount               int64         `json:"amount"`
	PayableAmount        int64         `json:"payable_amount"`
	UniqueAmountCode     int64         `json:"unique_amount_code"`
	Currency             string        `json:"currency"`
	Status               InvoiceStatus `json:"status"`
	ProviderReference    string        `json:"provider_reference,omitempty"`
	Validation           string        `json:"validation,omitempty"`
	MatchedTransactionID string        `json:"matched_transaction_id,omitempty"`
	CreatedAt            time.Time     `json:"created_at"`
	UpdatedAt            time.Time     `json:"updated_at"`
	ExpiresAt            time.Time     `json:"expires_at"`
	LastCheckedAt        *time.Time    `json:"last_checked_at,omitempty"`
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
