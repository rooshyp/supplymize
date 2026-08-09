package outbound

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/supplymize/supplymize/services/control-plane/internal/app"
	"github.com/supplymize/supplymize/services/control-plane/internal/domain"
	"github.com/supplymize/supplymize/services/control-plane/internal/requestid"
)

func TestForecastClientTranslatesContractAndPropagatesRequestID(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/forecasts", func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s", request.Method)
		}
		if got := request.Header.Get("X-Request-ID"); got != "req_123" {
			t.Errorf("X-Request-ID = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["sku"] != "SKU-1" || body["horizon"] != float64(2) {
			t.Errorf("request body = %#v", body)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{
			"sku":"SKU-1",
			"model":"ensemble-v2",
			"forecast":{
				"confidence":0.92,
				"points":[
					{"period":"p3","predicted_units":5,"lower":4,"upper":6},
					{"period":"p4","demand":7,"lower_bound":6,"upper_bound":8}
				]
			}
		}`))
	})
	mux.HandleFunc("/api/readyz", func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusNoContent)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := NewForecastClient(server.URL+"/api", server.Client(), nil)
	if err != nil {
		t.Fatalf("NewForecastClient() error = %v", err)
	}
	ctx := requestid.WithContext(context.Background(), "req_123")
	forecast, err := client.Forecast(ctx, app.ForecastInput{
		SKU:     "SKU-1",
		Horizon: 2,
		History: []domain.DemandObservation{{Period: "p1", Units: 3}, {Period: "p2", Units: 4}},
		Signals: domain.PlanningSignals{Trend: 0.1},
	})
	if err != nil {
		t.Fatalf("Forecast() error = %v", err)
	}
	if forecast.Model != "ensemble-v2" || forecast.TotalDemand != 12 || forecast.Confidence != 0.92 {
		t.Fatalf("Forecast() = %#v", forecast)
	}
	if len(forecast.Points) != 2 || forecast.Points[0].Demand != 5 || forecast.Points[1].UpperBound != 8 {
		t.Fatalf("Forecast().Points = %#v", forecast.Points)
	}
	if err := client.Ready(context.Background()); err != nil {
		t.Fatalf("Ready() error = %v", err)
	}
}

func TestOptimizerClientTranslatesContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/optimize" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if got := request.Header.Get("X-Request-ID"); got != "req_optimizer_123" {
			t.Errorf("X-Request-ID = %q", got)
		}
		var body optimizerRequestDTO
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.SKU != "SKU-1" || len(body.Suppliers) != 1 || len(body.Warehouses) != 1 || len(body.Lanes) != 1 || len(body.Demand) != 1 {
			t.Errorf("request body = %#v", body)
		}
		if body.Suppliers[0].CapacityUnits != 50 || body.Demand[0].Units != 10 || body.Lanes[0].TransportCostPerUnitKM != 2 {
			t.Errorf("translated optimizer request = %#v", body)
		}
		_, _ = response.Write([]byte(`{
			"allocations":[{"lane_id":"factory-store","supplier_id":"factory","warehouse_id":"store","units":9,"costs":{"transport":18,"storage":1.8,"carbon":0.15,"total":19.95},"carbon_kg":1.5}],
			"fulfilled_demand":9,
			"unfulfilled_demand":1,
			"costs":{"transport":18,"storage":1.8,"lost_sales":10,"carbon":0.15,"total":29.95},
			"carbon_kg":1.5,
			"explanations":["lane capacity preserved one unit of safety stock"]
		}`))
	}))
	defer server.Close()
	client, err := NewOptimizerClient(server.URL, server.Client(), nil)
	if err != nil {
		t.Fatalf("NewOptimizerClient() error = %v", err)
	}
	network := domain.SupplyNetwork{
		Nodes: []domain.SupplyNode{
			{ID: "factory", Type: "supplier", Capacity: 100, Available: 50},
			{ID: "store", Type: "warehouse", Capacity: 100, DemandShare: 1, StorageCostPerUnitDay: 0.2},
		},
		Lanes: []domain.SupplyLane{{From: "factory", To: "store", Capacity: 20, UnitCost: 2, CarbonKgPerUnit: 0.1}},
	}
	ctx := requestid.WithContext(context.Background(), "req_optimizer_123")
	optimization, err := client.Optimize(ctx, app.OptimizationInput{
		SKU: "SKU-1", Demand: 10, Network: network, LostSalePenalty: 10, CarbonPricePerKg: 0.1,
	})
	if err != nil {
		t.Fatalf("Optimize() error = %v", err)
	}
	if optimization.FulfilledDemand != 9 || optimization.UnfulfilledDemand != 1 || optimization.Costs.Total != 29.95 || optimization.Costs.Holding != 1.8 {
		t.Fatalf("Optimize() = %#v", optimization)
	}
	if len(optimization.Allocations) != 1 || optimization.Allocations[0].From != "factory" || optimization.Allocations[0].To != "store" {
		t.Fatalf("Optimize().Allocations = %#v", optimization.Allocations)
	}
}

func TestClientClassifiesRetryableRemoteError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusServiceUnavailable)
		_, _ = response.Write([]byte(`{"error":{"message":"warming up"}}`))
	}))
	defer server.Close()
	client, err := NewForecastClient(server.URL, server.Client(), nil)
	if err != nil {
		t.Fatal(err)
	}
	err = client.Ready(context.Background())
	var remote *remoteError
	if !errors.As(err, &remote) {
		t.Fatalf("Ready() error = %v, want remote error", err)
	}
	if !remote.RetryableError() || remote.StatusCode != http.StatusServiceUnavailable || remote.Message != "warming up" {
		t.Fatalf("remote error = %#v", remote)
	}
}

func TestClientsRejectInvalidBaseURL(t *testing.T) {
	if _, err := NewForecastClient("forecast:8000", nil, nil); err == nil {
		t.Fatal("NewForecastClient() error = nil")
	}
	if _, err := NewOptimizerClient("file:///tmp/optimizer", nil, nil); err == nil {
		t.Fatal("NewOptimizerClient() error = nil")
	}
}
