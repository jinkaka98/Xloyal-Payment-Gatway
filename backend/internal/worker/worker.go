package worker

import (
	"context"
	"strings"
	"time"

	"xloyal/backend/internal/domain"
	"xloyal/backend/internal/gateway"
	"xloyal/backend/internal/qris"
	"xloyal/backend/internal/store"
)

const PollInterval = time.Minute
const MerchantSyncInterval = 5 * time.Minute
const MerchantQueuePollInterval = 5 * time.Second
const MaxChecks = 30
const MaxAge = 30 * time.Minute
const TestPaymentMatchWindow = 10 * time.Minute
const TestPaymentPollInterval = 30 * time.Second
const TestPaymentQueuePollInterval = 5 * time.Second
const MerchantSyncTimeout = 11 * time.Minute

type MerchantSync func(context.Context, domain.MerchantConnection) ([]domain.PortalTransaction, error)

type Worker struct {
	Repo         store.Repository
	Gateway      gateway.Service
	SyncMerchant MerchantSync
	Now          func() time.Time
}

func (w Worker) RunOnce(ctx context.Context) error {
	if err := w.checkInvoices(ctx); err != nil {
		return err
	}
	if err := w.validateTestPayments(ctx); err != nil {
		return err
	}
	return w.syncMerchants(ctx, w.now())
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
	items, err := w.Repo.PendingTestPayments(ctx, now, 100)
	if err != nil {
		return err
	}
	claimedTransactionIDs := make(map[string]struct{})
	allPayments, err := w.Repo.ListTestPayments(ctx, 1000)
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
	startedAt := w.now()
	items, err := w.Repo.PendingTestPayments(ctx, startedAt, 100)
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
	return expireRemaining(completedAt)
}
func transactionID(transaction domain.PortalTransaction, now time.Time) string {
	return transaction.MerchantID + "-" + transaction.Reference + "-" + now.Format("20060102150405")
}
func (w Worker) Run(ctx context.Context) error {
	invoiceTicker := time.NewTicker(PollInterval)
	testPaymentTicker := time.NewTicker(TestPaymentQueuePollInterval)
	merchantTicker := time.NewTicker(MerchantQueuePollInterval)
	defer invoiceTicker.Stop()
	defer testPaymentTicker.Stop()
	defer merchantTicker.Stop()
	type browserResult struct {
		err error
	}
	browserDone := make(chan browserResult, 1)
	browserBusy := false
	validationPending := false
	startBrowserOperation := func(validation bool) {
		if browserBusy {
			if validation {
				validationPending = true
			}
			return
		}
		browserBusy = true
		go func() {
			if validation {
				browserDone <- browserResult{err: w.validateTestPayments(ctx)}
				return
			}
			browserDone <- browserResult{err: w.syncMerchants(ctx, w.now())}
		}()
	}
	startBrowserOperation(false)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-invoiceTicker.C:
			if err := w.checkInvoices(ctx); err != nil {
				return err
			}
		case <-testPaymentTicker.C:
			startBrowserOperation(true)
		case <-merchantTicker.C:
			startBrowserOperation(false)
		case result := <-browserDone:
			browserBusy = false
			if result.err != nil {
				return result.err
			}
			if validationPending {
				validationPending = false
				startBrowserOperation(true)
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
