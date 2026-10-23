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

	"example.com/shop/gateway/internal/app"
	"example.com/shop/gateway/internal/platform/auth"
	"example.com/shop/gateway/internal/platform/config"
	"example.com/shop/gateway/internal/platform/grpcx"
	"example.com/shop/gateway/internal/platform/httpx"
	"example.com/shop/gateway/internal/platform/telemetry"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := httpx.Probe(os.Getenv("HTTP_ADDR")); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
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
	tlsConfig, err := grpcx.LoadTLSConfig(os.Getenv)
	if err != nil {
		return err
	}
	key, err := auth.LoadPrivateKey(cfg.InternalTokenKeyFile)
	if err != nil {
		return err
	}
	signer := auth.NewSigner(key)
	telemetryConfig, err := telemetry.LoadConfig(os.Getenv, "gateway")
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

	backends, err := app.Dial(os.Getenv, tlsConfig)
	if err != nil {
		return err
	}
	defer backends.Close()

	mux := http.NewServeMux()
	health := httpx.NewHealth(logger)
	health.RegisterRoutes(mux)
	if err := app.RegisterRoutes(mux, backends, signer, logger); err != nil {
		return err
	}

	var handler http.Handler = auth.Anonymous(signer, logger, mux)
	handler = httpx.CORS(cors, handler)
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
