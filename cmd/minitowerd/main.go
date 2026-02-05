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

	"minitower/internal/config"
	"minitower/internal/db"
	"minitower/internal/httpapi"
	"minitower/internal/migrate"
	"minitower/internal/migrations"
	"minitower/internal/objects"
	"minitower/internal/store"
)

const shutdownTimeout = 10 * time.Second

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config error", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbConn, err := db.Open(ctx, cfg.DBPath)
	if err != nil {
		logger.Error("db error", "error", err)
		os.Exit(1)
	}
	defer func() {
		_ = dbConn.Close()
	}()

	migrator := migrate.New(migrations.FS)
	if err := migrator.Apply(ctx, dbConn); err != nil {
		logger.Error("migration error", "error", err)
		os.Exit(1)
	}

	objectStore, err := objects.NewLocalStore(cfg.ObjectsDir)
	if err != nil {
		logger.Error("objects store error", "error", err)
		os.Exit(1)
	}

	api := httpapi.New(cfg, dbConn, objectStore, logger)

	reaper := store.New(dbConn)
	if cfg.ExpiryCheckInterval > 0 {
		go func() {
			ticker := time.NewTicker(cfg.ExpiryCheckInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}

				processed, err := reaper.ReapExpiredAttempts(ctx, time.Now(), 100)
				if err != nil {
					logger.Error("expiry reaper error", "error", err)
					continue
				}
				if processed > 0 {
					logger.Info("expiry reaper processed attempts", "count", processed)
				}
			}
		}()
	}

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		_ = server.Shutdown(shutdownCtx)
	}()

	logger.Info("minitower listening", "addr", cfg.ListenAddr)

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
