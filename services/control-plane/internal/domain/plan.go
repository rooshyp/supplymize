package domain

import (
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	MinHistory = 7
	MaxHorizon = 365
	MaxHistory = 10_000
	MaxNodes   = 1_000
	MaxLanes   = 10_000
)

// DemandObservation captures actual demand for one ordered business period.
// Period is intentionally a string: clients can use ISO dates, ISO weeks, or
// another stable period identifier without leaking calendar policy into the API.
type DemandObservation struct {
	Period string  `json:"period"`
	Units  float64 `json:"units"`
}

// PlanningSignals are normalized exogenous features forwarded to the forecast
// engine. Zero is a valid neutral value for every signal.
type PlanningSignals struct {
	Trend         float64   `json:"trend"`
	Seasonality   []float64 `json:"seasonality"`
	Weather       float64   `json:"weather"`
	Event         float64   `json:"event"`
	SupplierSpeed float64   `json:"supplier_speed"`
}

// SupplyNode is a capacity-bearing point in the supply network.
type SupplyNode struct {
	ID                    string  `json:"id"`
	Type                  string  `json:"type"`
	Capacity              float64 `json:"capacity"`
	Available             float64 `json:"available"`
	DemandShare           float64 `json:"demand_share,omitempty"`
	StorageCostPerUnitDay float64 `json:"storage_cost_per_unit_day,omitempty"`
}

type NodeRole string

const (
	NodeRoleSupplier  NodeRole = "supplier"
	NodeRoleWarehouse NodeRole = "warehouse"
)

// Role maps friendly public node types onto the optimizer's two-tier network.
func (n SupplyNode) Role() (NodeRole, bool) {
	switch strings.ToLower(strings.TrimSpace(n.Type)) {
	case "supplier", "factory", "manufacturer", "plant":
		return NodeRoleSupplier, true
	case "warehouse", "distribution_center", "distribution-centre", "dc", "customer", "store":
		return NodeRoleWarehouse, true
	default:
		return "", false
	}
}

// SupplyLane describes a directed movement option between two nodes.
type SupplyLane struct {
	From            string  `json:"from"`
	To              string  `json:"to"`
	Capacity        float64 `json:"capacity"`
	UnitCost        float64 `json:"unit_cost"`
	LeadTimePeriods int     `json:"lead_time_periods"`
	CarbonKgPerUnit float64 `json:"carbon_kg_per_unit"`
}

type SupplyNetwork struct {
	Nodes []SupplyNode `json:"nodes"`
	Lanes []SupplyLane `json:"lanes"`
}

// PlanRequest is the public, combined planning contract. It contains the
// demand evidence required by forecasting and the network required by the
// optimizer so the caller receives one atomic planning result.
type PlanRequest struct {
	SKU              string              `json:"sku"`
	Horizon          int                 `json:"horizon"`
	History          []DemandObservation `json:"history"`
	Signals          PlanningSignals     `json:"signals"`
	Network          SupplyNetwork       `json:"network"`
	LostSalePenalty  float64             `json:"lost_sale_penalty"`
	CarbonPricePerKg float64             `json:"carbon_price_per_kg"`
}

type ForecastPoint struct {
	Period     string  `json:"period"`
	Demand     float64 `json:"demand"`
	LowerBound float64 `json:"lower_bound"`
	UpperBound float64 `json:"upper_bound"`
}

type Forecast struct {
	Model       string          `json:"model"`
	TotalDemand float64         `json:"total_demand"`
	Confidence  float64         `json:"confidence"`
	Points      []ForecastPoint `json:"points"`
}

type Allocation struct {
	From       string  `json:"from"`
	To         string  `json:"to"`
	Quantity   float64 `json:"quantity"`
	UnitCost   float64 `json:"unit_cost"`
	CarbonKg   float64 `json:"carbon_kg"`
	Period     string  `json:"period,omitempty"`
	LaneReason string  `json:"reason,omitempty"`
}

type CostBreakdown struct {
	Procurement float64 `json:"procurement"`
	Transport   float64 `json:"transport"`
	Holding     float64 `json:"holding"`
	LostSales   float64 `json:"lost_sales"`
	Carbon      float64 `json:"carbon"`
	Total       float64 `json:"total"`
}

type Optimization struct {
	Allocations       []Allocation  `json:"allocations"`
	FulfilledDemand   float64       `json:"fulfilled_demand"`
	UnfulfilledDemand float64       `json:"unfulfilled_demand"`
	Costs             CostBreakdown `json:"costs"`
	CarbonKg          float64       `json:"carbon_kg"`
	Explanations      []string      `json:"explanations"`
}

type SupplyPlan struct {
	PlanID       string       `json:"plan_id"`
	RequestID    string       `json:"request_id"`
	SKU          string       `json:"sku"`
	GeneratedAt  time.Time    `json:"generated_at"`
	Forecast     Forecast     `json:"forecast"`
	Optimization Optimization `json:"optimization"`
}

type FieldViolation struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationError groups all independently detectable input defects, allowing
// API clients to correct a request in one round trip.
type ValidationError struct {
	Violations []FieldViolation `json:"violations"`
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("request validation failed with %d violation(s)", len(e.Violations))
}

func (r PlanRequest) Validate() error {
	violations := make([]FieldViolation, 0)
	add := func(field, message string) {
		violations = append(violations, FieldViolation{Field: field, Message: message})
	}

	if strings.TrimSpace(r.SKU) == "" {
		add("sku", "must not be blank")
	} else if len(r.SKU) > 128 {
		add("sku", "must be at most 128 characters")
	}
	if r.Horizon < 1 || r.Horizon > MaxHorizon {
		add("horizon", fmt.Sprintf("must be between 1 and %d", MaxHorizon))
	}
	if len(r.History) < MinHistory {
		add("history", fmt.Sprintf("must contain at least %d observations", MinHistory))
	} else if len(r.History) > MaxHistory {
		add("history", fmt.Sprintf("must contain at most %d observations", MaxHistory))
	}
	periods := make(map[string]struct{}, len(r.History))
	for i, observation := range r.History {
		prefix := fmt.Sprintf("history[%d]", i)
		period := strings.TrimSpace(observation.Period)
		if period == "" {
			add(prefix+".period", "must not be blank")
		} else if _, exists := periods[period]; exists {
			add(prefix+".period", "must be unique")
		} else {
			periods[period] = struct{}{}
		}
		if !isNonNegativeFinite(observation.Units) {
			add(prefix+".units", "must be a finite number greater than or equal to 0")
		}
	}

	signalValues := []struct {
		name  string
		value float64
	}{
		{"signals.trend", r.Signals.Trend},
		{"signals.weather", r.Signals.Weather},
		{"signals.event", r.Signals.Event},
		{"signals.supplier_speed", r.Signals.SupplierSpeed},
	}
	for _, signal := range signalValues {
		if !isFinite(signal.value) {
			add(signal.name, "must be a finite number")
		}
	}
	if r.Signals.Trend < -0.25 || r.Signals.Trend > 0.25 {
		add("signals.trend", "must be between -0.25 and 0.25")
	}
	if len(r.Signals.Seasonality) == 1 || len(r.Signals.Seasonality) > 366 {
		add("signals.seasonality", "must be empty or contain between 2 and 366 factors")
	}
	for i, factor := range r.Signals.Seasonality {
		if !isFinite(factor) || factor <= 0 || factor > 5 {
			add(fmt.Sprintf("signals.seasonality[%d]", i), "must be a finite number greater than 0 and at most 5")
		}
	}
	for _, signal := range []struct {
		name  string
		value float64
	}{
		{"signals.weather", r.Signals.Weather},
		{"signals.event", r.Signals.Event},
		{"signals.supplier_speed", r.Signals.SupplierSpeed},
	} {
		if signal.value < -1 || signal.value > 1 {
			add(signal.name, "must be between -1 and 1")
		}
	}

	if len(r.Network.Nodes) == 0 {
		add("network.nodes", "must contain at least one node")
	} else if len(r.Network.Nodes) > MaxNodes {
		add("network.nodes", fmt.Sprintf("must contain at most %d nodes", MaxNodes))
	}
	nodeIDs := make(map[string]struct{}, len(r.Network.Nodes))
	nodeRoles := make(map[string]NodeRole, len(r.Network.Nodes))
	demandShareTotal := 0.0
	demandShareProvided := false
	supplierCount := 0
	warehouseCount := 0
	for i, node := range r.Network.Nodes {
		prefix := fmt.Sprintf("network.nodes[%d]", i)
		id := strings.TrimSpace(node.ID)
		if id == "" {
			add(prefix+".id", "must not be blank")
		} else if _, exists := nodeIDs[id]; exists {
			add(prefix+".id", "must be unique")
		} else {
			nodeIDs[id] = struct{}{}
		}
		role, roleOK := node.Role()
		if !roleOK {
			add(prefix+".type", "must identify a supplier/factory or warehouse/distribution center")
		} else {
			nodeRoles[id] = role
			if role == NodeRoleWarehouse {
				warehouseCount++
				demandShareTotal += node.DemandShare
				if node.DemandShare > 0 {
					demandShareProvided = true
				}
			} else {
				supplierCount++
				if node.DemandShare != 0 {
					add(prefix+".demand_share", "is only valid for warehouse nodes")
				}
			}
		}
		if !isNonNegativeFinite(node.Capacity) {
			add(prefix+".capacity", "must be a finite number greater than or equal to 0")
		}
		if !isNonNegativeFinite(node.Available) {
			add(prefix+".available", "must be a finite number greater than or equal to 0")
		}
		if isFinite(node.Capacity) && isFinite(node.Available) && node.Available > node.Capacity {
			add(prefix+".available", "must not exceed capacity")
		}
		if !isNonNegativeFinite(node.DemandShare) {
			add(prefix+".demand_share", "must be a finite number greater than or equal to 0")
		}
		if !isNonNegativeFinite(node.StorageCostPerUnitDay) {
			add(prefix+".storage_cost_per_unit_day", "must be a finite number greater than or equal to 0")
		}
	}
	if warehouseCount == 0 && len(r.Network.Nodes) > 0 {
		add("network.nodes", "must contain at least one warehouse or distribution-center node")
	}
	if supplierCount == 0 && len(r.Network.Nodes) > 0 {
		add("network.nodes", "must contain at least one supplier or factory node")
	}
	if demandShareProvided && absolute(demandShareTotal-1) > 1e-6 {
		add("network.nodes.demand_share", "warehouse demand shares must sum to 1")
	}
	if len(r.Network.Lanes) > MaxLanes {
		add("network.lanes", fmt.Sprintf("must contain at most %d lanes", MaxLanes))
	}
	lanes := make(map[string]struct{}, len(r.Network.Lanes))
	for i, lane := range r.Network.Lanes {
		prefix := fmt.Sprintf("network.lanes[%d]", i)
		from, to := strings.TrimSpace(lane.From), strings.TrimSpace(lane.To)
		if from == "" {
			add(prefix+".from", "must not be blank")
		} else if _, exists := nodeIDs[from]; !exists {
			add(prefix+".from", "must reference an existing node")
		}
		if to == "" {
			add(prefix+".to", "must not be blank")
		} else if _, exists := nodeIDs[to]; !exists {
			add(prefix+".to", "must reference an existing node")
		}
		if from != "" && from == to {
			add(prefix, "must connect two different nodes")
		}
		if fromRole, exists := nodeRoles[from]; exists && fromRole != NodeRoleSupplier {
			add(prefix+".from", "must reference a supplier or factory node")
		}
		if toRole, exists := nodeRoles[to]; exists && toRole != NodeRoleWarehouse {
			add(prefix+".to", "must reference a warehouse or distribution-center node")
		}
		laneKey := from + "\x00" + to
		if _, exists := lanes[laneKey]; exists {
			add(prefix, "must not duplicate a directed lane")
		} else {
			lanes[laneKey] = struct{}{}
		}
		if !isPositiveFinite(lane.Capacity) {
			add(prefix+".capacity", "must be a finite number greater than 0")
		}
		if !isNonNegativeFinite(lane.UnitCost) {
			add(prefix+".unit_cost", "must be a finite number greater than or equal to 0")
		}
		if lane.LeadTimePeriods < 0 || lane.LeadTimePeriods > MaxHorizon {
			add(prefix+".lead_time_periods", fmt.Sprintf("must be between 0 and %d", MaxHorizon))
		}
		if !isNonNegativeFinite(lane.CarbonKgPerUnit) {
			add(prefix+".carbon_kg_per_unit", "must be a finite number greater than or equal to 0")
		}
	}
	if !isNonNegativeFinite(r.LostSalePenalty) {
		add("lost_sale_penalty", "must be a finite number greater than or equal to 0")
	}
	if !isNonNegativeFinite(r.CarbonPricePerKg) {
		add("carbon_price_per_kg", "must be a finite number greater than or equal to 0")
	}

	if len(violations) > 0 {
		return &ValidationError{Violations: violations}
	}
	return nil
}

func isFinite(value float64) bool {
	return !math.IsInf(value, 0) && !math.IsNaN(value)
}

func isNonNegativeFinite(value float64) bool {
	return isFinite(value) && value >= 0
}

func isPositiveFinite(value float64) bool {
	return isFinite(value) && value > 0
}

func absolute(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}
