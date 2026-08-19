package worker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"xloyal/backend/internal/domain"
	"xloyal/backend/internal/gateway"
	"xloyal/backend/internal/qris"
	"xloyal/backend/internal/security"
	"xloyal/backend/internal/store"
	"xloyal/backend/internal/webhook"
)

const PollInterval = time.Minute
const MerchantSyncInterval = 5 * time.Minute
const MerchantQueuePollInterval = time.Minute
const MaxChecks = 30
const MaxAge = 30 * time.Minute
const TestPaymentMatchWindow = 10 * time.Minute
const TestPaymentPollInterval = 30 * time.Second
const TestPaymentQueuePollInterval = 5 * time.Second
const MerchantSyncTimeout = 11 * time.Minute
const TestPaymentBatchSize = 500
const BrowserJobLease = 13 * time.Minute
const BrowserJobPollInterval = time.Second
const OutboxPollInterval = time.Second
const OutboxLease = time.Minute
const OutboxBatchSize = 100

type MerchantSync func(context.Context, domain.MerchantConnection) ([]domain.PortalTransaction, error)

type Worker struct {
	Repo         store.Repository
	Gateway      gateway.Service
	SyncMerchant MerchantSync
	ManualLogin  func(context.Context, domain.MerchantConnection) error
	Now          func() time.Time
	Logger       *slog.Logger
	JobOwner     string
	Cipher       *security.Cipher
}

func (w Worker) RunOnce(ctx context.Context) error {
	if err := w.dispatchOutbox(ctx); err != nil {
		return err
	}
	if err := w.checkInvoices(ctx); err != nil {
		return err
	}
	if err := w.validateTestPayments(ctx); err != nil {
		return err
	}
	return w.syncMerchants(ctx, w.now())
}

// dispatchOutbox is the durable worker-side dispatcher. SSE clients recover
// from payment_events by sequence, so claiming/acknowledging here is safe even
// when the API process (and its in-memory transport registry) restarts.
func (w Worker) dispatchOutbox(ctx context.Context) error {
	now := w.now()
	owner := w.JobOwner
	if owner == "" {
		owner = "xloyal-worker"
	}
	items, err := w.Repo.ClaimOutboxEvents(ctx, owner, now, OutboxLease, OutboxBatchSize)
	if err != nil {
		return err
	}
	for _, item := range items {
		events, eventErr := w.Repo.PaymentEvents(ctx, item.TenantID, item.AggregateID)
		if eventErr != nil {
			_ = w.Repo.MarkOutboxRetry(ctx, item.ID, owner, now, OutboxLease, eventErr.Error())
			continue
		}
		var sequence int64
		for _, event := range events {
			if event.EventID == item.EventID {
				sequence = event.SequenceNumber
				break
			}
		}
		if sequence == 0 {
			_ = w.Repo.MarkOutboxFailed(ctx, item.ID, owner, now, "payment event not found")
			continue
		}
		if err := (webhook.Delivery{Repo: w.Repo, Cipher: w.Cipher, Logger: w.Logger, Owner: owner}).EnqueueForEvent(ctx, eventForOutbox(events, item.EventID)); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				// The tenant may have been soft-deleted after the payment event.
				// There is no destination left to deliver to, so acknowledge the
				// durable event without exposing internal lookup details.
				w.logger().Info("webhook delivery skipped for missing tenant", "tenant_id", item.TenantID, "event_id", item.EventID, "outbox_event_id", item.ID)
			} else {
				w.logger().Warn("webhook delivery enqueue failed", "tenant_id", item.TenantID, "event_id", item.EventID, "outbox_event_id", item.ID, "error", truncateError(err.Error()))
				if retryErr := w.Repo.MarkOutboxRetry(ctx, item.ID, owner, now, OutboxLease, truncateError(err.Error())); retryErr != nil && !errors.Is(retryErr, store.ErrConflict) {
					return retryErr
				}
				continue
			}
		}
		w.logger().Info("payment event dispatched", "payment_session_id", item.AggregateID, "invoice_id", eventInvoiceID(events, item.EventID), "event_id", item.EventID, "sequence", sequence, "outbox_event_id", item.ID, "subscriber_count", 0)
		if err := w.Repo.MarkOutboxDelivered(ctx, item.ID, owner, now); err != nil && !errors.Is(err, store.ErrConflict) {
			return err
		}
	}
	if err := (webhook.Delivery{Repo: w.Repo, Cipher: w.Cipher, Logger: w.Logger, Owner: owner}).DispatchOnce(ctx); err != nil {
		return err
	}
	return nil
}

func eventForOutbox(events []domain.PaymentEvent, eventID string) domain.PaymentEvent {
	for _, event := range events {
		if event.EventID == eventID {
			return event
		}
	}
	return domain.PaymentEvent{}
}

func eventInvoiceID(events []domain.PaymentEvent, eventID string) string {
	for _, event := range events {
		if event.EventID == eventID {
			return event.InvoiceID
		}
	}
	return ""
}

func (w Worker) checkInvoices(ctx context.Context) error {
	now := w.now()
	items, err := w.Repo.PendingInvoices(ctx, now.Add(-PollInterval), 100)
	if err != nil {
		return err
	}
	for _, inv := range items {
		if inv.CheckCount >= MaxChecks || !inv.ExpiresAt.After(now) || now.Sub(inv.CreatedAt) >= MaxAge {
			if err := w.Gateway.Expire(ctx, inv); err != nil {
				return err
			}
			continue
		}
		if _, err := w.Gateway.Check(ctx, inv.TenantID, inv.ID); err != nil {
			continue
		}
	}
	return nil
}
func (w Worker) syncMerchants(ctx context.Context, now time.Time) error {
	if w.SyncMerchant == nil {
		return nil
	}
	connections, err := w.Repo.ListDueMerchantConnections(ctx, now.Add(-MerchantSyncInterval), 50)
	if err != nil {
		return err
	}
	return w.syncConnections(ctx, now, connections)
}

func (w Worker) syncConnections(ctx context.Context, now time.Time, connections []domain.MerchantConnection) error {
	if w.SyncMerchant == nil {
		return nil
	}
	for _, connection := range connections {
		connection.LastError = "Browser sync in progress"
		connection.UpdatedAt = now
		if err := w.Repo.UpsertMerchantConnection(ctx, connection); err != nil {
			return err
		}
		syncCtx, cancel := context.WithTimeout(ctx, MerchantSyncTimeout)
		transactions, syncErr := w.SyncMerchant(syncCtx, connection)
		cancel()
		if syncErr != nil {
			connection.Status = domain.ConnectionReconnectRequired
			connection.LastError = "browser sync failed: " + truncateError(syncErr.Error())
			connection.UpdatedAt = now
			_ = w.Repo.UpsertMerchantConnection(ctx, connection)
			continue
		}
		persistenceFailed := false
		for _, transaction := range transactions {
			transaction.MerchantID = connection.MerchantID
			transaction.ID = transactionID(transaction, now)
			transaction.Source = "browser"
			transaction.CreatedAt = now
			w.reconcileTransaction(ctx, &transaction, now)
			if err := w.Repo.CreatePortalTransaction(ctx, transaction); err != nil {
				connection.Status = domain.ConnectionReconnectRequired
				connection.LastError = "browser sync persistence failed: " + truncateError(err.Error())
				connection.UpdatedAt = now
				_ = w.Repo.UpsertMerchantConnection(ctx, connection)
				persistenceFailed = true
				break
			}
		}
		if persistenceFailed {
			continue
		}
		completedAt := w.now()
		connection.LastSyncedAt = &completedAt
		if connection.HistoryBackfilledAt == nil {
			connection.HistoryBackfilledAt = &completedAt
		}
		connection.LastError = ""
		connection.Status = domain.ConnectionConnected
		connection.UpdatedAt = completedAt
		if err := w.Repo.UpsertMerchantConnection(ctx, connection); err != nil {
			return err
		}
	}
	return nil
}

func truncateError(message string) string {
	message = strings.TrimSpace(message)
	if len(message) <= 600 {
		return message
	}
	return message[:240]
}
func (w Worker) reconcileTransaction(ctx context.Context, transaction *domain.PortalTransaction, now time.Time) {
	tenants, err := w.Repo.ListTenants(ctx)
	if err != nil {
		return
	}
	var uniqueMatches []domain.Invoice
	var timeMatches []domain.Invoice
	for _, tenant := range tenants {
		if tenant.MerchantID != transaction.MerchantID {
			continue
		}
		invoices, listErr := w.Repo.ListInvoices(ctx, tenant.ID, 200)
		if listErr != nil {
			continue
		}
		for _, invoice := range invoices {
			if invoice.Status != domain.InvoicePending || invoice.Amount != transaction.Amount {
				continue
			}
			if referenceMatches(transaction.Reference, invoiceUniqueCode(invoice)) {
				uniqueMatches = append(uniqueMatches, invoice)
				continue
			}
			if withinMatchWindow(invoice.CreatedAt, transaction.PaidAt, 10*time.Minute) {
				timeMatches = append(timeMatches, invoice)
			}
		}
	}
	matches, confidence := uniqueMatches, "reference_unique"
	if len(matches) == 0 {
		matches, confidence = timeMatches, "amount_time_unique"
	}
	if len(matches) != 1 {
		transaction.MatchConfidence = "unmatched"
		return
	}
	match := matches[0]
	if match.Transition(domain.InvoicePaid, now) != nil {
		return
	}
	updated, err := w.Repo.UpdatePendingInvoice(ctx, match)
	if err != nil || !updated {
		return
	}
	transaction.TenantID, transaction.InvoiceID, transaction.MatchConfidence = match.TenantID, match.ID, confidence
}

func invoiceUniqueCode(inv domain.Invoice) string {
	if code, err := qris.BillNumber(inv.QRPayload); err == nil && strings.TrimSpace(code) != "" {
		return code
	}
	return strings.TrimSpace(inv.ProviderReference)
}

func referenceMatches(reference, code string) bool {
	code = strings.TrimSpace(code)
	if code == "" {
		return false
	}
	reference = strings.TrimSpace(reference)
	return reference == code || strings.Contains(reference, code)
}

func withinMatchWindow(a, b time.Time, window time.Duration) bool {
	delta := a.Sub(b)
	if delta < 0 {
		delta = -delta
	}
	return delta <= window
}
func (w Worker) checkTestPayments(ctx context.Context, now time.Time) error {
	if err := w.checkTestPaymentsForMerchants(ctx, now, nil); err != nil {
		return err
	}
	_, err := w.Repo.ExpirePendingTestPayments(ctx, now)
	return err
}

func (w Worker) checkTestPaymentsForMerchants(ctx context.Context, now time.Time, syncedMerchants map[string]struct{}) error {
	var items []domain.TestPayment
	var err error
	if syncedMerchants != nil {
		items, err = w.Repo.PendingConnectedTestPayments(ctx, now, TestPaymentBatchSize)
	} else {
		items, err = w.Repo.PendingTestPayments(ctx, now, TestPaymentBatchSize)
	}
	if err != nil {
		return err
	}
	claimedTransactionIDs := make(map[string]struct{})
	allPayments, err := w.Repo.ListTestPayments(ctx, TestPaymentBatchSize*10)
	if err != nil {
		return err
	}
	for _, payment := range allPayments {
		if payment.MatchedTransactionID != "" {
			claimedTransactionIDs[payment.MatchedTransactionID] = struct{}{}
		}
	}
	ledgers := make(map[string][]domain.PortalTransaction)
	for _, payment := range items {
		if syncedMerchants != nil {
			if _, ok := syncedMerchants[payment.MerchantID]; !ok {
				continue
			}
		}
		checkedAt := now
		payment.LastCheckedAt = &checkedAt
		payment.UpdatedAt = now
		payment.CheckCount++
		payment.MatchConfidence = "no_matching_merchant_transaction"

		var uniqueMatches []domain.PortalTransaction
		var timeMatches []domain.PortalTransaction
		if payment.MerchantID != "" {
			transactions, ok := ledgers[payment.MerchantID]
			if !ok {
				var listErr error
				transactions, listErr = w.Repo.ListPortalTransactions(ctx, payment.MerchantID, "", 500)
				if listErr != nil {
					return listErr
				}
				ledgers[payment.MerchantID] = transactions
			}
			for _, transaction := range transactions {
				if transaction.Status != "paid" || transaction.Amount != payableAmount(payment) || transaction.InvoiceID != "" {
					continue
				}
				if _, claimed := claimedTransactionIDs[transaction.ID]; claimed {
					continue
				}
				if referenceMatches(transaction.Reference, payment.UniqueCode) {
					uniqueMatches = append(uniqueMatches, transaction)
					continue
				}
				if uniquelyMatchesPaymentByAmountTime(transaction, payment, allPayments) {
					timeMatches = append(timeMatches, transaction)
				}
			}
		} else {
			payment.MatchConfidence = "merchant_not_linked"
		}

		matches := uniqueMatches
		unique := true
		if len(matches) == 0 {
			matches = timeMatches
			unique = false
		}

		var matchedTransaction *domain.PortalTransaction
		switch len(matches) {
		case 1:
			payment.Status = domain.InvoicePaid
			payment.NextCheckAt = nil
			payment.MatchConfidence = "amount_time_unique"
			if unique {
				payment.MatchConfidence = "reference_unique"
			}
			payment.MatchedTransactionID = matches[0].ID
			transaction := matches[0]
			if payment.TenantID != "" {
				transaction.TenantID = payment.TenantID
			}
			transaction.MatchConfidence = "qris_test_amount_time_unique"
			if unique {
				transaction.MatchConfidence = "qris_test_reference_unique"
			}
			matchedTransaction = &transaction
		default:
			if len(matches) > 1 {
				payment.MatchConfidence = "ambiguous_amount_time"
				if unique {
					payment.MatchConfidence = "ambiguous_reference"
				}
			}
			if !payment.ExpiresAt.After(now) {
				payment.Status = domain.InvoiceExpired
				payment.MatchConfidence = "expired_no_match"
				payment.NextCheckAt = nil
			} else {
				next := now.Add(TestPaymentPollInterval)
				if next.After(payment.ExpiresAt) {
					next = payment.ExpiresAt
				}
				payment.NextCheckAt = &next
			}
		}
		var updated bool
		if matchedTransaction != nil {
			updated, err = w.Repo.MatchPendingTestPayment(ctx, payment, *matchedTransaction)
		} else {
			updated, err = w.Repo.UpdatePendingTestPayment(ctx, payment)
		}
		if err != nil {
			return err
		}
		if !updated {
			continue
		}
		if matchedTransaction != nil {
			claimedTransactionIDs[payment.MatchedTransactionID] = struct{}{}
		}
	}
	return nil
}

func uniquelyMatchesPaymentByAmountTime(transaction domain.PortalTransaction, target domain.TestPayment, payments []domain.TestPayment) bool {
	matchedID := ""
	count := 0
	for _, payment := range payments {
		if payment.Status != domain.InvoicePending || payment.MerchantID != transaction.MerchantID || payableAmount(payment) != transaction.Amount {
			continue
		}
		if transaction.PaidAt.Before(payment.CreatedAt) || transaction.PaidAt.After(payment.ExpiresAt) || !withinMatchWindow(transaction.PaidAt, payment.CreatedAt, TestPaymentMatchWindow) {
			continue
		}
		if referenceMatches(transaction.Reference, payment.UniqueCode) {
			return payment.ID == target.ID
		}
		matchedID = payment.ID
		count++
	}
	return count == 1 && matchedID == target.ID
}

func payableAmount(payment domain.TestPayment) int64 {
	if payment.PayableAmount > 0 {
		return payment.PayableAmount
	}
	return payment.Amount
}

func (w Worker) validateTestPayments(ctx context.Context) error {
	cycleStarted := time.Now()
	startedAt := w.now()
	items, err := w.Repo.PendingConnectedTestPayments(ctx, startedAt, TestPaymentBatchSize)
	if err != nil {
		return err
	}
	expireRemaining := func(now time.Time) error {
		_, expireErr := w.Repo.ExpirePendingTestPayments(ctx, now)
		return expireErr
	}
	if len(items) == 0 {
		return expireRemaining(startedAt)
	}
	queuedMerchants := make(map[string]struct{})
	queuedConnections := make([]domain.MerchantConnection, 0)
	for _, payment := range items {
		if payment.MerchantID == "" {
			continue
		}
		if _, queued := queuedMerchants[payment.MerchantID]; queued {
			continue
		}
		connection, connErr := w.Repo.MerchantConnection(ctx, payment.MerchantID)
		if connErr != nil || connection.Status != domain.ConnectionConnected {
			continue
		}
		connection.UpdatedAt = time.Unix(0, 0).UTC()
		connection.LastError = "QRIS test validation batch queued"
		if err := w.Repo.UpsertMerchantConnection(ctx, connection); err != nil {
			return err
		}
		queuedMerchants[payment.MerchantID] = struct{}{}
		queuedConnections = append(queuedConnections, connection)
	}
	if len(queuedMerchants) == 0 {
		return expireRemaining(startedAt)
	}
	if err := w.syncConnections(ctx, startedAt, queuedConnections); err != nil {
		return err
	}
	syncedMerchants := make(map[string]struct{})
	for merchantID := range queuedMerchants {
		connection, connErr := w.Repo.MerchantConnection(ctx, merchantID)
		if connErr != nil || connection.Status != domain.ConnectionConnected || connection.LastError != "" || connection.LastSyncedAt == nil || connection.LastSyncedAt.Add(time.Millisecond).Before(startedAt) {
			continue
		}
		syncedMerchants[merchantID] = struct{}{}
	}
	completedAt := w.now()
	if len(syncedMerchants) != 0 {
		if err := w.checkTestPaymentsForMerchants(ctx, completedAt, syncedMerchants); err != nil {
			return err
		}
	}
	if err := expireRemaining(completedAt); err != nil {
		return err
	}
	w.logger().Info("qris validation cycle completed",
		"queued_payments", len(items),
		"merchants", len(queuedMerchants),
		"duration_ms", time.Since(cycleStarted).Milliseconds(),
	)
	return nil
}
func transactionID(transaction domain.PortalTransaction, now time.Time) string {
	return transaction.MerchantID + "-" + transaction.Reference + "-" + now.Format("20060102150405")
}
func (w Worker) Run(ctx context.Context) error {
	invoiceTicker := time.NewTicker(PollInterval)
	testPaymentTicker := time.NewTicker(TestPaymentQueuePollInterval)
	merchantTicker := time.NewTicker(MerchantQueuePollInterval)
	jobTicker := time.NewTicker(BrowserJobPollInterval)
	outboxTicker := time.NewTicker(OutboxPollInterval)
	defer invoiceTicker.Stop()
	defer testPaymentTicker.Stop()
	defer merchantTicker.Stop()
	defer jobTicker.Stop()
	defer outboxTicker.Stop()
	type jobResult struct {
		processed bool
		err       error
	}
	jobDone := make(chan jobResult, 1)
	browserBusy := false
	startBrowserJob := func() {
		if browserBusy {
			return
		}
		browserBusy = true
		go func() {
			processed, err := w.processNextBrowserJob(ctx)
			jobDone <- jobResult{processed: processed, err: err}
		}()
	}
	now := w.now()
	if _, _, err := w.enqueueBrowserJob(ctx, "payment_validation", "", 100, now); err != nil {
		return err
	}
	if _, _, err := w.enqueueBrowserJob(ctx, "merchant_sync", "", 10, now); err != nil {
		return err
	}
	startBrowserJob()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-outboxTicker.C:
			if err := w.dispatchOutbox(ctx); err != nil {
				return err
			}
		case <-invoiceTicker.C:
			if err := w.checkInvoices(ctx); err != nil {
				return err
			}
		case <-testPaymentTicker.C:
			if _, _, err := w.enqueueBrowserJob(ctx, "payment_validation", "", 100, w.now()); err != nil {
				return err
			}
			startBrowserJob()
		case <-merchantTicker.C:
			if _, _, err := w.enqueueBrowserJob(ctx, "merchant_sync", "", 10, w.now()); err != nil {
				return err
			}
			startBrowserJob()
		case <-jobTicker.C:
			startBrowserJob()
		case result := <-jobDone:
			browserBusy = false
			if result.err != nil {
				return result.err
			}
			if result.processed {
				startBrowserJob()
			}
		}
	}
}
func (w Worker) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}

func (w Worker) logger() *slog.Logger {
	if w.Logger != nil {
		return w.Logger
	}
	return slog.Default()
}

func (w Worker) processNextBrowserJob(ctx context.Context) (bool, error) {
	owner := w.JobOwner
	if owner == "" {
		owner = "xloyal-worker"
	}
	job, claimed, err := w.Repo.ClaimBrowserJob(ctx, owner, w.now(), BrowserJobLease)
	if err != nil || !claimed {
		return claimed, err
	}
	w.logger().Info("browser job started", "job_id", job.ID, "kind", job.Kind, "merchant_id", job.MerchantID, "attempt", job.Attempt, "request_count", job.RequestCount)
	started := time.Now()
	err = w.executeBrowserJob(ctx, job)
	completedAt := w.now()
	if err != nil {
		message := truncateError(err.Error())
		if failErr := w.Repo.FailBrowserJob(ctx, job.ID, owner, completedAt, message); failErr != nil {
			return true, fmt.Errorf("browser job failed: %v; persist failure: %w", err, failErr)
		}
		w.logger().Error("browser job failed", "job_id", job.ID, "kind", job.Kind, "merchant_id", job.MerchantID, "duration_ms", time.Since(started).Milliseconds(), "error", err)
		return true, nil
	}
	if err := w.Repo.CompleteBrowserJob(ctx, job.ID, owner, completedAt); err != nil {
		return true, err
	}
	w.logger().Info("browser job completed", "job_id", job.ID, "kind", job.Kind, "merchant_id", job.MerchantID, "duration_ms", time.Since(started).Milliseconds())
	return true, nil
}

func (w Worker) executeBrowserJob(ctx context.Context, job domain.BrowserJob) error {
	switch job.Kind {
	case "manual_login":
		if w.ManualLogin == nil {
			return fmt.Errorf("manual browser login is not configured")
		}
		connection, err := w.Repo.MerchantConnection(ctx, job.MerchantID)
		if err != nil {
			return err
		}
		connection.Status = domain.ConnectionReconnectRequired
		connection.LastError = "Manual browser login in progress"
		connection.UpdatedAt = w.now()
		if err := w.Repo.UpsertMerchantConnection(ctx, connection); err != nil {
			return err
		}
		if err := w.ManualLogin(ctx, connection); err != nil {
			connection.LastError = "Manual browser login failed: " + truncateError(err.Error())
			connection.UpdatedAt = w.now()
			_ = w.Repo.UpsertMerchantConnection(ctx, connection)
			return err
		}
		connection.LastSyncedAt = nil
		connection.LastError = "Browser connection queued"
		connection.UpdatedAt = w.now()
		if err := w.Repo.UpsertMerchantConnection(ctx, connection); err != nil {
			return err
		}
		_, _, err = w.enqueueBrowserJob(ctx, "merchant_sync", connection.MerchantID, 90, w.now())
		return err
	case "merchant_sync":
		if job.MerchantID == "" {
			return w.syncMerchants(ctx, w.now())
		}
		connection, err := w.Repo.MerchantConnection(ctx, job.MerchantID)
		if err != nil {
			return err
		}
		if connection.Status != domain.ConnectionConnected && connection.Status != domain.ConnectionReconnectRequired {
			return fmt.Errorf("merchant connection is not available")
		}
		return w.syncConnections(ctx, w.now(), []domain.MerchantConnection{connection})
	case "payment_validation":
		return w.validateTestPayments(ctx)
	default:
		return fmt.Errorf("unsupported browser job kind %q", job.Kind)
	}
}

func (w Worker) enqueueBrowserJob(ctx context.Context, kind, merchantID string, priority int, now time.Time) (domain.BrowserJob, bool, error) {
	return w.Repo.EnqueueBrowserJob(ctx, domain.BrowserJob{ID: newBrowserJobID(), ResourceKey: "neko-shared", MerchantID: merchantID, Kind: kind, Priority: priority, State: "queued", NotBefore: now, RequestedAt: now, RequestCount: 1})
}

func newBrowserJobID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}
