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
	metrics := api.Metrics()

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

				now := time.Now()

				results, err := reaper.ReapExpiredAttempts(ctx, now, 100)
				if err != nil {
					logger.Error("expiry reaper error", "error", err)
					continue
				}
				if len(results) > 0 {
					logger.Info("expiry reaper processed attempts", "count", len(results))
				}

				for _, r := range results {
					team, _ := reaper.GetTeamByID(ctx, r.TeamID)
					app, _ := reaper.GetAppByIDDirect(ctx, r.AppID)
					teamSlug := ""
					appSlug := ""
					if team != nil {
						teamSlug = team.Slug
					}
					if app != nil {
						appSlug = app.Slug
					}

					switch r.Outcome {
					case "retried":
						metrics.RunRetried(teamSlug, appSlug)
					case "dead", "cancelled":
						metrics.RunCompleted(teamSlug, appSlug, r.Outcome)
					}
				}

				// Mark long-inactive runners offline so admin visibility reflects
				// current availability and stale tokens are fenced.
				offlineThreshold := now.Add(-(2 * cfg.LeaseTTL))
				marked, err := reaper.MarkStaleRunnersOffline(ctx, offlineThreshold)
				if err != nil {
					logger.Error("runner offline sweep error", "error", err)
					continue
				}
				if marked > 0 {
					logger.Info("marked stale runners offline", "count", marked)
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
