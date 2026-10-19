package config

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"
)

type Config struct {
	GRPCAddr             string
	HealthAddr           string
	DatabaseURL          string
	InternalTokenKeyFile string
	LogLevel             slog.Level
	ShutdownTimeout      time.Duration
	ShutdownDelay        time.Duration
	DBMaxOpenConns       int
	DBMaxIdleConns       int
	DBConnMaxLifetime    time.Duration
}

func Load(getenv func(string) string) (Config, error) {
	l := loader{getenv: getenv}
	cfg := Config{
		GRPCAddr:             l.string("GRPC_ADDR", ":9000"),
		HealthAddr:           l.string("HEALTH_ADDR", ":8081"),
		DatabaseURL:          l.string("DATABASE_URL", ""),
		InternalTokenKeyFile: l.string("INTERNAL_TOKEN_PUBLIC_KEY_FILE", ""),
		ShutdownTimeout:      l.duration("SHUTDOWN_TIMEOUT", 20*time.Second),
		ShutdownDelay:        l.delay("SHUTDOWN_DELAY"),
		DBMaxOpenConns:       l.int("DB_MAX_OPEN_CONNS", 25),
		DBMaxIdleConns:       l.int("DB_MAX_IDLE_CONNS", 25),
		DBConnMaxLifetime:    l.duration("DB_CONN_MAX_LIFETIME", 30*time.Minute),
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

func (l *loader) delay(key string) time.Duration {
	v := l.getenv(key)
	if v == "" {
		return 0
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		l.errs = append(l.errs, fmt.Errorf("%s must be a non-negative duration such as 5s, got %q", key, v))
		return 0
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
