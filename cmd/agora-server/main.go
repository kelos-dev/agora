package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kelos-dev/agora/internal/agora"
	"github.com/kelos-dev/agora/internal/server"
)

func main() {
	cfg := configFromEnv()

	store, err := agora.NewStore(cfg.DataPath)
	if err != nil {
		slog.Error("Failed to open event store", "error", err, "path", cfg.DataPath)
		os.Exit(1)
	}

	app, err := server.New(store, server.Config{Token: cfg.Token})
	if err != nil {
		slog.Error("Failed to create server", "error", err)
		os.Exit(1)
	}

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errs := make(chan error, 1)
	go func() {
		slog.Info("Starting Agora server", "addr", cfg.Addr, "data", cfg.DataPath)
		errs <- httpServer.ListenAndServe()
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-signals:
		slog.Info("Shutting down server", "signal", sig.String())
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("Server stopped", "error", err)
			os.Exit(1)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("Failed to shut down server", "error", err)
		os.Exit(1)
	}
}

type config struct {
	Addr     string
	DataPath string
	Token    string
}

func configFromEnv() config {
	cfg := config{
		Addr:     "127.0.0.1:8080",
		DataPath: "agora.jsonl",
		Token:    os.Getenv("AGORA_TOKEN"),
	}
	if v := os.Getenv("AGORA_ADDR"); v != "" {
		cfg.Addr = v
	}
	if v := os.Getenv("AGORA_DATA"); v != "" {
		cfg.DataPath = v
	}
	return cfg
}
