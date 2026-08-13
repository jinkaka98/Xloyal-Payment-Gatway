package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"xloyal/backend/internal/domain"
)

type OpenAPIConfig struct {
	BaseURL    string       `json:"base_url"`
	CreatePath string       `json:"create_path,omitempty"`
	CheckPath  string       `json:"check_path,omitempty"`
	MerchantID string       `json:"merchant_id"`
	APIKey     string       `json:"api_key"`
	Client     *http.Client `json:"-"`
}

const InteractiveQRISProvider = "interactive_qris"
type OpenAPI struct{ cfg OpenAPIConfig }

func NewOpenAPI(cfg OpenAPIConfig) (*OpenAPI, error) {
	base, err := url.ParseRequestURI(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("provider base URL: %w", err)
	}
	if base.Scheme != "https" && cfg.Client == nil {
		return nil, errors.New("provider base URL must use HTTPS")
	}
	if cfg.Client == nil && !strings.EqualFold(base.Hostname(), "qris.interactive.co.id") {
		return nil, errors.New("provider host must be qris.interactive.co.id")
	}
	if base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, errors.New("provider base URL must contain only scheme, host and optional path")
	}
	if strings.TrimSpace(cfg.MerchantID) == "" || strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("provider merchant ID and API key are required")
	}
	if cfg.CreatePath == "" {
		cfg.CreatePath = "/restapi/qris/show_qris.php"
	}
	if cfg.CheckPath == "" {
		cfg.CheckPath = "/restapi/qris/checkpaid_qris.php"
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{
			// InterActive documents normal check-status responses of 6–30s;
			// keep headroom above that so valid responses are not cut off.
			Timeout: 35 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	return &OpenAPI{cfg: cfg}, nil
}
func (p *OpenAPI) CreatePayment(ctx context.Context, req domain.CreatePaymentRequest) (domain.CreatePaymentResult, error) {
	var out struct {
		Status string `json:"status"`
		Data   struct {
			QRPayload   string `json:"qris_content"`
			RequestDate string `json:"qris_request_date"`
			InvoiceID   string `json:"qris_invoiceid"`
			NMID        string `json:"qris_nmid"`
		} `json:"data"`
	}
	q := url.Values{"do": {"create-invoice"}, "apikey": {p.cfg.APIKey}, "mID": {p.cfg.MerchantID}, "cliTrxNumber": {req.InvoiceID}, "cliTrxAmount": {strconv.FormatInt(req.Amount, 10)}, "useTip": {"no"}}
	if err := p.do(ctx, p.cfg.CreatePath, q, &out); err != nil {
		return domain.CreatePaymentResult{}, err
	}
	if strings.ToLower(out.Status) != "success" || out.Data.InvoiceID == "" || !validQRISPayload(out.Data.QRPayload) || out.Data.RequestDate == "" {
		return domain.CreatePaymentResult{}, errors.New("provider returned incomplete payment")
	}
	return domain.CreatePaymentResult{ProviderReference: out.Data.InvoiceID, QRPayload: out.Data.QRPayload, ProviderRequestDate: out.Data.RequestDate}, nil
}
func (p *OpenAPI) CheckPayment(ctx context.Context, req domain.CheckPaymentRequest) (domain.CheckPaymentResult, error) {
	var out struct {
		Status string `json:"status"`
		Data   struct {
			QRISStatus string `json:"qris_status"`
		} `json:"data"`
	}
	q := url.Values{"do": {"checkStatus"}, "apikey": {p.cfg.APIKey}, "mID": {p.cfg.MerchantID}, "invid": {req.ProviderInvoiceID}, "trxvalue": {strconv.FormatInt(req.Amount, 10)}, "trxdate": {normalizeRequestDate(req.RequestDate)}}
	if err := p.do(ctx, p.cfg.CheckPath, q, &out); err != nil {
		return domain.CheckPaymentResult{}, err
	}
	// InterActive returns top-level "failed" for both genuine failures and the
	// still-pending "unpaid" state, so the mapping must key off data.qris_status.
	switch strings.ToLower(out.Data.QRISStatus) {
	case "pending", "unpaid":
		return domain.CheckPaymentResult{Status: domain.InvoicePending}, nil
	case "paid", "success", "settled":
		return domain.CheckPaymentResult{Status: domain.InvoicePaid}, nil
	case "expired":
		return domain.CheckPaymentResult{Status: domain.InvoiceExpired}, nil
	case "failed", "cancelled":
		return domain.CheckPaymentResult{Status: domain.InvoiceFailed}, nil
	default:
		return domain.CheckPaymentResult{}, fmt.Errorf("unknown provider status %q (top-level %q)", out.Data.QRISStatus, out.Status)
	}
}

func normalizeRequestDate(raw string) string {
	raw = strings.TrimSpace(raw)
	// Create-invoice returns "YYYY-mm-dd HH:mm:ss"; check-status requires only
	// the "YYYY-mm-dd" date component.
	if len(raw) >= 10 {
		raw = raw[:10]
	}
	return raw
}
func (p *OpenAPI) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	q := url.Values{"do": {"checkStatus"}, "apikey": {p.cfg.APIKey}, "mID": {p.cfg.MerchantID}, "invid": {"healthcheck"}, "trxvalue": {"1"}, "trxdate": {normalizeRequestDate(time.Now().UTC().Format("2006-01-02"))}}
	target, err := p.endpoint(p.cfg.CheckPath, q)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	res, err := p.cfg.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("provider HTTP status %d", res.StatusCode)
	}
	var out struct {
		Status string `json:"status"`
		Data   struct {
			QRISStatus string `json:"qris_status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out); err != nil {
		return fmt.Errorf("provider health response is not JSON: %w", err)
	}
	if strings.TrimSpace(out.Status) == "" {
		return errors.New("provider health response missing status")
	}
	// A reachable endpoint that still speaks the documented check-status schema
	// is healthy. An empty or unknown qris_status means the request was not
	// accepted (for example invalid credentials), which must fail the check.
	switch strings.ToLower(out.Data.QRISStatus) {
	case "pending", "unpaid", "paid", "success", "settled", "expired", "failed", "cancelled":
		return nil
	default:
		return fmt.Errorf("provider health returned unknown status %q", out.Data.QRISStatus)
	}
}
func (p *OpenAPI) do(ctx context.Context, path string, q url.Values, output any) error {
	target, err := p.endpoint(path, q)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	res, err := p.cfg.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("provider HTTP status %d", res.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(output)
}

func (p *OpenAPI) endpoint(path string, q url.Values) (string, error) {
	base, err := url.Parse(p.cfg.BaseURL)
	if err != nil {
		return "", err
	}
	rel, err := url.Parse(path)
	if err != nil || rel.IsAbs() || rel.Host != "" {
		return "", errors.New("provider path must be relative")
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/" + strings.TrimLeft(rel.Path, "/")
	base.RawQuery = q.Encode()
	return base.String(), nil
}

func validQRISPayload(payload string) bool {
	return len(payload) >= 12 && len(payload) <= 4096 && strings.HasPrefix(payload, "000201")
}
