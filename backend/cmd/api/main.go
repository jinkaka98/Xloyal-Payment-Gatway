package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"xloyal/backend/internal/httpapi"
	appRuntime "xloyal/backend/internal/runtime"
	"xloyal/backend/internal/security"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{ReplaceAttr: security.RedactAttrs}))
	app, err := appRuntime.Open()
	if err != nil {
		log.Error("startup failed", "error", err)
		os.Exit(1)
	}
	defer app.DB.Close()
	tokens := map[string]string{}
	if raw := os.Getenv("ADMIN_TOKENS_JSON"); raw != "" {
		err = json.Unmarshal([]byte(raw), &tokens)
	}
	if err != nil {
		log.Error("invalid ADMIN_TOKENS_JSON")
		os.Exit(1)
	}
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	server := http.Server{
		Addr:              addr,
		Handler:           httpapi.Server{Repo: app.Repo, Gateway: app.Gateway, Cipher: app.Cipher, AdminTokens: tokens, ManualLogin: app.ManualLogin, WebhookSecret: os.Getenv("GITHUB_WEBHOOK_SECRET"), WebhookSignalPath: os.Getenv("GITHUB_WEBHOOK_SIGNAL_PATH"), PublicPaymentBaseURL: os.Getenv("PUBLIC_PAYMENT_BASE_URL"), SSE: &httpapi.PaymentSSEHub{BufferSize: 16}}.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		// SSE payment event streams are long-lived; request/read timeouts still
		// protect the API while writes remain open until the client disconnects.
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if shutdownErr := server.Shutdown(shutdownCtx); shutdownErr != nil {
			log.Error("API shutdown failed", "error", shutdownErr)
		}
	}()
	log.Info("API listening", "addr", addr)
	if err = server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("API stopped", "error", err)
		os.Exit(1)
	}
}
