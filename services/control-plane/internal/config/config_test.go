package config

import (
	"os"
	"testing"
	"time"
)

var configEnvironmentKeys = []string{
	"SUPPLYMIZE_ENV",
	"SUPPLYMIZE_HTTP_ADDR",
	"SUPPLYMIZE_FORECAST_URL",
	"SUPPLYMIZE_OPTIMIZER_URL",
	"SUPPLYMIZE_LOG_LEVEL",
	"SUPPLYMIZE_READ_TIMEOUT",
	"SUPPLYMIZE_WRITE_TIMEOUT",
	"SUPPLYMIZE_IDLE_TIMEOUT",
	"SUPPLYMIZE_SHUTDOWN_TIMEOUT",
	"SUPPLYMIZE_PLAN_TIMEOUT",
	"SUPPLYMIZE_DEPENDENCY_TIMEOUT",
	"SUPPLYMIZE_READINESS_TIMEOUT",
	"SUPPLYMIZE_MAX_REQUEST_BYTES",
}

func TestLoadDefaults(t *testing.T) {
	clearConfigEnvironment(t)
	config, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if config.HTTPAddress != ":8080" || config.ForecastURL != "http://forecasting:8001" || config.OptimizerURL != "http://optimizer:8002" {
		t.Fatalf("Load() service defaults = %#v", config)
	}
	if config.PlanTimeout != 20*time.Second || config.MaxRequestBytes != 1<<20 {
		t.Fatalf("Load() operational defaults = %#v", config)
	}
}

func TestLoadOverrides(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("SUPPLYMIZE_ENV", "production")
	t.Setenv("SUPPLYMIZE_HTTP_ADDR", "127.0.0.1:9000")
	t.Setenv("SUPPLYMIZE_FORECAST_URL", "https://forecast.example.test/api")
	t.Setenv("SUPPLYMIZE_OPTIMIZER_URL", "https://optimizer.example.test")
	t.Setenv("SUPPLYMIZE_LOG_LEVEL", "debug")
	t.Setenv("SUPPLYMIZE_PLAN_TIMEOUT", "12s")
	t.Setenv("SUPPLYMIZE_WRITE_TIMEOUT", "13s")
	t.Setenv("SUPPLYMIZE_MAX_REQUEST_BYTES", "2048")
	config, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if config.Environment != "production" || config.HTTPAddress != "127.0.0.1:9000" || config.PlanTimeout != 12*time.Second || config.MaxRequestBytes != 2048 {
		t.Fatalf("Load() = %#v", config)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "relative URL", key: "SUPPLYMIZE_FORECAST_URL", value: "/forecast"},
		{name: "bad duration", key: "SUPPLYMIZE_PLAN_TIMEOUT", value: "soon"},
		{name: "zero duration", key: "SUPPLYMIZE_READ_TIMEOUT", value: "0s"},
		{name: "bad log level", key: "SUPPLYMIZE_LOG_LEVEL", value: "verbose"},
		{name: "request body too small", key: "SUPPLYMIZE_MAX_REQUEST_BYTES", value: "12"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clearConfigEnvironment(t)
			t.Setenv(test.key, test.value)
			if _, err := Load(); err == nil {
				t.Fatal("Load() error = nil")
			}
		})
	}
}

func clearConfigEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range configEnvironmentKeys {
		value, exists := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("Unsetenv(%q): %v", key, err)
		}
		t.Cleanup(func() {
			if exists {
				_ = os.Setenv(key, value)
			} else {
				_ = os.Unsetenv(key)
			}
		})
	}
}
