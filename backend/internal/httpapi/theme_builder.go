package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"xloyal/backend/internal/domain"
	"xloyal/backend/internal/store"
)

const maxThemeConfigBytes = 64 * 1024

var themeColor = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

type themeDTO struct {
	ID        string          `json:"id"`
	TenantID  string          `json:"tenant_id"`
	Name      string          `json:"name"`
	Template  string          `json:"template"`
	Status    string          `json:"status"`
	Version   int             `json:"version"`
	IsDefault bool            `json:"is_default"`
	Config    json.RawMessage `json:"config,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

func toThemeDTO(theme domain.PaymentTheme, config []byte) themeDTO {
	var parsed map[string]any
	_ = json.Unmarshal(config, &parsed)
	template, _ := parsed["template_key"].(string)
	return themeDTO{ID: theme.ID, TenantID: theme.TenantID, Name: theme.Name, Template: template, Status: theme.Status, Version: theme.CurrentVersion, IsDefault: theme.IsDefault, Config: config, CreatedAt: theme.CreatedAt, UpdatedAt: theme.UpdatedAt}
}
func themeTenant(r *http.Request) string { return strings.TrimSpace(r.URL.Query().Get("tenant_id")) }
func requireThemeTenant(w http.ResponseWriter, r *http.Request) (string, bool) {
	// Empty tenant_id addresses the global theme library. A tenant query is
	// optional context for listing or editing tenant-scoped overrides.
	return themeTenant(r), true
}

func (s Server) listPaymentThemes(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	tenant, ok := requireThemeTenant(w, r)
	if !ok {
		return
	}
	themes, err := s.Repo.ListPaymentThemes(r.Context(), tenant)
	if err != nil {
		problem(w, 500, "theme list failed")
		return
	}
	result := make([]themeDTO, 0, len(themes))
	for _, theme := range themes {
		result = append(result, toThemeDTO(theme, theme.DraftConfig))
	}
	write(w, 200, result)
}
func (s Server) getPaymentTheme(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	tenant, ok := requireThemeTenant(w, r)
	if !ok {
		return
	}
	theme, err := s.Repo.PaymentTheme(r.Context(), tenant, r.PathValue("id"))
	if err != nil {
		themeError(w, err)
		return
	}
	write(w, 200, toThemeDTO(theme, theme.DraftConfig))
}
func (s Server) createPaymentTheme(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	var input struct {
		Name     string          `json:"name"`
		TenantID string          `json:"tenant_id"`
		Template string          `json:"template"`
		Config   json.RawMessage `json:"config"`
	}
	if !decode(w, r, &input) {
		return
	}
	tenant := strings.TrimSpace(input.TenantID)
	if tenant == "" {
		tenant = themeTenant(r)
	}
	config := input.Config
	if len(config) == 0 {
		config = json.RawMessage(defaultThemeConfig(input.Template))
	}
	if err := validateThemeJSON(config); err != nil {
		problem(w, 400, "theme_invalid: "+err.Error())
		return
	}
	now := time.Now().UTC()
	theme := domain.PaymentTheme{ID: "theme_" + newID()[:16], TenantID: tenant, Name: strings.TrimSpace(input.Name), Status: domain.ThemeDraft, DraftConfig: config, CreatedAt: now, UpdatedAt: now}
	if theme.Name == "" || len(theme.Name) > 120 {
		problem(w, 400, "name must be 1-120 characters")
		return
	}
	if err := s.Repo.CreatePaymentTheme(r.Context(), theme); err != nil {
		problem(w, 500, "theme create failed")
		return
	}
	s.appendThemeAudit(r, theme, "payment_theme.created")
	write(w, 201, toThemeDTO(theme, config))
}
func (s Server) updatePaymentTheme(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	tenant, ok := requireThemeTenant(w, r)
	if !ok {
		return
	}
	var input struct {
		Name   string          `json:"name"`
		Config json.RawMessage `json:"config"`
	}
	if !decode(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 120 {
		problem(w, 400, "name must be 1-120 characters")
		return
	}
	if err := validateThemeJSON(input.Config); err != nil {
		problem(w, 400, "theme_invalid: "+err.Error())
		return
	}
	theme, err := s.Repo.UpdatePaymentThemeDraft(r.Context(), tenant, r.PathValue("id"), input.Name, input.Config, time.Now().UTC())
	if err != nil {
		themeError(w, err)
		return
	}
	s.appendThemeAudit(r, theme, "payment_theme.draft_updated")
	write(w, 200, toThemeDTO(theme, input.Config))
}
func (s Server) publishPaymentTheme(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	tenant, ok := requireThemeTenant(w, r)
	if !ok {
		return
	}
	theme, version, err := s.Repo.PublishPaymentTheme(r.Context(), tenant, r.PathValue("id"), time.Now().UTC())
	if err != nil {
		themeError(w, err)
		return
	}
	s.appendThemeAudit(r, theme, "payment_theme.published")
	write(w, 200, toThemeDTO(theme, version.Config))
}
func (s Server) archivePaymentTheme(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	tenant, ok := requireThemeTenant(w, r)
	if !ok {
		return
	}
	theme, err := s.Repo.ArchivePaymentTheme(r.Context(), tenant, r.PathValue("id"), time.Now().UTC())
	if err != nil {
		themeError(w, err)
		return
	}
	s.appendThemeAudit(r, theme, "payment_theme.archived")
	write(w, 200, toThemeDTO(theme, theme.DraftConfig))
}
func (s Server) setDefaultPaymentTheme(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	tenant, ok := requireThemeTenant(w, r)
	if !ok {
		return
	}
	theme, err := s.Repo.SetDefaultPaymentTheme(r.Context(), tenant, r.PathValue("id"), time.Now().UTC())
	if err != nil {
		themeError(w, err)
		return
	}
	s.appendThemeAudit(r, theme, "payment_theme.default_set")
	write(w, 200, toThemeDTO(theme, theme.DraftConfig))
}
func (s Server) duplicatePaymentTheme(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	var input struct {
		Name     string `json:"name"`
		TenantID string `json:"tenant_id"`
	}
	if !decode(w, r, &input) {
		return
	}
	tenant := strings.TrimSpace(input.TenantID)
	if tenant == "" {
		tenant = themeTenant(r)
	}
	now := time.Now().UTC()
	theme := domain.PaymentTheme{ID: "theme_" + newID()[:16], TenantID: tenant, Name: strings.TrimSpace(input.Name), CreatedAt: now, UpdatedAt: now}
	if theme.Name == "" {
		theme.Name = "Custom theme"
	}
	if len(theme.Name) > 120 {
		problem(w, 400, "name must be 1-120 characters")
		return
	}
	if err := s.Repo.DuplicatePaymentTheme(r.Context(), tenant, r.PathValue("id"), theme); err != nil {
		themeError(w, err)
		return
	}
	created, err := s.Repo.PaymentTheme(r.Context(), tenant, theme.ID)
	if err != nil {
		problem(w, 500, "theme duplicate failed")
		return
	}
	s.appendThemeAudit(r, created, "payment_theme.duplicated")
	write(w, 201, toThemeDTO(created, created.DraftConfig))
}
func (s Server) deletePaymentTheme(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	tenant, ok := requireThemeTenant(w, r)
	if !ok {
		return
	}
	if err := s.Repo.DeletePaymentTheme(r.Context(), tenant, r.PathValue("id")); err != nil {
		themeError(w, err)
		return
	}
	_ = s.Repo.AppendAudit(r.Context(), domain.AuditEvent{ID: newID(), TenantID: tenant, Actor: "admin", Action: "payment_theme.deleted", ResourceType: "payment_theme", ResourceID: r.PathValue("id"), CreatedAt: time.Now().UTC()})
	write(w, 200, map[string]string{"status": "deleted"})
}
func (s Server) appendThemeAudit(r *http.Request, theme domain.PaymentTheme, action string) {
	_ = s.Repo.AppendAudit(r.Context(), domain.AuditEvent{ID: newID(), TenantID: theme.TenantID, Actor: "admin", Action: action, ResourceType: "payment_theme", ResourceID: theme.ID, Metadata: map[string]any{"version": theme.CurrentVersion}, CreatedAt: time.Now().UTC()})
}
func themeError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		problem(w, 404, "not found")
		return
	}
	if errors.Is(err, store.ErrConflict) {
		problem(w, 409, "invalid_state")
		return
	}
	problem(w, 500, "theme operation failed")
}

func validateThemeJSON(raw []byte) error {
	if len(raw) == 0 || len(raw) > maxThemeConfigBytes {
		return errors.New("config must be 1-65536 bytes")
	}
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil || len(root) == 0 {
		return errors.New("invalid JSON")
	}
	allowed := map[string]bool{"schema_version": true, "template_key": true, "branding": true, "tenant_branding": true, "colors": true, "layout": true, "payment_visibility": true, "timer": true, "success_copy": true, "redirect_delay": true}
	for key := range root {
		if !allowed[key] {
			return errors.New("unknown field " + key)
		}
	}
	if version, ok := root["schema_version"].(float64); ok && version != 1 {
		return errors.New("schema_version must be 1")
	}
	template, ok := root["template_key"].(string)
	if !ok || !map[string]bool{"modern": true, "minimal": true, "dark": true, "corporate": true, "compact": true}[template] {
		return errors.New("template_key must be a supported preset")
	}
	if err := validateThemeObject(root, "branding", map[string]func(any) bool{"display_name": safeThemeText, "logo_url": safeLogoURL, "tagline": safeThemeText}); err != nil {
		return err
	}
	if err := validateThemeObject(root, "tenant_branding", map[string]func(any) bool{"display_name": safeThemeText, "logo_url": safeLogoURL, "tagline": safeThemeText, "primary_color": isThemeColor, "favicon_url": safeLogoURL}); err != nil {
		return err
	}
	if err := validateThemeObject(root, "colors", map[string]func(any) bool{"primary": isThemeColor, "background": isThemeColor, "surface": isThemeColor, "text": isThemeColor, "muted": isThemeColor, "success": isThemeColor, "danger": isThemeColor}); err != nil {
		return err
	}
	if err := validateThemeObject(root, "layout", map[string]func(any) bool{"max_width": func(v any) bool { return numberInRange(v, 320, 720) }, "radius": func(v any) bool { return numberInRange(v, 0, 32) }, "density": func(v any) bool { s, ok := v.(string); return ok && (s == "comfortable" || s == "compact") }}); err != nil {
		return err
	}
	if err := validateThemeObject(root, "payment_visibility", map[string]func(any) bool{"show_qr": isBool, "show_amount": isBool, "show_description": isBool, "show_reference": isBool}); err != nil {
		return err
	}
	if err := validateThemeObject(root, "timer", map[string]func(any) bool{"enabled": isBool, "warning_seconds": func(v any) bool { return numberInRange(v, 0, 3600) }}); err != nil {
		return err
	}
	if err := validateThemeObject(root, "success_copy", map[string]func(any) bool{"title": safeThemeText, "message": safeThemeText}); err != nil {
		return err
	}
	if delay, present := root["redirect_delay"]; present && !numberInRange(delay, 0, 30) {
		return errors.New("redirect_delay must be 0-30")
	}
	return nil
}
func validateThemeObject(root map[string]any, name string, fields map[string]func(any) bool) error {
	raw, present := root[name]
	if !present {
		return nil
	}
	object, ok := raw.(map[string]any)
	if !ok {
		return errors.New(name + " must be an object")
	}
	for key, value := range object {
		valid, known := fields[key]
		if !known || !valid(value) {
			return errors.New("invalid " + name + "." + key)
		}
	}
	return nil
}
func safeThemeText(value any) bool {
	text, ok := value.(string)
	return ok && len(text) <= 240 && !strings.ContainsAny(text, "<>")
}
func safeLogoURL(value any) bool {
	raw, ok := value.(string)
	if !ok || len(raw) == 0 || len(raw) > 48*1024 || strings.ContainsAny(raw, "<> ") {
		return false
	}
	if strings.HasPrefix(raw, "https://") {
		return len(raw) <= 2048
	}
	if !strings.HasPrefix(raw, "data:image/") {
		return false
	}
	separator := strings.Index(raw, ";base64,")
	return separator > len("data:image/") && len(raw) > separator+8
}
func isThemeColor(value any) bool {
	color, ok := value.(string)
	return ok && themeColor.MatchString(color)
}
func isBool(value any) bool { _, ok := value.(bool); return ok }
func numberInRange(value any, min, max float64) bool {
	number, ok := value.(float64)
	return ok && number >= min && number <= max && number == float64(int(number))
}
func defaultThemeConfig(template string) string {
	if !map[string]bool{"modern": true, "minimal": true, "dark": true, "corporate": true, "compact": true}[template] {
		template = "modern"
	}
	return `{"schema_version":1,"template_key":"` + template + `","colors":{"primary":"#1A5C55","background":"#F4F6F2","surface":"#FFFFFF","text":"#18231F","muted":"#68766F","success":"#16805B","danger":"#B44444"},"timer":{"enabled":true,"warning_seconds":120},"success_copy":{"title":"Pembayaran Berhasil","message":"Pembayaran telah diterima."},"redirect_delay":5}`
}
