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

	"github.com/supplymize/supplymize/services/control-plane/internal/app"
	"github.com/supplymize/supplymize/services/control-plane/internal/config"
	"github.com/supplymize/supplymize/services/control-plane/internal/httpapi"
	"github.com/supplymize/supplymize/services/control-plane/internal/outbound"
)

var version = "development"

func main() {
	config, err := config.Load()
	if err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("configuration rejected", "error", err)
		os.Exit(1)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: config.LogLevel})).With(
		"service", "control-plane",
		"environment", config.Environment,
		"version", version,
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, config, logger); err != nil {
		logger.Error("control-plane stopped with an error", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, config config.Config, logger *slog.Logger) error {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 20
	transport.IdleConnTimeout = 90 * time.Second
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   config.DependencyTimeout,
	}

	forecaster, err := outbound.NewForecastClient(config.ForecastURL, httpClient, logger)
	if err != nil {
		return err
	}
	optimizer, err := outbound.NewOptimizerClient(config.OptimizerURL, httpClient, logger)
	if err != nil {
		return err
	}
	planner := app.NewPlanner(forecaster, optimizer)
	readiness := app.NewReadiness(
		app.NamedChecker{Name: "forecast", Checker: forecaster},
		app.NamedChecker{Name: "optimizer", Checker: optimizer},
	)
	handler := httpapi.NewHandler(planner, readiness, logger, httpapi.Options{
		PlanTimeout:      config.PlanTimeout,
		ReadinessTimeout: config.ReadinessTimeout,
		MaxRequestBytes:  config.MaxRequestBytes,
		Version:          version,
	})
	server := &http.Server{
		Addr:              config.HTTPAddress,
		Handler:           handler,
		ReadHeaderTimeout: config.ReadTimeout,
		ReadTimeout:       config.ReadTimeout,
		WriteTimeout:      config.WriteTimeout,
		IdleTimeout:       config.IdleTimeout,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()
	logger.Info("control-plane listening", "address", config.HTTPAddress)

	select {
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownContext, cancel := context.WithTimeout(context.Background(), config.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		_ = server.Close()
		return err
	}
	if err := <-serverErrors; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	logger.Info("control-plane shutdown complete")
	return nil
}
