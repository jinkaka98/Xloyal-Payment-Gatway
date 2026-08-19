package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	appRuntime "xloyal/backend/internal/runtime"
	"xloyal/backend/internal/security"
	"xloyal/backend/internal/worker"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{ReplaceAttr: security.RedactAttrs}))
	app, err := appRuntime.Open()
	if err != nil {
		log.Error("startup failed", "error", err)
		os.Exit(1)
	}
	defer app.DB.Close()
	hostname, _ := os.Hostname()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	err = (worker.Worker{Repo: app.Repo, Gateway: app.Gateway, SyncMerchant: app.MerchantSync, ManualLogin: app.ManualLogin, Logger: log, JobOwner: fmt.Sprintf("%s-%d", hostname, os.Getpid()), Cipher: app.Cipher}).Run(ctx)
	if err != nil && err != context.Canceled {
		log.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}
