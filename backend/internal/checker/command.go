package checker

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"

	"xloyal/backend/internal/domain"
	"xloyal/backend/internal/security"
)

type commandInput struct {
	MerchantID string          `json:"merchant_id"`
	Cookies    json.RawMessage `json:"cookies"`
}
type commandOutput struct {
	Transactions []domain.PortalTransaction `json:"transactions"`
}

// CommandRunner delegates authenticated portal parsing to a local Camoufox helper.
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
		input, err := json.Marshal(commandInput{MerchantID: connection.MerchantID, Cookies: cookies})
		if err != nil {
			return nil, err
		}
		parts := strings.Fields(command)
		if len(parts) == 0 {
			return nil, errors.New("invalid CAMOUFOX_CHECKER_CMD")
		}
		cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
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
		if err = json.Unmarshal(raw, &output); err != nil {
			return nil, err
		}
		return output.Transactions, nil
	}
}
