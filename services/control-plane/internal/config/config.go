package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultMaxRequestBytes = int64(1 << 20)
)

type Config struct {
	Environment       string
	HTTPAddress       string
	ForecastURL       string
	OptimizerURL      string
	LogLevel          slog.Level
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	PlanTimeout       time.Duration
	DependencyTimeout time.Duration
	ReadinessTimeout  time.Duration
	MaxRequestBytes   int64
}

func Load() (Config, error) {
	config := Config{
		Environment:       envOrDefault("SUPPLYMIZE_ENV", "development"),
		HTTPAddress:       envOrDefault("SUPPLYMIZE_HTTP_ADDR", ":8080"),
		ForecastURL:       envOrDefault("SUPPLYMIZE_FORECAST_URL", "http://forecasting:8001"),
		OptimizerURL:      envOrDefault("SUPPLYMIZE_OPTIMIZER_URL", "http://optimizer:8002"),
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		ShutdownTimeout:   15 * time.Second,
		PlanTimeout:       20 * time.Second,
		DependencyTimeout: 10 * time.Second,
		ReadinessTimeout:  2 * time.Second,
		MaxRequestBytes:   defaultMaxRequestBytes,
	}

	var err error
	if config.LogLevel, err = parseLogLevel(envOrDefault("SUPPLYMIZE_LOG_LEVEL", "info")); err != nil {
		return Config{}, err
	}
	durations := []struct {
		key    string
		target *time.Duration
	}{
		{"SUPPLYMIZE_READ_TIMEOUT", &config.ReadTimeout},
		{"SUPPLYMIZE_WRITE_TIMEOUT", &config.WriteTimeout},
		{"SUPPLYMIZE_IDLE_TIMEOUT", &config.IdleTimeout},
		{"SUPPLYMIZE_SHUTDOWN_TIMEOUT", &config.ShutdownTimeout},
		{"SUPPLYMIZE_PLAN_TIMEOUT", &config.PlanTimeout},
		{"SUPPLYMIZE_DEPENDENCY_TIMEOUT", &config.DependencyTimeout},
		{"SUPPLYMIZE_READINESS_TIMEOUT", &config.ReadinessTimeout},
	}
	for _, duration := range durations {
		if value, exists := os.LookupEnv(duration.key); exists {
			parsed, parseErr := time.ParseDuration(value)
			if parseErr != nil {
				return Config{}, fmt.Errorf("%s must be a valid duration: %w", duration.key, parseErr)
			}
			*duration.target = parsed
		}
	}
	if value, exists := os.LookupEnv("SUPPLYMIZE_MAX_REQUEST_BYTES"); exists {
		config.MaxRequestBytes, err = strconv.ParseInt(value, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf("SUPPLYMIZE_MAX_REQUEST_BYTES must be an integer: %w", err)
		}
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.HTTPAddress) == "" {
		return fmt.Errorf("SUPPLYMIZE_HTTP_ADDR must not be blank")
	}
	if err := validServiceURL("SUPPLYMIZE_FORECAST_URL", c.ForecastURL); err != nil {
		return err
	}
	if err := validServiceURL("SUPPLYMIZE_OPTIMIZER_URL", c.OptimizerURL); err != nil {
		return err
	}
	durations := []struct {
		name  string
		value time.Duration
	}{
		{"SUPPLYMIZE_READ_TIMEOUT", c.ReadTimeout},
		{"SUPPLYMIZE_WRITE_TIMEOUT", c.WriteTimeout},
		{"SUPPLYMIZE_IDLE_TIMEOUT", c.IdleTimeout},
		{"SUPPLYMIZE_SHUTDOWN_TIMEOUT", c.ShutdownTimeout},
		{"SUPPLYMIZE_PLAN_TIMEOUT", c.PlanTimeout},
		{"SUPPLYMIZE_DEPENDENCY_TIMEOUT", c.DependencyTimeout},
		{"SUPPLYMIZE_READINESS_TIMEOUT", c.ReadinessTimeout},
	}
	for _, duration := range durations {
		if duration.value <= 0 {
			return fmt.Errorf("%s must be greater than zero", duration.name)
		}
	}
	if c.WriteTimeout < c.PlanTimeout {
		return fmt.Errorf("SUPPLYMIZE_WRITE_TIMEOUT must be greater than or equal to SUPPLYMIZE_PLAN_TIMEOUT")
	}
	if c.MaxRequestBytes < 1_024 || c.MaxRequestBytes > 64<<20 {
		return fmt.Errorf("SUPPLYMIZE_MAX_REQUEST_BYTES must be between 1024 and 67108864")
	}
	return nil
}

func envOrDefault(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func validServiceURL(name, raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%s must be an absolute HTTP(S) URL", name)
	}
	return nil
}

func parseLogLevel(raw string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("SUPPLYMIZE_LOG_LEVEL must be debug, info, warn, or error")
	}
}
