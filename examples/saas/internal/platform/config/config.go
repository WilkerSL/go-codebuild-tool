package config

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr          string
	DatabaseURL       string
	LogLevel          slog.Level
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	DBMaxOpenConns    int
	DBMaxIdleConns    int
	DBConnMaxLifetime time.Duration
}

func Load(getenv func(string) string) (Config, error) {
	l := loader{getenv: getenv}
	cfg := Config{
		HTTPAddr:          l.string("HTTP_ADDR", ":8080"),
		DatabaseURL:       l.string("DATABASE_URL", ""),
		ReadHeaderTimeout: l.duration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
		ReadTimeout:       l.duration("HTTP_READ_TIMEOUT", 15*time.Second),
		WriteTimeout:      l.duration("HTTP_WRITE_TIMEOUT", 15*time.Second),
		IdleTimeout:       l.duration("HTTP_IDLE_TIMEOUT", 60*time.Second),
		ShutdownTimeout:   l.duration("SHUTDOWN_TIMEOUT", 20*time.Second),
		DBMaxOpenConns:    l.int("DB_MAX_OPEN_CONNS", 25),
		DBMaxIdleConns:    l.int("DB_MAX_IDLE_CONNS", 25),
		DBConnMaxLifetime: l.duration("DB_CONN_MAX_LIFETIME", 30*time.Minute),
	}
	if err := cfg.LogLevel.UnmarshalText([]byte(l.string("LOG_LEVEL", "info"))); err != nil {
		l.errs = append(l.errs, fmt.Errorf("LOG_LEVEL: %w", err))
	}
	if cfg.DatabaseURL == "" {
		l.errs = append(l.errs, errors.New("DATABASE_URL is required"))
	}
	return cfg, errors.Join(l.errs...)
}

type loader struct {
	getenv func(string) string
	errs   []error
}

func (l *loader) string(key, fallback string) string {
	if v := l.getenv(key); v != "" {
		return v
	}
	return fallback
}

func (l *loader) duration(key string, fallback time.Duration) time.Duration {
	v := l.getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		l.errs = append(l.errs, fmt.Errorf("%s must be a positive duration such as 15s, got %q", key, v))
		return fallback
	}
	return d
}

func (l *loader) int(key string, fallback int) int {
	v := l.getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		l.errs = append(l.errs, fmt.Errorf("%s must be a non-negative integer, got %q", key, v))
		return fallback
	}
	return n
}
