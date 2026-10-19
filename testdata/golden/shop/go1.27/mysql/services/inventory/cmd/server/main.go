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

	"example.com/shop/services/inventory/internal/app"
	"example.com/shop/services/inventory/internal/platform/auth"
	"example.com/shop/services/inventory/internal/platform/config"
	"example.com/shop/services/inventory/internal/platform/database"
	"example.com/shop/services/inventory/internal/platform/grpcx"
	"example.com/shop/services/inventory/internal/platform/httpx"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := httpx.Probe(os.Getenv("HEALTH_ADDR")); err != nil {
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
	key, err := auth.LoadPublicKey(cfg.InternalTokenKeyFile)
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

	peers, err := app.DialPeers(os.Getenv, tlsConfig)
	if err != nil {
		return err
	}
	defer peers.Close()

	server, err := grpcx.NewServer(tlsConfig, auth.NewVerifier(key), logger)
	if err != nil {
		return err
	}
	if err := app.Register(server, db, peers, logger); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return err
	}

	health := httpx.NewHealth(db, logger)
	mux := http.NewServeMux()
	health.RegisterRoutes(mux)
	healthServer := &http.Server{Addr: cfg.HealthAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	errCh := make(chan error, 2)
	go func() {
		logger.Info("grpc server listening", slog.String("addr", cfg.GRPCAddr))
		errCh <- server.Serve(listener)
	}()
	go func() {
		errCh <- healthServer.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
	}

	logger.Info("shutting down", slog.Duration("delay", cfg.ShutdownDelay))
	health.Drain()
	time.Sleep(cfg.ShutdownDelay)
	stopped := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(cfg.ShutdownTimeout):
		server.Stop()
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	return healthServer.Shutdown(shutdownCtx)
}
