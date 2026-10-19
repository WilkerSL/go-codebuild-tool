package config

import (
	"errors"
	"fmt"
	"log/slog"
	"time"
)

type Config struct {
	HTTPAddr             string
	InternalTokenKeyFile string
	LogLevel             slog.Level
	ReadHeaderTimeout    time.Duration
	ReadTimeout          time.Duration
	WriteTimeout         time.Duration
	IdleTimeout          time.Duration
	ShutdownTimeout      time.Duration
	ShutdownDelay        time.Duration
}

func Load(getenv func(string) string) (Config, error) {
	l := loader{getenv: getenv}
	cfg := Config{
		HTTPAddr:             l.string("HTTP_ADDR", ":8080"),
		InternalTokenKeyFile: l.string("INTERNAL_TOKEN_PRIVATE_KEY_FILE", ""),
		ReadHeaderTimeout:    l.duration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
		ReadTimeout:          l.duration("HTTP_READ_TIMEOUT", 15*time.Second),
		WriteTimeout:         l.duration("HTTP_WRITE_TIMEOUT", 15*time.Second),
		IdleTimeout:          l.duration("HTTP_IDLE_TIMEOUT", 60*time.Second),
		ShutdownTimeout:      l.duration("SHUTDOWN_TIMEOUT", 20*time.Second),
		ShutdownDelay:        l.delay("SHUTDOWN_DELAY"),
	}
	if err := cfg.LogLevel.UnmarshalText([]byte(l.string("LOG_LEVEL", "info"))); err != nil {
		l.errs = append(l.errs, fmt.Errorf("LOG_LEVEL: %w", err))
	}
	if cfg.InternalTokenKeyFile == "" {
		l.errs = append(l.errs, errors.New("INTERNAL_TOKEN_PRIVATE_KEY_FILE is required: the gateway signs the internal token every service verifies; generate one with go run ./gateway/cmd/devcerts"))
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
