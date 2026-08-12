package worker

import (
	"context"
	"strings"
	"time"

	"xloyal/backend/internal/domain"
	"xloyal/backend/internal/gateway"
	"xloyal/backend/internal/store"
)

const PollInterval = time.Minute
const MerchantSyncInterval = 5 * time.Minute
const MerchantQueuePollInterval = 5 * time.Second
const MaxChecks = 30
const MaxAge = 30 * time.Minute
const TestPaymentMatchWindow = 10 * time.Minute
const TestPaymentPollInterval = 30 * time.Second
const MerchantSyncTimeout = 150 * time.Second

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
	if err := w.syncMerchants(ctx, w.now()); err != nil {
		return err
	}
	return w.checkTestPayments(ctx, w.now())
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
	for _, connection := range connections {
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
		for _, transaction := range transactions {
			transaction.MerchantID = connection.MerchantID
			transaction.ID = transactionID(transaction, now)
			transaction.Source = "browser"
			transaction.CreatedAt = now
			w.reconcileTransaction(ctx, &transaction, now)
			_ = w.Repo.CreatePortalTransaction(ctx, transaction)
		}
		connection.LastSyncedAt = &now
		connection.LastError = ""
		connection.Status = domain.ConnectionConnected
		connection.UpdatedAt = now
		_ = w.Repo.UpsertMerchantConnection(ctx, connection)
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
	var matches []domain.Invoice
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
			delta := invoice.CreatedAt.Sub(transaction.PaidAt)
			if delta < 0 {
				delta = -delta
			}
			if delta <= 10*time.Minute {
				matches = append(matches, invoice)
			}
		}
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
	transaction.TenantID, transaction.InvoiceID, transaction.MatchConfidence = match.TenantID, match.ID, "amount_time_unique"
}
func (w Worker) checkTestPayments(ctx context.Context, now time.Time) error {
	if _, err := w.Repo.ExpirePendingTestPayments(ctx, now); err != nil {
		return err
	}
	items, err := w.Repo.PendingTestPayments(ctx, now, 100)
	if err != nil {
		return err
	}
	// One browser sync is enough for all due payments belonging to the same
	// merchant. Queue it once, then match every payment against that ledger.
	merchantIDs := make(map[string]struct{})
	for _, payment := range items {
		if payment.MerchantID != "" {
			merchantIDs[payment.MerchantID] = struct{}{}
		}
	}
	for merchantID := range merchantIDs {
		connection, connErr := w.Repo.MerchantConnection(ctx, merchantID)
		if connErr != nil {
			continue
		}
		// A failed session must keep its diagnostic and wait for a reconnect.
		// Re-queuing it on every test tick would hide the real browser error.
		if connection.Status != domain.ConnectionConnected {
			continue
		}
		connection.UpdatedAt = time.Unix(0, 0).UTC()
		connection.LastError = "QRIS test validation batch queued"
		if err := w.Repo.UpsertMerchantConnection(ctx, connection); err != nil {
			return err
		}
	}
	ledgers := make(map[string][]domain.PortalTransaction)
	for _, payment := range items {
		checkedAt := now
		payment.LastCheckedAt = &checkedAt
		payment.UpdatedAt = now
		payment.CheckCount++
		payment.MatchConfidence = "no_matching_merchant_transaction"

		var matches []domain.PortalTransaction
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
				if transaction.Status != "paid" || transaction.Amount != payment.Amount || transaction.InvoiceID != "" {
					continue
				}
				delta := transaction.PaidAt.Sub(payment.CreatedAt)
				if delta < 0 {
					delta = -delta
				}
				if delta <= TestPaymentMatchWindow && !transaction.PaidAt.After(payment.ExpiresAt) {
					matches = append(matches, transaction)
				}
			}
		} else {
			payment.MatchConfidence = "merchant_not_linked"
		}

		switch len(matches) {
		case 1:
			payment.Status = domain.InvoicePaid
			payment.NextCheckAt = nil
			payment.MatchConfidence = "amount_time_unique"
			payment.MatchedTransactionID = matches[0].ID
			transaction := matches[0]
			if payment.TenantID != "" {
				transaction.TenantID = payment.TenantID
			}
			transaction.MatchConfidence = "qris_test_amount_time_unique"
			_ = w.Repo.CreatePortalTransaction(ctx, transaction)
		default:
			if len(matches) > 1 {
				payment.MatchConfidence = "ambiguous_amount_time"
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
		if _, err := w.Repo.UpdatePendingTestPayment(ctx, payment); err != nil {
			return err
		}
	}
	return nil
}
func transactionID(transaction domain.PortalTransaction, now time.Time) string {
	return transaction.MerchantID + "-" + transaction.Reference + "-" + now.Format("20060102150405")
}
func (w Worker) Run(ctx context.Context) error {
	invoiceTicker := time.NewTicker(PollInterval)
	testPaymentTicker := time.NewTicker(TestPaymentPollInterval)
	merchantTicker := time.NewTicker(MerchantQueuePollInterval)
	defer invoiceTicker.Stop()
	defer testPaymentTicker.Stop()
	defer merchantTicker.Stop()
	merchantDone := make(chan error, 1)
	merchantSyncing := false
	startMerchantSync := func() {
		if merchantSyncing {
			return
		}
		merchantSyncing = true
		now := w.now()
		go func() { merchantDone <- w.syncMerchants(ctx, now) }()
	}
	startMerchantSync()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-invoiceTicker.C:
			if err := w.checkInvoices(ctx); err != nil {
				return err
			}
		case <-testPaymentTicker.C:
			if err := w.checkTestPayments(ctx, w.now()); err != nil {
				return err
			}
		case <-merchantTicker.C:
			startMerchantSync()
		case err := <-merchantDone:
			merchantSyncing = false
			if err != nil {
				return err
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
