package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"xloyal/backend/internal/domain"
)

func (m *Memory) ListPaymentThemes(_ context.Context, tenant string) ([]domain.PaymentTheme, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []domain.PaymentTheme{}
	for _, v := range m.themes {
		if v.TenantID == tenant || v.TenantID == "" {
			out = append(out, v)
		}
	}
	return out, nil
}
func (m *Memory) PaymentTheme(_ context.Context, tenant, id string) (domain.PaymentTheme, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.themes[id]
	if !ok || (v.TenantID != tenant && v.TenantID != "") {
		return domain.PaymentTheme{}, ErrNotFound
	}
	return v, nil
}
func (m *Memory) CreatePaymentTheme(_ context.Context, v domain.PaymentTheme) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.themes[v.ID]; ok {
		return ErrConflict
	}
	m.themes[v.ID] = v
	return nil
}
func (m *Memory) UpdatePaymentThemeDraft(_ context.Context, tenant, id, name string, cfg []byte, now time.Time) (domain.PaymentTheme, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.themes[id]
	if !ok || (tenant != "" && v.TenantID != tenant) || (tenant == "" && v.TenantID != "") {
		return v, ErrNotFound
	}
	if tenant != "" && v.TenantID != tenant {
		return v, ErrNotFound
	}
	if !validThemeConfig(cfg) {
		return v, ErrConflict
	}
	v.Name, v.DraftConfig, v.UpdatedAt = name, append([]byte(nil), cfg...), now
	if v.Status == domain.ThemeArchived {
		v.Status = domain.ThemeDraft
	}
	m.themes[id] = v
	return v, nil
}
func (m *Memory) PublishPaymentTheme(_ context.Context, tenant, id string, now time.Time) (domain.PaymentTheme, domain.PaymentThemeVersion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.themes[id]
	if !ok || (tenant != "" && v.TenantID != tenant) || (tenant == "" && v.TenantID != "") {
		return v, domain.PaymentThemeVersion{}, ErrNotFound
	}
	if !validThemeConfig(v.DraftConfig) {
		return v, domain.PaymentThemeVersion{}, ErrConflict
	}
	v.CurrentVersion++
	v.Status = domain.ThemePublished
	v.UpdatedAt = now
	version := domain.PaymentThemeVersion{ID: fmt.Sprintf("%s-v%d", id, v.CurrentVersion), ThemeID: id, Version: v.CurrentVersion, Status: domain.ThemePublished, Config: append([]byte(nil), v.DraftConfig...), CreatedAt: now}
	m.themes[id] = v
	m.themeVersions[id+":"+fmt.Sprint(version.Version)] = version
	return v, version, nil
}
func (m *Memory) ArchivePaymentTheme(_ context.Context, tenant, id string, now time.Time) (domain.PaymentTheme, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.themes[id]
	if !ok || (tenant != "" && v.TenantID != tenant) || (tenant == "" && v.TenantID != "") {
		return v, ErrNotFound
	}
	if v.IsDefault {
		return v, ErrConflict
	}
	v.Status = domain.ThemeArchived
	v.UpdatedAt = now
	m.themes[id] = v
	return v, nil
}
func (m *Memory) SetDefaultPaymentTheme(_ context.Context, tenant, id string, now time.Time) (domain.PaymentTheme, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.themes[id]
	if !ok || (tenant != "" && v.TenantID != tenant) || (tenant == "" && v.TenantID != "") {
		return v, ErrNotFound
	}
	if v.Status != domain.ThemePublished {
		return v, ErrConflict
	}
	for k, x := range m.themes {
		if x.TenantID == tenant {
			x.IsDefault = false
			m.themes[k] = x
		}
	}
	v.IsDefault = true
	v.UpdatedAt = now
	m.themes[id] = v
	return v, nil
}
func (m *Memory) DuplicatePaymentTheme(_ context.Context, tenant, source string, v domain.PaymentTheme) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	src, ok := m.themes[source]
	if !ok || (src.TenantID != tenant && src.TenantID != "") {
		return ErrNotFound
	}
	if _, ok = m.themes[v.ID]; ok {
		return ErrConflict
	}
	cfg := src.DraftConfig
	if len(cfg) == 0 {
		for _, pv := range m.themeVersions {
			if pv.ThemeID == source && pv.Version == src.CurrentVersion {
				cfg = pv.Config
			}
		}
	}
	v.TenantID, v.Status, v.IsDefault, v.DraftConfig = tenant, domain.ThemeDraft, false, append([]byte(nil), cfg...)
	m.themes[v.ID] = v
	return nil
}
func (m *Memory) DeletePaymentTheme(_ context.Context, tenant, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.themes[id]
	if !ok || (tenant != "" && v.TenantID != tenant) || (tenant == "" && v.TenantID != "") {
		return ErrNotFound
	}
	if v.IsDefault || v.Status == domain.ThemePublished {
		return ErrConflict
	}
	delete(m.themes, id)
	return nil
}
func validThemeConfig(cfg []byte) bool {
	if len(cfg) == 0 || len(cfg) > 65536 {
		return false
	}
	var v map[string]any
	if json.Unmarshal(cfg, &v) != nil {
		return false
	}
	_, ok := v["template_key"].(string)
	return ok
}
