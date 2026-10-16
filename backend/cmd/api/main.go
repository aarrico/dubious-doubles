package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aarrico/dubious-doubles/backend/internal/config"
	"github.com/aarrico/dubious-doubles/backend/internal/httpapi"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, logger); err != nil {
		logger.Error("API stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return errors.New("invalid database configuration")
	}
	poolCfg.MaxConns = 4
	poolCfg.ConnConfig.ConnectTimeout = cfg.DatabaseTimeout
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return errors.New("could not create database pool")
	}
	defer pool.Close()
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: httpapi.Handler(pool.Ping, cfg.DatabaseTimeout), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	logger.Info("API listening", "address", listener.Addr().String())
	select {
	case err := <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
		logger.Info("API shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return fmt.Errorf("shutdown: %w", err)
		}
		return nil
	}
}
