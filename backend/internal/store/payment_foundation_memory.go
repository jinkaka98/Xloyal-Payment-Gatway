package store

import (
	"context"
	"fmt"
	"sort"
	"time"

	"xloyal/backend/internal/domain"
)

func (m *Memory) CreatePaymentSession(_ context.Context, session domain.PaymentSession, event domain.PaymentEvent, outbox domain.OutboxEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	invoice, ok := m.invoices[session.InvoiceID]
	if !ok || invoice.TenantID != session.TenantID {
		return ErrNotFound
	}
	if invoice.Status != domain.InvoicePending || session.ExpiresAt.After(invoice.ExpiresAt) {
		return ErrConflict
	}
	if session.Status != domain.PaymentSessionOpen || event.ID == "" || event.EventID == "" || !domain.IsPaymentEventType(event.EventType) || outbox.ID == "" {
		return ErrConflict
	}
	if (session.ThemeID == "") != (session.ThemeVersion == 0) {
		return ErrConflict
	}
	if session.ThemeID != "" {
		theme, themeOK := m.themes[session.ThemeID]
		version, versionOK := m.themeVersions[session.ThemeID+":"+fmt.Sprint(session.ThemeVersion)]
		themeAllowed := theme.TenantID == session.TenantID || (theme.TenantID == "" && theme.IsDefault)
		if !themeOK || !versionOK || !themeAllowed || theme.Status != domain.ThemePublished || version.Status != domain.ThemePublished {
			return ErrConflict
		}
	}
	redirects := []struct {
		kind string
		url  string
	}{
		{domain.RedirectSuccess, session.ReturnURL},
		{domain.RedirectSuccess, session.SuccessURL},
		{domain.RedirectCancel, session.CancelURL},
		{domain.RedirectFailed, session.FailedURL},
		{domain.RedirectExpired, session.ExpiredURL},
	}
	for _, redirect := range redirects {
		if redirect.url == "" {
			continue
		}
		allowed := false
		for _, configured := range m.redirectURLs {
			if configured.TenantID == session.TenantID && configured.URL == redirect.url && configured.Type == redirect.kind && configured.Active {
				allowed = true
				break
			}
		}
		if !allowed {
			return ErrConflict
		}
	}
	event.TenantID, event.InvoiceID, event.PaymentSessionID = session.TenantID, session.InvoiceID, session.ID
	if event.SequenceNumber == 0 {
		event.SequenceNumber = 1
	}
	outbox.EventID, outbox.TenantID, outbox.EventType, outbox.AggregateType, outbox.AggregateID = event.EventID, session.TenantID, event.EventType, "payment_session", session.ID
	if outbox.Status == "" {
		outbox.Status = domain.OutboxPending
	}
	if _, ok := m.paymentSessions[session.ID]; ok {
		return ErrConflict
	}
	for _, existing := range m.paymentSessions {
		if existing.TenantID == session.TenantID && existing.InvoiceID == session.InvoiceID &&
			(existing.Status == domain.PaymentSessionOpen || existing.Status == domain.PaymentSessionPaymentPending) {
			return ErrConflict
		}
	}
	if _, ok := m.outboxEvents[outbox.ID]; ok {
		return ErrConflict
	}
	for _, existing := range m.paymentEvents {
		if existing.EventID == event.EventID {
			return ErrConflict
		}
	}
	for _, existing := range m.paymentSessions {
		if existing.PublicTokenHash == session.PublicTokenHash {
			return ErrConflict
		}
	}
	m.paymentSessions[session.ID] = session
	m.paymentEvents = append(m.paymentEvents, event)
	m.outboxEvents[outbox.ID] = outbox
	return nil
}

func (m *Memory) PaymentSession(_ context.Context, tenantID, id string) (domain.PaymentSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.paymentSessions[id]
	if !ok || v.TenantID != tenantID {
		return domain.PaymentSession{}, ErrNotFound
	}
	return v, nil
}

func (m *Memory) PaymentSessionByTokenHash(_ context.Context, hash string) (domain.PaymentSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.paymentSessions {
		if v.PublicTokenHash == hash {
			return v, nil
		}
	}
	return domain.PaymentSession{}, ErrNotFound
}

func (m *Memory) TransitionPaymentSession(_ context.Context, tenantID, id string, next domain.PaymentSessionStatus, now time.Time, event domain.PaymentEvent, outbox domain.OutboxEvent) (domain.PaymentSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.paymentSessions[id]
	if !ok || v.TenantID != tenantID {
		return domain.PaymentSession{}, ErrNotFound
	}
	if !v.Status.CanTransition(next) {
		return v, ErrConflict
	}
	expectedEventType, ok := v.Status.PaymentEventTypeForTransition(next)
	if !ok || event.EventType != expectedEventType {
		return v, ErrConflict
	}
	if _, exists := m.outboxEvents[outbox.ID]; exists {
		return v, ErrConflict
	}
	for _, existing := range m.paymentEvents {
		if existing.EventID == event.EventID {
			return v, ErrConflict
		}
	}
	if event.EventID == "" || outbox.ID == "" {
		return v, ErrConflict
	}
	event.TenantID, event.InvoiceID, event.PaymentSessionID = tenantID, v.InvoiceID, id
	outbox.EventID, outbox.TenantID, outbox.EventType, outbox.AggregateType, outbox.AggregateID = event.EventID, tenantID, event.EventType, "payment_session", id
	if outbox.Status == "" {
		outbox.Status = domain.OutboxPending
	}
	invoiceStatus, terminal := next.InvoiceTerminalStatus()
	if terminal {
		invoice, exists := m.invoices[v.InvoiceID]
		if !exists || invoice.TenantID != tenantID || (invoice.Status != domain.InvoicePending && invoice.Status != invoiceStatus) {
			return v, ErrConflict
		}
		invoice.Status, invoice.UpdatedAt = invoiceStatus, now
		m.invoices[v.InvoiceID] = invoice
		m.cooldownHostedInvoiceLocked(v.InvoiceID, invoiceStatus, now)
	}
	if event.SequenceNumber == 0 {
		event.SequenceNumber = 1
		for _, existing := range m.paymentEvents {
			if existing.PaymentSessionID == id && existing.SequenceNumber >= event.SequenceNumber {
				event.SequenceNumber = existing.SequenceNumber + 1
			}
		}
	}
	v.Status, v.UpdatedAt = next, now
	m.paymentSessions[id] = v
	m.paymentEvents = append(m.paymentEvents, event)
	m.outboxEvents[outbox.ID] = outbox
	return v, nil
}

func (m *Memory) cooldownHostedInvoiceLocked(invoiceID string, status domain.InvoiceStatus, now time.Time) {
	for key, reservation := range m.hostedAmountReservations {
		if reservation.PaymentID != invoiceID || reservation.State == "cooldown" {
			continue
		}
		reservation.State = "cooldown"
		reservation.TerminalStatus = string(status)
		reservation.TerminalAt = now
		reservation.CooldownUntil = now.Add(time.Duration(reservation.CooldownMinutes) * time.Minute)
		m.hostedAmountReservations[key] = reservation
	}
}

func (m *Memory) ClaimOutboxEvents(_ context.Context, owner string, now time.Time, lease time.Duration, limit int) ([]domain.OutboxEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, ev := range m.outboxEvents {
		if ev.Status == "PROCESSING" && ev.LockedAt != nil && !ev.LockedAt.Add(lease).After(now) {
			ev.Status, ev.LockedBy, ev.LockedAt = "PENDING", "", nil
			m.outboxEvents[id] = ev
		}
	}
	ids := make([]string, 0)
	for id, ev := range m.outboxEvents {
		if ev.Status == "PENDING" && !ev.NextAttemptAt.After(now) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return m.outboxEvents[ids[i]].CreatedAt.Before(m.outboxEvents[ids[j]].CreatedAt) })
	if len(ids) > limit {
		ids = ids[:limit]
	}
	locked := now
	out := make([]domain.OutboxEvent, 0, len(ids))
	for _, id := range ids {
		ev := m.outboxEvents[id]
		ev.Status, ev.LockedBy, ev.LockedAt, ev.AttemptCount = "PROCESSING", owner, &locked, ev.AttemptCount+1
		m.outboxEvents[id] = ev
		out = append(out, ev)
	}
	return out, nil
}

func (m *Memory) CompleteOutboxEvent(_ context.Context, id, owner string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ev, ok := m.outboxEvents[id]
	if !ok {
		return ErrNotFound
	}
	if ev.Status != "PROCESSING" || ev.LockedBy != owner {
		return ErrConflict
	}
	ev.Status, ev.ProcessedAt, ev.LockedBy, ev.LockedAt = "DELIVERED", &now, "", nil
	m.outboxEvents[id] = ev
	return nil
}

func (m *Memory) FailOutboxEvent(_ context.Context, id, errText string, now time.Time, retry time.Duration, owner string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ev, ok := m.outboxEvents[id]
	if !ok {
		return ErrNotFound
	}
	if ev.Status != "PROCESSING" || ev.LockedBy != owner {
		return ErrConflict
	}
	ev.Status, ev.LastError, ev.NextAttemptAt, ev.LockedBy, ev.LockedAt = "PENDING", errText, now.Add(retry), "", nil
	m.outboxEvents[id] = ev
	return nil
}

func (m *Memory) RedirectURLAllowed(_ context.Context, tenantID, url, kind string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.redirectURLs {
		if v.TenantID == tenantID && v.URL == url && v.Type == kind && v.Active {
			return true, nil
		}
	}
	return false, nil
}

func (m *Memory) PublishedPaymentThemeVersion(_ context.Context, tenantID, themeID string, version int) (domain.PaymentThemeVersion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	theme, ok := m.themes[themeID]
	if !ok || theme.TenantID != tenantID || theme.Status != "PUBLISHED" {
		return domain.PaymentThemeVersion{}, ErrNotFound
	}
	if version > 0 {
		v, ok := m.themeVersions[themeID+":"+fmt.Sprint(version)]
		if !ok || v.Status != "PUBLISHED" {
			return domain.PaymentThemeVersion{}, ErrNotFound
		}
		return v, nil
	}
	var latest domain.PaymentThemeVersion
	for _, candidate := range m.themeVersions {
		if candidate.ThemeID == themeID && candidate.Status == domain.ThemePublished && candidate.Version > latest.Version {
			latest = candidate
		}
	}
	if latest.ID == "" {
		return domain.PaymentThemeVersion{}, ErrNotFound
	}
	return latest, nil
}

func (m *Memory) DefaultPublishedPaymentThemeVersion(_ context.Context, tenantID string) (domain.PaymentThemeVersion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	find := func(scope string) domain.PaymentThemeVersion {
		var selected domain.PaymentThemeVersion
		for _, theme := range m.themes {
			if theme.TenantID != scope || !theme.IsDefault || theme.Status != domain.ThemePublished {
				continue
			}
			for _, version := range m.themeVersions {
				if version.ThemeID == theme.ID && version.Status == domain.ThemePublished && version.Version > selected.Version {
					selected = version
				}
			}
		}
		return selected
	}
	selected := find(tenantID)
	if selected.ID == "" {
		selected = find("")
	}
	if selected.ID == "" {
		return domain.PaymentThemeVersion{}, ErrNotFound
	}
	return selected, nil
}

func (m *Memory) PaymentThemeVersion(_ context.Context, themeID string, version int) (domain.PaymentThemeVersion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.themeVersions[themeID+":"+fmt.Sprint(version)]
	if !ok || (v.Status != domain.ThemePublished && v.Status != domain.ThemeArchived) {
		return domain.PaymentThemeVersion{}, ErrNotFound
	}
	return v, nil
}

// PutPaymentThemeVersion seeds the in-memory repository used by foundation tests.
func (m *Memory) PutPaymentThemeVersion(theme domain.PaymentTheme, version domain.PaymentThemeVersion) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.themes[theme.ID] = theme
	m.themeVersions[theme.ID+":"+fmt.Sprint(version.Version)] = version
}

// PutAllowedRedirectURL seeds the in-memory repository used by foundation tests.
func (m *Memory) PutAllowedRedirectURL(v domain.AllowedRedirectURL) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.redirectURLs[v.ID] = v
}

func (m *Memory) TouchPaymentSession(_ context.Context, tenantID, id string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.paymentSessions[id]
	if !ok || v.TenantID != tenantID {
		return ErrNotFound
	}
	v.LastSeenAt = &now
	v.UpdatedAt = now
	m.paymentSessions[id] = v
	return nil
}

func (m *Memory) CreatePaymentEvent(_ context.Context, event domain.PaymentEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.paymentSessions[event.PaymentSessionID]
	if !ok {
		return ErrNotFound
	}
	if event.ID == "" || event.EventID == "" || !domain.IsPaymentEventType(event.EventType) {
		return ErrConflict
	}
	if event.TenantID != "" && event.TenantID != session.TenantID {
		return ErrConflict
	}
	if event.InvoiceID != "" && event.InvoiceID != session.InvoiceID {
		return ErrConflict
	}
	event.TenantID, event.InvoiceID = session.TenantID, session.InvoiceID
	for _, existing := range m.paymentEvents {
		if existing.EventID == event.EventID {
			return ErrConflict
		}
	}
	if event.SequenceNumber == 0 {
		event.SequenceNumber = 1
		for _, existing := range m.paymentEvents {
			if existing.PaymentSessionID == event.PaymentSessionID && existing.SequenceNumber >= event.SequenceNumber {
				event.SequenceNumber = existing.SequenceNumber + 1
			}
		}
	}
	m.paymentEvents = append(m.paymentEvents, event)
	return nil
}

func (m *Memory) PaymentEvents(_ context.Context, tenantID, sessionID string) ([]domain.PaymentEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.PaymentEvent, 0)
	for _, event := range m.paymentEvents {
		if event.TenantID == tenantID && event.PaymentSessionID == sessionID {
			out = append(out, event)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SequenceNumber < out[j].SequenceNumber })
	return out, nil
}

func (m *Memory) CreateOutboxEvent(_ context.Context, event domain.OutboxEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var paymentEvent domain.PaymentEvent
	for _, candidate := range m.paymentEvents {
		if candidate.EventID == event.EventID {
			paymentEvent = candidate
			break
		}
	}
	if paymentEvent.EventID == "" {
		return ErrNotFound
	}
	if (event.TenantID != "" && event.TenantID != paymentEvent.TenantID) ||
		(event.EventType != "" && event.EventType != paymentEvent.EventType) ||
		(event.AggregateType != "" && event.AggregateType != "payment_session") ||
		(event.AggregateID != "" && event.AggregateID != paymentEvent.PaymentSessionID) {
		return ErrConflict
	}
	event.TenantID, event.EventType, event.AggregateType, event.AggregateID = paymentEvent.TenantID, paymentEvent.EventType, "payment_session", paymentEvent.PaymentSessionID
	if _, ok := m.outboxEvents[event.ID]; ok {
		return ErrConflict
	}
	m.outboxEvents[event.ID] = event
	return nil
}

func (m *Memory) MarkOutboxDelivered(ctx context.Context, id, owner string, now time.Time) error {
	return m.CompleteOutboxEvent(ctx, id, owner, now)
}
func (m *Memory) MarkOutboxRetry(ctx context.Context, id, owner string, now time.Time, retry time.Duration, errText string) error {
	return m.FailOutboxEvent(ctx, id, errText, now, retry, owner)
}
func (m *Memory) MarkOutboxFailed(_ context.Context, id, owner string, now time.Time, errText string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ev, ok := m.outboxEvents[id]
	if !ok {
		return ErrNotFound
	}
	if ev.Status != "PROCESSING" || ev.LockedBy != owner {
		return ErrConflict
	}
	ev.Status, ev.LastError, ev.ProcessedAt, ev.LockedBy, ev.LockedAt = "FAILED", errText, &now, "", nil
	m.outboxEvents[id] = ev
	return nil
}
