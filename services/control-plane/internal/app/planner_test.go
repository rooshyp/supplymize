package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/supplymize/supplymize/services/control-plane/internal/domain"
	"github.com/supplymize/supplymize/services/control-plane/internal/requestid"
)

type forecasterFunc func(context.Context, ForecastInput) (domain.Forecast, error)

func (function forecasterFunc) Forecast(ctx context.Context, input ForecastInput) (domain.Forecast, error) {
	return function(ctx, input)
}

type optimizerFunc func(context.Context, OptimizationInput) (domain.Optimization, error)

func (function optimizerFunc) Optimize(ctx context.Context, input OptimizationInput) (domain.Optimization, error) {
	return function(ctx, input)
}

type testRetryableError struct{}

func (testRetryableError) Error() string        { return "temporarily unavailable" }
func (testRetryableError) RetryableError() bool { return true }

func TestPlannerPlanOrchestratesDependencies(t *testing.T) {
	request := appValidRequest()
	forecast := appValidForecast()
	optimization := appValidOptimization()
	fixedTime := time.Date(2026, time.August, 8, 12, 30, 0, 0, time.FixedZone("test", -7*60*60))

	forecastCalled := false
	forecaster := forecasterFunc(func(_ context.Context, input ForecastInput) (domain.Forecast, error) {
		forecastCalled = true
		if input.SKU != request.SKU || input.Horizon != request.Horizon {
			t.Fatalf("forecast input = %#v", input)
		}
		return forecast, nil
	})
	optimizerCalled := false
	optimizer := optimizerFunc(func(_ context.Context, input OptimizationInput) (domain.Optimization, error) {
		optimizerCalled = true
		if input.Demand != forecast.TotalDemand {
			t.Fatalf("optimizer demand = %v, want %v", input.Demand, forecast.TotalDemand)
		}
		if len(input.ForecastPoints) != len(forecast.Points) || len(input.Network.Nodes) != len(request.Network.Nodes) {
			t.Fatalf("optimizer input = %#v", input)
		}
		return optimization, nil
	})
	planner := NewPlannerWithDependencies(forecaster, optimizer, func(prefix string) (string, error) {
		if prefix != "plan" {
			t.Fatalf("ID prefix = %q", prefix)
		}
		return "plan_test", nil
	}, func() time.Time { return fixedTime })
	ctx := requestid.WithContext(context.Background(), "req_test")

	plan, err := planner.Plan(ctx, request)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if !forecastCalled || !optimizerCalled {
		t.Fatalf("dependency calls: forecast=%t optimizer=%t", forecastCalled, optimizerCalled)
	}
	if plan.PlanID != "plan_test" || plan.RequestID != "req_test" || plan.SKU != request.SKU {
		t.Fatalf("Plan() identity fields = %#v", plan)
	}
	if !plan.GeneratedAt.Equal(fixedTime.UTC()) {
		t.Fatalf("GeneratedAt = %v, want %v", plan.GeneratedAt, fixedTime.UTC())
	}
}

func TestPlannerPlanValidatesBeforeCallingDependencies(t *testing.T) {
	called := false
	forecaster := forecasterFunc(func(context.Context, ForecastInput) (domain.Forecast, error) {
		called = true
		return domain.Forecast{}, nil
	})
	planner := NewPlanner(forecaster, optimizerFunc(func(context.Context, OptimizationInput) (domain.Optimization, error) {
		called = true
		return domain.Optimization{}, nil
	}))
	request := appValidRequest()
	request.SKU = ""

	_, err := planner.Plan(context.Background(), request)
	var validation *domain.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("Plan() error = %v, want validation error", err)
	}
	if called {
		t.Fatal("dependency called for invalid request")
	}
}

func TestPlannerPlanClassifiesRetryableDependencyFailure(t *testing.T) {
	planner := NewPlanner(
		forecasterFunc(func(context.Context, ForecastInput) (domain.Forecast, error) {
			return domain.Forecast{}, testRetryableError{}
		}),
		optimizerFunc(func(context.Context, OptimizationInput) (domain.Optimization, error) {
			t.Fatal("optimizer called after forecast failed")
			return domain.Optimization{}, nil
		}),
	)

	_, err := planner.Plan(context.Background(), appValidRequest())
	var dependency *DependencyError
	if !errors.As(err, &dependency) {
		t.Fatalf("Plan() error = %v, want dependency error", err)
	}
	if dependency.Stage != "forecast" || !dependency.Retryable {
		t.Fatalf("dependency error = %#v", dependency)
	}
}

func TestPlannerPlanRejectsInvalidDependencyResults(t *testing.T) {
	tests := []struct {
		name      string
		forecast  domain.Forecast
		optimizer domain.Optimization
		wantStage string
	}{
		{
			name:      "empty forecast",
			forecast:  domain.Forecast{TotalDemand: 10, Confidence: 0.9},
			optimizer: appValidOptimization(),
			wantStage: "forecast",
		},
		{
			name:      "unaccounted optimization demand",
			forecast:  appValidForecast(),
			optimizer: domain.Optimization{FulfilledDemand: 1, UnfulfilledDemand: 1},
			wantStage: "optimization",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			planner := NewPlanner(
				forecasterFunc(func(context.Context, ForecastInput) (domain.Forecast, error) { return test.forecast, nil }),
				optimizerFunc(func(context.Context, OptimizationInput) (domain.Optimization, error) { return test.optimizer, nil }),
			)
			_, err := planner.Plan(context.Background(), appValidRequest())
			var dependency *DependencyError
			if !errors.As(err, &dependency) || dependency.Stage != test.wantStage {
				t.Fatalf("Plan() error = %#v, want %s dependency error", err, test.wantStage)
			}
		})
	}
}

func appValidRequest() domain.PlanRequest {
	return domain.PlanRequest{
		SKU:     "SKU-001",
		Horizon: 2,
		History: []domain.DemandObservation{
			{Period: "p1", Units: 8},
			{Period: "p2", Units: 10},
			{Period: "p3", Units: 11},
			{Period: "p4", Units: 9},
			{Period: "p5", Units: 12},
			{Period: "p6", Units: 13},
			{Period: "p7", Units: 14},
		},
		Network: domain.SupplyNetwork{
			Nodes: []domain.SupplyNode{
				{ID: "factory", Type: "factory", Capacity: 100, Available: 50},
				{ID: "store", Type: "customer", Capacity: 100, Available: 0},
			},
			Lanes: []domain.SupplyLane{{From: "factory", To: "store", Capacity: 100}},
		},
		LostSalePenalty: 10,
	}
}

func appValidForecast() domain.Forecast {
	return domain.Forecast{
		Model:       "ensemble-v1",
		TotalDemand: 20,
		Confidence:  0.9,
		Points: []domain.ForecastPoint{
			{Period: "p3", Demand: 9, LowerBound: 8, UpperBound: 10},
			{Period: "p4", Demand: 11, LowerBound: 10, UpperBound: 12},
		},
	}
}

func appValidOptimization() domain.Optimization {
	return domain.Optimization{
		Allocations:       []domain.Allocation{{From: "factory", To: "store", Quantity: 19}},
		FulfilledDemand:   19,
		UnfulfilledDemand: 1,
		Costs:             domain.CostBreakdown{Transport: 19, LostSales: 10, Total: 29},
		CarbonKg:          2,
		Explanations:      []string{"capacity constrained"},
	}
}
