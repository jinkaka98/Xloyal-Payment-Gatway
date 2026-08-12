package runtime

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"xloyal/backend/internal/checker"
	"xloyal/backend/internal/domain"
	"xloyal/backend/internal/gateway"
	"xloyal/backend/internal/postgres"
	"xloyal/backend/internal/provider"
	"xloyal/backend/internal/security"
)

type App struct {
	DB           *sql.DB
	Repo         *postgres.Repository
	Cipher       *security.Cipher
	Gateway      gateway.Service
	MerchantSync func(context.Context, domain.MerchantConnection) ([]domain.PortalTransaction, error)
	ManualLogin  func(context.Context, domain.MerchantConnection) error
}

func Open() (*App, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	raw, err := base64.RawStdEncoding.DecodeString(os.Getenv("CREDENTIAL_ENCRYPTION_KEY"))
	if err != nil {
		db.Close()
		return nil, errors.New("CREDENTIAL_ENCRYPTION_KEY must be unpadded base64")
	}
	cipher, err := security.NewCipher(raw)
	if err != nil {
		db.Close()
		return nil, err
	}
	repo := postgres.New(db)
	resolve := func(_ context.Context, m domain.MerchantAccount) (domain.PaymentProvider, error) {
		if m.Provider != provider.InteractiveQRISProvider {
			return nil, errors.New("unsupported payment provider; use interactive_qris")
		}
		plain, err := cipher.Decrypt(m.CredentialCiphertext)
		if err != nil {
			return nil, err
		}
		var cfg provider.OpenAPIConfig
		if err = json.Unmarshal(plain, &cfg); err != nil {
			return nil, errors.New("invalid encrypted provider configuration")
		}
		return provider.NewOpenAPI(cfg)
	}
	g := gateway.Service{Repo: repo, Provider: resolve}
	var merchantSync func(context.Context, domain.MerchantConnection) ([]domain.PortalTransaction, error)
	var manualLogin func(context.Context, domain.MerchantConnection) error
	if command := os.Getenv("CAMOUFOX_CHECKER_CMD"); command != "" {
		merchantSync = checker.CommandRunner(command, cipher)
		manualLogin = checker.ManualLoginRunner(command, cipher)
	}
	return &App{DB: db, Repo: repo, Cipher: cipher, Gateway: g, MerchantSync: merchantSync, ManualLogin: manualLogin}, nil
}
