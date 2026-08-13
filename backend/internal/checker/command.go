package checker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"xloyal/backend/internal/domain"
	"xloyal/backend/internal/security"
)

type commandInput struct {
	MerchantID        string          `json:"merchant_id"`
	Cookies           json.RawMessage `json:"cookies"`
	BrowserCredential json.RawMessage `json:"browser_credential,omitempty"`
	SyncFrom          string          `json:"sync_from,omitempty"`
}
type commandOutput struct {
	Transactions []domain.PortalTransaction `json:"transactions"`
}

// CommandRunner delegates authenticated portal parsing to a local Camoufox helper.
func ManualLoginRunner(command string, cipher *security.Cipher) func(context.Context, domain.MerchantConnection) error {
	return func(ctx context.Context, connection domain.MerchantConnection) error {
		if strings.TrimSpace(command) == "" {
			return errors.New("CAMOUFOX_CHECKER_CMD is not configured")
		}
		cookies := []byte("[]")
		if connection.SessionCiphertext != "" {
			var err error
			cookies, err = cipher.Decrypt(connection.SessionCiphertext)
			if err != nil {
				return err
			}
		}
		browserCredential := []byte("null")
		if connection.BrowserCredentialCiphertext != "" {
			var err error
			browserCredential, err = cipher.Decrypt(connection.BrowserCredentialCiphertext)
			if err != nil {
				return err
			}
		}
		input, err := json.Marshal(commandInput{MerchantID: connection.MerchantID, Cookies: cookies, BrowserCredential: browserCredential})
		if err != nil {
			return err
		}
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", "$env:WEBWRIGHT_MANUAL_LOGIN='true'; "+command)
		} else {
			cmd = exec.CommandContext(ctx, "sh", "-c", "WEBWRIGHT_MANUAL_LOGIN=true "+command)
		}
		cmd.Stdin = strings.NewReader(string(input))
		raw, err := cmd.CombinedOutput()
		if err == nil {
			return nil
		}
		message := strings.TrimSpace(string(raw))
		if len(message) > 500 {
			message = message[len(message)-500:]
		}
		if message != "" {
			return errors.New(err.Error() + ": " + message)
		}
		return err
	}
}

func CommandRunner(command string, cipher *security.Cipher) func(context.Context, domain.MerchantConnection) ([]domain.PortalTransaction, error) {
	return func(ctx context.Context, connection domain.MerchantConnection) ([]domain.PortalTransaction, error) {
		if strings.TrimSpace(command) == "" {
			return nil, errors.New("CAMOUFOX_CHECKER_CMD is not configured")
		}
		cookies := []byte("[]")
		if connection.SessionCiphertext != "" {
			var err error
			cookies, err = cipher.Decrypt(connection.SessionCiphertext)
			if err != nil {
				return nil, err
			}
		}
		browserCredential := []byte("null")
		if connection.BrowserCredentialCiphertext != "" {
			var err error
			browserCredential, err = cipher.Decrypt(connection.BrowserCredentialCiphertext)
			if err != nil {
				return nil, err
			}
		}
		syncFrom := time.Now().AddDate(0, 0, -30)
		if connection.HistoryBackfilledAt != nil && connection.LastSyncedAt != nil {
			syncFrom = connection.LastSyncedAt.Add(-5 * time.Minute)
		}
		input, err := json.Marshal(commandInput{MerchantID: connection.MerchantID, Cookies: cookies, BrowserCredential: browserCredential, SyncFrom: syncFrom.Format("2006-01-02")})
		if err != nil {
			return nil, err
		}
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", command)
		} else {
			parts := strings.Fields(command)
			if len(parts) == 0 {
				return nil, errors.New("invalid CAMOUFOX_CHECKER_CMD")
			}
			cmd = exec.CommandContext(ctx, parts[0], parts[1:]...)
		}
		cmd.Stdin = strings.NewReader(string(input))
		raw, err := cmd.CombinedOutput()
		if err != nil {
			message := strings.TrimSpace(string(raw))
			if len(message) > 500 {
				message = message[len(message)-500:]
			}
			if message != "" {
				return nil, errors.New(err.Error() + ": " + message)
			}
			return nil, err
		}
		var output commandOutput
		payloadStart := bytes.LastIndex(raw, []byte(`{"transactions"`))
		if payloadStart < 0 {
			return nil, errors.New("checker output does not contain a transactions payload")
		}
		if err = json.Unmarshal(raw[payloadStart:], &output); err != nil {
			return nil, err
		}
		return output.Transactions, nil
	}
}
