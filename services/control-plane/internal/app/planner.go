package app

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/supplymize/supplymize/services/control-plane/internal/domain"
	"github.com/supplymize/supplymize/services/control-plane/internal/requestid"
)

type ForecastInput struct {
	SKU     string
	Horizon int
	History []domain.DemandObservation
	Signals domain.PlanningSignals
}

type OptimizationInput struct {
	SKU              string
	Demand           float64
	ForecastPoints   []domain.ForecastPoint
	Network          domain.SupplyNetwork
	LostSalePenalty  float64
	CarbonPricePerKg float64
}

type Forecaster interface {
	Forecast(context.Context, ForecastInput) (domain.Forecast, error)
}

type Optimizer interface {
	Optimize(context.Context, OptimizationInput) (domain.Optimization, error)
}

type IDGenerator func(prefix string) (string, error)
type Clock func() time.Time

type Planner struct {
	forecaster Forecaster
	optimizer  Optimizer
	newID      IDGenerator
	now        Clock
}

func NewPlanner(forecaster Forecaster, optimizer Optimizer) *Planner {
	return NewPlannerWithDependencies(forecaster, optimizer, requestid.New, time.Now)
}

// NewPlannerWithDependencies exposes deterministic seams for tests without
// imposing a dependency-injection framework on production code.
func NewPlannerWithDependencies(forecaster Forecaster, optimizer Optimizer, newID IDGenerator, now Clock) *Planner {
	return &Planner{
		forecaster: forecaster,
		optimizer:  optimizer,
		newID:      newID,
		now:        now,
	}
}

func (p *Planner) Plan(ctx context.Context, request domain.PlanRequest) (domain.SupplyPlan, error) {
	if err := request.Validate(); err != nil {
		return domain.SupplyPlan{}, err
	}
	if p == nil || p.forecaster == nil || p.optimizer == nil || p.newID == nil || p.now == nil {
		return domain.SupplyPlan{}, errors.New("planner is not configured")
	}

	forecast, err := p.forecaster.Forecast(ctx, ForecastInput{
		SKU:     strings.TrimSpace(request.SKU),
		Horizon: request.Horizon,
		History: request.History,
		Signals: request.Signals,
	})
	if err != nil {
		return domain.SupplyPlan{}, &DependencyError{Stage: "forecast", Err: err, Retryable: isRetryable(err)}
	}
	if err := validateForecast(forecast); err != nil {
		return domain.SupplyPlan{}, &DependencyError{Stage: "forecast", Err: err}
	}

	optimization := domain.Optimization{Allocations: []domain.Allocation{}, Explanations: []string{}}
	if forecast.TotalDemand == 0 {
		optimization.Explanations = append(optimization.Explanations, "Forecast demand is zero; no network allocation is required.")
	} else {
		optimization, err = p.optimizer.Optimize(ctx, OptimizationInput{
			SKU:              strings.TrimSpace(request.SKU),
			Demand:           forecast.TotalDemand,
			ForecastPoints:   forecast.Points,
			Network:          request.Network,
			LostSalePenalty:  request.LostSalePenalty,
			CarbonPricePerKg: request.CarbonPricePerKg,
		})
		if err != nil {
			return domain.SupplyPlan{}, &DependencyError{Stage: "optimization", Err: err, Retryable: isRetryable(err)}
		}
	}
	if err := validateOptimization(optimization, forecast.TotalDemand); err != nil {
		return domain.SupplyPlan{}, &DependencyError{Stage: "optimization", Err: err}
	}

	planID, err := p.newID("plan")
	if err != nil {
		return domain.SupplyPlan{}, fmt.Errorf("generate plan ID: %w", err)
	}

	return domain.SupplyPlan{
		PlanID:       planID,
		RequestID:    requestid.FromContext(ctx),
		SKU:          strings.TrimSpace(request.SKU),
		GeneratedAt:  p.now().UTC(),
		Forecast:     forecast,
		Optimization: optimization,
	}, nil
}

// DependencyError names the failed orchestration stage without exposing an
// infrastructure-specific error type to the HTTP layer.
type DependencyError struct {
	Stage     string
	Retryable bool
	Err       error
}

func (e *DependencyError) Error() string {
	return fmt.Sprintf("%s dependency failed: %v", e.Stage, e.Err)
}

func (e *DependencyError) Unwrap() error { return e.Err }

type retryableError interface {
	RetryableError() bool
}

func isRetryable(err error) bool {
	var retryable retryableError
	return errors.As(err, &retryable) && retryable.RetryableError()
}

func validateForecast(forecast domain.Forecast) error {
	if !nonNegativeFinite(forecast.TotalDemand) {
		return errors.New("forecast returned invalid total demand")
	}
	if forecast.Confidence < 0 || forecast.Confidence > 1 || !finite(forecast.Confidence) {
		return errors.New("forecast returned invalid confidence")
	}
	if len(forecast.Points) == 0 {
		return errors.New("forecast returned no points")
	}
	for i, point := range forecast.Points {
		if strings.TrimSpace(point.Period) == "" {
			return fmt.Errorf("forecast point %d has no period", i)
		}
		if !nonNegativeFinite(point.Demand) || !nonNegativeFinite(point.LowerBound) || !nonNegativeFinite(point.UpperBound) {
			return fmt.Errorf("forecast point %d contains a non-finite or negative value", i)
		}
		if point.LowerBound > point.Demand || point.Demand > point.UpperBound {
			return fmt.Errorf("forecast point %d has inconsistent confidence bounds", i)
		}
	}
	return nil
}

func validateOptimization(optimization domain.Optimization, expectedDemand float64) error {
	values := []float64{
		optimization.FulfilledDemand,
		optimization.UnfulfilledDemand,
		optimization.CarbonKg,
		optimization.Costs.Procurement,
		optimization.Costs.Transport,
		optimization.Costs.Holding,
		optimization.Costs.LostSales,
		optimization.Costs.Carbon,
		optimization.Costs.Total,
	}
	for _, value := range values {
		if !nonNegativeFinite(value) {
			return errors.New("optimizer returned a non-finite or negative summary value")
		}
	}
	for i, allocation := range optimization.Allocations {
		if strings.TrimSpace(allocation.From) == "" || strings.TrimSpace(allocation.To) == "" {
			return fmt.Errorf("optimizer allocation %d has an empty endpoint", i)
		}
		if !nonNegativeFinite(allocation.Quantity) || !nonNegativeFinite(allocation.UnitCost) || !nonNegativeFinite(allocation.CarbonKg) {
			return fmt.Errorf("optimizer allocation %d contains a non-finite or negative value", i)
		}
	}
	accounted := optimization.FulfilledDemand + optimization.UnfulfilledDemand
	tolerance := max(1e-6, expectedDemand*1e-6)
	if abs(accounted-expectedDemand) > tolerance {
		return fmt.Errorf("optimizer accounted for %.4f units; expected %.4f", accounted, expectedDemand)
	}
	return nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func nonNegativeFinite(value float64) bool {
	return finite(value) && value >= 0
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
