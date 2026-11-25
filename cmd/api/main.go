// Command api starts the register API server.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/S-Matsoso/goliath/internal/app"
	"github.com/S-Matsoso/goliath/internal/config"
	"github.com/S-Matsoso/goliath/internal/platform/logger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log := logger.New(cfg.AppEnv, cfg.LogLevel).With("service", cfg.AppName)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return app.New(cfg, log).Run(ctx)
}
