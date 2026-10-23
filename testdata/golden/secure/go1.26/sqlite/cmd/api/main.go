//go:generate go tool codebuild build

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"example.com/secure/internal/app"
	"example.com/secure/internal/platform/config"
	"example.com/secure/internal/platform/database"
	"example.com/secure/internal/platform/httpx"
	"example.com/secure/internal/platform/telemetry"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	logger := slog.New(httpx.NewLogHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})))
	telemetryConfig, err := telemetry.LoadConfig(os.Getenv, "api")
	if err != nil {
		return err
	}
	cors, err := httpx.LoadCORS(os.Getenv)
	if err != nil {
		return err
	}
	tel, err := telemetry.Start(telemetryConfig, logger)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := database.Open(ctx, database.Options{
		DSN:             cfg.DatabaseURL,
		MaxOpenConns:    cfg.DBMaxOpenConns,
		MaxIdleConns:    cfg.DBMaxIdleConns,
		ConnMaxLifetime: cfg.DBConnMaxLifetime,
	})
	if err != nil {
		return err
	}
	defer db.Close()
	telemetry.ObserveDB(db)

	mux := http.NewServeMux()
	health := httpx.NewHealth(db, logger)
	health.RegisterRoutes(mux)
	if err := app.RegisterRoutes(mux, db, logger); err != nil {
		return err
	}

	var handler http.Handler = httpx.CORS(cors, mux)
	handler = httpx.AccessLog(logger, handler)
	handler = httpx.Recover(logger, handler)
	handler = telemetry.HTTP(mux, handler, httpx.LivenessPath, httpx.ReadinessPath)
	handler = httpx.RequestID(handler)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server listening", slog.String("addr", cfg.HTTPAddr))
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
	}

	logger.Info("shutting down", slog.Duration("delay", cfg.ShutdownDelay))
	health.Drain()
	time.Sleep(cfg.ShutdownDelay)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return tel.Shutdown(shutdownCtx)
}
