package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"xloyal/backend/internal/domain"
	"xloyal/backend/internal/security"
	"xloyal/backend/internal/store"
)

const (
	DefaultTimeout   = 10 * time.Second
	DefaultLease     = time.Minute
	DefaultBatchSize = 50
	MaxAttempts      = 12
)

var retryableStatus = map[int]bool{408: true, 429: true, 500: true, 502: true, 503: true, 504: true}

type Delivery struct {
	Repo       store.Repository
	Cipher     *security.Cipher
	HTTPClient *http.Client
	Now        func() time.Time
	Logger     *slog.Logger
	Owner      string
	Timeout    time.Duration
	AllowHTTP  bool
}

type Envelope struct {
	EventID   string       `json:"event_id"`
	Event     string       `json:"event"`
	Timestamp string       `json:"timestamp"`
	Data      EnvelopeData `json:"data"`
}

type EnvelopeData struct {
	PaymentSessionID string    `json:"payment_session_id"`
	InvoiceID        string    `json:"invoice_id"`
	Status           string    `json:"status"`
	Amount           int64     `json:"amount"`
	Currency         string    `json:"currency"`
	PaidAt           time.Time `json:"paid_at,omitempty"`
}

func (d Delivery) DispatchOnce(ctx context.Context) error {
	now := d.now()
	owner := d.Owner
	if owner == "" {
		owner = "xloyal-webhook"
	}
	items, err := d.Repo.ClaimWebhookDeliveries(ctx, owner, now, DefaultLease, DefaultBatchSize)
	if err != nil {
		return err
	}
	for _, item := range items {
		err = d.deliver(ctx, item, now)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d Delivery) EnqueueForEvent(ctx context.Context, event domain.PaymentEvent) error {
	tenant, err := d.Repo.Tenant(ctx, event.TenantID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(tenant.WebhookURL) == "" {
		return nil
	}
	now := d.now()
	payload, err := d.payload(ctx, event)
	if err != nil {
		return err
	}
	return d.Repo.CreateWebhookDelivery(ctx, domain.WebhookDelivery{ID: event.EventID + "-" + shortEndpoint(tenant.WebhookURL), TenantID: event.TenantID, EventID: event.EventID, EventType: event.EventType, PaymentSessionID: event.PaymentSessionID, InvoiceID: event.InvoiceID, Endpoint: tenant.WebhookURL, Payload: payload, Status: domain.WebhookDeliveryPending, NextAttemptAt: now, CreatedAt: now, UpdatedAt: now})
}

func (d Delivery) deliver(ctx context.Context, item domain.WebhookDelivery, now time.Time) error {
	tenant, err := d.Repo.Tenant(ctx, item.TenantID)
	if err != nil {
		return d.fail(ctx, item, now, 0, "tenant lookup failed")
	}
	if err = ValidateEndpoint(ctx, item.Endpoint, tenant.SandboxMode || d.AllowHTTP); err != nil {
		return d.fail(ctx, item, now, 0, err.Error())
	}
	secret, err := d.secret(tenant)
	if err != nil {
		return d.fail(ctx, item, now, 0, "webhook secret unavailable")
	}
	stamp := strconv.FormatInt(now.Unix(), 10)
	signature := Sign(secret, stamp, item.Payload)
	timeout := d.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	client := d.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, item.Endpoint, strings.NewReader(string(item.Payload)))
	if err != nil {
		return d.fail(ctx, item, now, 0, "request construction failed")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Xloyal-Event", item.EventType)
	req.Header.Set("X-Xloyal-Event-ID", item.EventID)
	req.Header.Set("X-Xloyal-Timestamp", stamp)
	req.Header.Set("X-Xloyal-Signature", "sha256="+signature)
	response, err := client.Do(req)
	if err != nil {
		return d.retryOrFail(ctx, item, now, 0, err.Error())
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode >= 200 && response.StatusCode <= 299 {
		d.log("webhook delivered", item, response.StatusCode)
		return d.Repo.MarkWebhookDelivered(ctx, item.ID, d.owner(), now, response.StatusCode)
	}
	message := fmt.Sprintf("webhook response status %d", response.StatusCode)
	if retryableStatus[response.StatusCode] {
		d.log("webhook retry scheduled", item, response.StatusCode)
		return d.retryOrFail(ctx, item, now, response.StatusCode, message)
	}
	d.log("webhook permanently failed", item, response.StatusCode)
	return d.fail(ctx, item, now, response.StatusCode, message)
}

func (d Delivery) retryOrFail(ctx context.Context, item domain.WebhookDelivery, now time.Time, status int, message string) error {
	if item.AttemptCount >= MaxAttempts {
		return d.fail(ctx, item, now, status, message)
	}
	return d.Repo.MarkWebhookRetry(ctx, item.ID, d.owner(), now, jitteredBackoff(item.AttemptCount), status, truncate(message))
}

func (d Delivery) fail(ctx context.Context, item domain.WebhookDelivery, now time.Time, status int, message string) error {
	return d.Repo.MarkWebhookFailed(ctx, item.ID, d.owner(), now, status, truncate(message))
}

func (d Delivery) payload(ctx context.Context, event domain.PaymentEvent) ([]byte, error) {
	invoice, err := d.Repo.Invoice(ctx, event.TenantID, event.InvoiceID)
	if err != nil {
		return nil, err
	}
	status := strings.TrimPrefix(event.EventType, "payment.")
	envelope := Envelope{EventID: event.EventID, Event: event.EventType, Timestamp: event.OccurredAt.UTC().Format(time.RFC3339), Data: EnvelopeData{PaymentSessionID: event.PaymentSessionID, InvoiceID: event.InvoiceID, Status: status, Amount: invoice.Amount, Currency: invoice.Currency}}
	if invoice.Status == domain.InvoicePaid {
		envelope.Data.PaidAt = invoice.UpdatedAt
	}
	return json.Marshal(envelope)
}

func (d Delivery) secret(tenant domain.Tenant) ([]byte, error) {
	if tenant.WebhookSecretCiphertext == "" {
		return nil, errors.New("webhook secret is not configured")
	}
	if d.Cipher == nil {
		return nil, errors.New("credential cipher is not configured")
	}
	return d.Cipher.Decrypt(tenant.WebhookSecretCiphertext)
}

func (d Delivery) owner() string {
	if d.Owner == "" {
		return "xloyal-webhook"
	}
	return d.Owner
}

func (d Delivery) log(message string, item domain.WebhookDelivery, statusCode int) {
	if d.Logger != nil {
		d.Logger.Info(message, "tenant_id", item.TenantID, "event_id", item.EventID, "payment_session_id", item.PaymentSessionID, "invoice_id", item.InvoiceID, "delivery_id", item.ID, "attempt", item.AttemptCount, "status_code", statusCode, "delivery_status", item.Status)
	}
}
func (d Delivery) now() time.Time {
	if d.Now != nil {
		return d.Now().UTC()
	}
	return time.Now().UTC()
}
func shortEndpoint(endpoint string) string {
	sum := sha256.Sum256([]byte(endpoint))
	return hex.EncodeToString(sum[:])[:12]
}
func truncate(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 500 {
		return value[:500]
	}
	return value
}

func Sign(secret []byte, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(timestamp + "."))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func VerifySignature(secret []byte, timestamp string, body []byte, signature string, now time.Time, window time.Duration) bool {
	parsed, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	if window <= 0 {
		window = 5 * time.Minute
	}
	if delta := now.Unix() - parsed; delta > int64(window/time.Second) || delta < -int64(window/time.Second) {
		return false
	}
	provided := strings.TrimPrefix(signature, "sha256=")
	expected := Sign(secret, timestamp, body)
	return hmac.Equal([]byte(strings.ToLower(provided)), []byte(expected))
}

func Backoff(attempt int) time.Duration {
	values := []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 5 * time.Minute, 10 * time.Minute, 30 * time.Minute}
	if attempt < 1 {
		attempt = 1
	}
	if attempt > len(values) {
		attempt = len(values)
	}
	return values[attempt-1]
}

func jitteredBackoff(attempt int) time.Duration {
	base := Backoff(attempt)
	// Add bounded +/-20% jitter so multiple tenants do not synchronize retries.
	n, err := rand.Int(rand.Reader, big.NewInt(41))
	if err != nil {
		return base
	}
	percent := int64(80) + n.Int64()
	return time.Duration(int64(base) * percent / 100)
}

func ValidateEndpoint(ctx context.Context, raw string, allowHTTP bool) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.User != nil || u.Hostname() == "" {
		return errors.New("invalid webhook endpoint")
	}
	if u.Scheme != "https" && !(allowHTTP && u.Scheme == "http") {
		return errors.New("webhook endpoint must use HTTPS")
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "localhost" || host == "metadata.google.internal" || host == "169.254.169.254" {
		return errors.New("webhook endpoint targets a reserved host")
	}
	ips := net.ParseIP(host)
	if ips != nil {
		if privateIP(ips) {
			return errors.New("webhook endpoint targets a private address")
		}
		return nil
	}
	resolved, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return errors.New("webhook endpoint DNS lookup failed")
	}
	for _, ip := range resolved {
		if privateIP(ip.IP) {
			return errors.New("webhook endpoint resolves to a private address")
		}
	}
	return nil
}

func privateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		return ip4[0] == 0 || (ip4[0] == 169 && ip4[1] == 254)
	}
	return ip.IsUnspecified()
}
