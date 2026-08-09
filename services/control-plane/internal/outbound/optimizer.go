package outbound

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/supplymize/supplymize/services/control-plane/internal/app"
	"github.com/supplymize/supplymize/services/control-plane/internal/domain"
)

type OptimizerClient struct {
	client *jsonClient
}

func NewOptimizerClient(baseURL string, httpClient HTTPDoer, logger *slog.Logger) (*OptimizerClient, error) {
	client, err := newJSONClient("optimizer", baseURL, httpClient, logger)
	if err != nil {
		return nil, err
	}
	return &OptimizerClient{client: client}, nil
}

type supplierDTO struct {
	ID            string  `json:"id"`
	CapacityUnits float64 `json:"capacity_units"`
}

type warehouseDTO struct {
	ID                    string  `json:"id"`
	CapacityUnits         float64 `json:"capacity_units"`
	StorageCostPerUnitDay float64 `json:"storage_cost_per_unit_day"`
}

type laneDTO struct {
	ID                     string  `json:"id"`
	SupplierID             string  `json:"supplier_id"`
	WarehouseID            string  `json:"warehouse_id"`
	DistanceKM             float64 `json:"distance_km"`
	TransportCostPerUnitKM float64 `json:"transport_cost_per_unit_km"`
	CarbonKgPerUnitKM      float64 `json:"carbon_kg_per_unit_km"`
	TruckCapacityUnits     float64 `json:"truck_capacity_units"`
	MaxTrucks              int     `json:"max_trucks"`
}

type warehouseDemandDTO struct {
	WarehouseID string  `json:"warehouse_id"`
	Units       float64 `json:"units"`
}

type optimizerRequestDTO struct {
	SKU                string               `json:"sku"`
	Suppliers          []supplierDTO        `json:"suppliers"`
	Warehouses         []warehouseDTO       `json:"warehouses"`
	Lanes              []laneDTO            `json:"lanes"`
	Demand             []warehouseDemandDTO `json:"demand"`
	LostSalePenalty    float64              `json:"lost_sale_penalty"`
	CarbonPricePerKg   float64              `json:"carbon_price_per_kg"`
	AverageStorageDays float64              `json:"average_storage_days"`
	Currency           string               `json:"currency"`
}

type optimizerCostDTO struct {
	Procurement float64 `json:"procurement"`
	Transport   float64 `json:"transport"`
	Storage     float64 `json:"storage"`
	Holding     float64 `json:"holding"`
	LostSales   float64 `json:"lost_sales"`
	Carbon      float64 `json:"carbon"`
	Total       float64 `json:"total"`
}

func (cost optimizerCostDTO) domainCost() domain.CostBreakdown {
	holding := cost.Storage
	if holding == 0 {
		holding = cost.Holding
	}
	return domain.CostBreakdown{
		Procurement: cost.Procurement,
		Transport:   cost.Transport,
		Holding:     holding,
		LostSales:   cost.LostSales,
		Carbon:      cost.Carbon,
		Total:       cost.Total,
	}
}

type allocationDTO struct {
	LaneID      string           `json:"lane_id"`
	SupplierID  string           `json:"supplier_id"`
	WarehouseID string           `json:"warehouse_id"`
	Units       float64          `json:"units"`
	Costs       optimizerCostDTO `json:"costs"`
	From        string           `json:"from"`
	To          string           `json:"to"`
	Source      string           `json:"source"`
	Destination string           `json:"destination"`
	Quantity    float64          `json:"quantity"`
	UnitCost    float64          `json:"unit_cost"`
	CarbonKg    float64          `json:"carbon_kg"`
	Period      string           `json:"period"`
	Reason      string           `json:"reason"`
}

type optimizerPayloadDTO struct {
	Allocations       []allocationDTO  `json:"allocations"`
	FulfilledDemand   float64          `json:"fulfilled_demand"`
	UnfulfilledDemand float64          `json:"unfulfilled_demand"`
	Costs             optimizerCostDTO `json:"costs"`
	CarbonKg          float64          `json:"carbon_kg"`
	Explanations      []string         `json:"explanations"`
}

type optimizerResponseDTO struct {
	optimizerPayloadDTO
	Optimization *optimizerPayloadDTO `json:"optimization"`
}

func (c *OptimizerClient) Optimize(ctx context.Context, input app.OptimizationInput) (domain.Optimization, error) {
	request, err := optimizerRequest(input)
	if err != nil {
		return domain.Optimization{}, err
	}
	var response optimizerResponseDTO
	if err := c.client.do(ctx, http.MethodPost, "/v1/optimize", request, &response); err != nil {
		return domain.Optimization{}, err
	}
	payload := response.optimizerPayloadDTO
	if response.Optimization != nil {
		payload = *response.Optimization
	}
	allocations := make([]domain.Allocation, 0, len(payload.Allocations))
	for _, allocation := range payload.Allocations {
		from := firstString(allocation.SupplierID, allocation.From, allocation.Source)
		to := firstString(allocation.WarehouseID, allocation.To, allocation.Destination)
		quantity := allocation.Units
		if quantity == 0 {
			quantity = allocation.Quantity
		}
		unitCost := allocation.UnitCost
		if unitCost == 0 && quantity > 0 {
			unitCost = allocation.Costs.Total / quantity
		}
		reason := firstString(allocation.Reason, allocation.LaneID)
		allocations = append(allocations, domain.Allocation{
			From:       from,
			To:         to,
			Quantity:   quantity,
			UnitCost:   unitCost,
			CarbonKg:   allocation.CarbonKg,
			Period:     strings.TrimSpace(allocation.Period),
			LaneReason: reason,
		})
	}
	explanations := append([]string(nil), payload.Explanations...)
	if explanations == nil {
		explanations = []string{}
	}
	return domain.Optimization{
		Allocations:       allocations,
		FulfilledDemand:   payload.FulfilledDemand,
		UnfulfilledDemand: payload.UnfulfilledDemand,
		Costs:             payload.Costs.domainCost(),
		CarbonKg:          payload.CarbonKg,
		Explanations:      explanations,
	}, nil
}

func optimizerRequest(input app.OptimizationInput) (optimizerRequestDTO, error) {
	request := optimizerRequestDTO{
		SKU:                input.SKU,
		Suppliers:          []supplierDTO{},
		Warehouses:         []warehouseDTO{},
		Lanes:              make([]laneDTO, 0, len(input.Network.Lanes)),
		Demand:             []warehouseDemandDTO{},
		LostSalePenalty:    input.LostSalePenalty,
		CarbonPricePerKg:   input.CarbonPricePerKg,
		AverageStorageDays: 1,
		Currency:           "USD",
	}
	warehouseNodes := make([]domain.SupplyNode, 0)
	hasDemandShares := false
	for _, node := range input.Network.Nodes {
		role, ok := node.Role()
		if !ok {
			return optimizerRequestDTO{}, fmt.Errorf("node %q has an unsupported type", node.ID)
		}
		switch role {
		case domain.NodeRoleSupplier:
			capacity := node.Capacity
			if node.Available < capacity {
				capacity = node.Available
			}
			request.Suppliers = append(request.Suppliers, supplierDTO{ID: node.ID, CapacityUnits: capacity})
		case domain.NodeRoleWarehouse:
			request.Warehouses = append(request.Warehouses, warehouseDTO{
				ID:                    node.ID,
				CapacityUnits:         node.Capacity,
				StorageCostPerUnitDay: node.StorageCostPerUnitDay,
			})
			warehouseNodes = append(warehouseNodes, node)
			if node.DemandShare > 0 {
				hasDemandShares = true
			}
		}
	}
	for _, lane := range input.Network.Lanes {
		request.Lanes = append(request.Lanes, laneDTO{
			ID:                     laneID(lane.From, lane.To),
			SupplierID:             lane.From,
			WarehouseID:            lane.To,
			DistanceKM:             1,
			TransportCostPerUnitKM: lane.UnitCost,
			CarbonKgPerUnitKM:      lane.CarbonKgPerUnit,
			TruckCapacityUnits:     lane.Capacity,
			MaxTrucks:              1,
		})
	}
	remainingDemand := input.Demand
	remainingWarehouses := len(warehouseNodes)
	for _, warehouse := range warehouseNodes {
		share := 1 / float64(len(warehouseNodes))
		if hasDemandShares {
			share = warehouse.DemandShare
		}
		units := input.Demand * share
		remainingWarehouses--
		if remainingWarehouses == 0 {
			units = remainingDemand
		}
		remainingDemand -= units
		if units > 0 {
			request.Demand = append(request.Demand, warehouseDemandDTO{WarehouseID: warehouse.ID, Units: units})
		}
	}
	if len(request.Suppliers) == 0 || len(request.Warehouses) == 0 {
		return optimizerRequestDTO{}, fmt.Errorf("optimizer requires at least one supplier and one warehouse")
	}
	if input.Demand > 0 && len(request.Demand) == 0 {
		return optimizerRequestDTO{}, fmt.Errorf("optimizer demand distribution is empty")
	}
	return request, nil
}

func laneID(from, to string) string {
	return strings.TrimSpace(from) + "-" + strings.TrimSpace(to)
}

func firstString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (c *OptimizerClient) Ready(ctx context.Context) error {
	return c.client.do(ctx, http.MethodGet, "/readyz", nil, nil)
}
