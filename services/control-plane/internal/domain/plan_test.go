package domain

import (
	"errors"
	"math"
	"testing"
)

func TestPlanRequestValidate(t *testing.T) {
	t.Run("accepts a valid planning request", func(t *testing.T) {
		if err := validPlanRequest().Validate(); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})

	t.Run("returns all independently detectable violations", func(t *testing.T) {
		request := validPlanRequest()
		request.SKU = " "
		request.Horizon = 0
		request.History = []DemandObservation{{Period: "p1", Units: -1}, {Period: "p1", Units: math.NaN()}}
		request.Signals.Weather = math.Inf(1)
		request.Network.Nodes = []SupplyNode{
			{ID: "n1", Type: "", Capacity: 2, Available: 3},
			{ID: "n1", Type: "customer", Capacity: -1, Available: 0},
		}
		request.Network.Lanes = []SupplyLane{
			{From: "missing", To: "missing", Capacity: 0, UnitCost: -1, LeadTimePeriods: 366, CarbonKgPerUnit: -1},
		}
		request.LostSalePenalty = -1
		request.CarbonPricePerKg = math.Inf(-1)

		err := request.Validate()
		var validation *ValidationError
		if !errors.As(err, &validation) {
			t.Fatalf("Validate() error = %v, want *ValidationError", err)
		}
		fields := make(map[string]bool, len(validation.Violations))
		for _, violation := range validation.Violations {
			fields[violation.Field] = true
		}
		for _, field := range []string{
			"sku", "horizon", "history[0].units", "history[1].period", "history[1].units",
			"signals.weather", "network.nodes[0].type", "network.nodes[0].available",
			"network.nodes[1].id", "network.nodes[1].capacity", "network.lanes[0].from",
			"network.lanes[0].to", "network.lanes[0]", "network.lanes[0].capacity",
			"network.lanes[0].unit_cost", "network.lanes[0].lead_time_periods",
			"network.lanes[0].carbon_kg_per_unit", "lost_sale_penalty", "carbon_price_per_kg",
		} {
			if !fields[field] {
				t.Errorf("missing violation for %q; got %#v", field, validation.Violations)
			}
		}
	})

	t.Run("rejects a lane with the same endpoints", func(t *testing.T) {
		request := validPlanRequest()
		request.Network.Lanes[0].To = request.Network.Lanes[0].From
		if err := request.Validate(); err == nil {
			t.Fatal("Validate() error = nil, want validation error")
		}
	})
}

func validPlanRequest() PlanRequest {
	return PlanRequest{
		SKU:     "SKU-001",
		Horizon: 2,
		History: []DemandObservation{
			{Period: "2026-W01", Units: 8},
			{Period: "2026-W02", Units: 10},
			{Period: "2026-W03", Units: 11},
			{Period: "2026-W04", Units: 12},
			{Period: "2026-W05", Units: 11},
			{Period: "2026-W06", Units: 13},
			{Period: "2026-W07", Units: 14},
		},
		Signals: PlanningSignals{Trend: 0.1, Seasonality: []float64{0.9, 1.1}, SupplierSpeed: 1},
		Network: SupplyNetwork{
			Nodes: []SupplyNode{
				{ID: "plant", Type: "factory", Capacity: 100, Available: 80},
				{ID: "store", Type: "customer", Capacity: 100, Available: 0},
			},
			Lanes: []SupplyLane{
				{From: "plant", To: "store", Capacity: 50, UnitCost: 1.5, LeadTimePeriods: 1, CarbonKgPerUnit: 0.2},
			},
		},
		LostSalePenalty:  15,
		CarbonPricePerKg: 0.1,
	}
}
