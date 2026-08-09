package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/supplymize/supplymize/services/control-plane/internal/app"
	"github.com/supplymize/supplymize/services/control-plane/internal/domain"
	"github.com/supplymize/supplymize/services/control-plane/internal/requestid"
)

type plannerFunc func(context.Context, domain.PlanRequest) (domain.SupplyPlan, error)

func (function plannerFunc) Plan(ctx context.Context, request domain.PlanRequest) (domain.SupplyPlan, error) {
	return function(ctx, request)
}

type readinessFunc func(context.Context) map[string]error

func (function readinessFunc) Check(ctx context.Context) map[string]error { return function(ctx) }

func TestPlanEndpointReturnsOrchestratedPlanAndRequestID(t *testing.T) {
	planner := plannerFunc(func(ctx context.Context, request domain.PlanRequest) (domain.SupplyPlan, error) {
		if request.SKU != "SKU-001" {
			t.Fatalf("request = %#v", request)
		}
		return domain.SupplyPlan{
			PlanID:      "plan_123",
			RequestID:   requestid.FromContext(ctx),
			SKU:         request.SKU,
			GeneratedAt: time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC),
			Forecast: domain.Forecast{
				Model: "ensemble", TotalDemand: 10, Confidence: 0.9,
				Points: []domain.ForecastPoint{{Period: "p3", Demand: 10, LowerBound: 8, UpperBound: 12}},
			},
			Optimization: domain.Optimization{FulfilledDemand: 10, Explanations: []string{}},
		}, nil
	})
	handler := newTestHandler(planner, readinessFunc(func(context.Context) map[string]error { return map[string]error{} }), Options{})
	body, _ := json.Marshal(apiValidRequest())
	request := httptest.NewRequest(http.MethodPost, "/v1/plans", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	request.Header.Set("X-Request-ID", "client-trace_123")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("X-Request-ID"); got != "client-trace_123" {
		t.Fatalf("X-Request-ID = %q", got)
	}
	if got := response.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
	var plan domain.SupplyPlan
	if err := json.NewDecoder(response.Body).Decode(&plan); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if plan.PlanID != "plan_123" || plan.RequestID != "client-trace_123" {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestPlanEndpointRejectsBadRequests(t *testing.T) {
	planner := plannerFunc(func(_ context.Context, request domain.PlanRequest) (domain.SupplyPlan, error) {
		if err := request.Validate(); err != nil {
			return domain.SupplyPlan{}, err
		}
		return domain.SupplyPlan{}, nil
	})
	tests := []struct {
		name        string
		contentType string
		body        string
		options     Options
		wantStatus  int
		wantCode    string
	}{
		{name: "missing content type", body: `{}`, wantStatus: http.StatusUnsupportedMediaType, wantCode: "unsupported_media_type"},
		{name: "malformed JSON", contentType: "application/json", body: `{`, wantStatus: http.StatusBadRequest, wantCode: "invalid_json"},
		{name: "unknown field", contentType: "application/json", body: `{"unknown":true}`, wantStatus: http.StatusBadRequest, wantCode: "invalid_json"},
		{name: "multiple values", contentType: "application/json", body: `{ } { }`, wantStatus: http.StatusBadRequest, wantCode: "invalid_json"},
		{name: "body too large", contentType: "application/json", body: strings.Repeat(" ", 20) + `{}`, options: Options{MaxRequestBytes: 10}, wantStatus: http.StatusRequestEntityTooLarge, wantCode: "request_too_large"},
		{name: "invalid domain", contentType: "application/json", body: `{}`, wantStatus: http.StatusUnprocessableEntity, wantCode: "validation_failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := newTestHandler(planner, nil, test.options)
			request := httptest.NewRequest(http.MethodPost, "/v1/plans", strings.NewReader(test.body))
			if test.contentType != "" {
				request.Header.Set("Content-Type", test.contentType)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, test.wantStatus, response.Body.String())
			}
			var envelope errorEnvelope
			if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if envelope.Error.Code != test.wantCode || envelope.RequestID == "" {
				t.Fatalf("error = %#v", envelope)
			}
			if test.wantCode == "validation_failed" && len(envelope.Error.Details) == 0 {
				t.Fatal("validation response has no details")
			}
		})
	}
}

func TestPlanEndpointMapsDependencyErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "retryable", err: &app.DependencyError{Stage: "forecast", Retryable: true, Err: errors.New("down")}, wantStatus: http.StatusServiceUnavailable, wantCode: "dependency_unavailable"},
		{name: "invalid response", err: &app.DependencyError{Stage: "optimization", Err: errors.New("bad response")}, wantStatus: http.StatusBadGateway, wantCode: "dependency_failure"},
		{name: "timeout", err: context.DeadlineExceeded, wantStatus: http.StatusGatewayTimeout, wantCode: "planning_timeout"},
	}
	body, _ := json.Marshal(apiValidRequest())
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := newTestHandler(plannerFunc(func(context.Context, domain.PlanRequest) (domain.SupplyPlan, error) {
				return domain.SupplyPlan{}, test.err
			}), nil, Options{})
			request := httptest.NewRequest(http.MethodPost, "/v1/plans", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			var envelope errorEnvelope
			_ = json.NewDecoder(response.Body).Decode(&envelope)
			if envelope.Error.Code != test.wantCode {
				t.Fatalf("code = %q, want %q", envelope.Error.Code, test.wantCode)
			}
		})
	}
}

func TestHealthAndReadinessEndpoints(t *testing.T) {
	readiness := readinessFunc(func(context.Context) map[string]error {
		return map[string]error{"forecast": nil, "optimizer": errors.New("warming up")}
	})
	handler := newTestHandler(nil, readiness, Options{Version: "v1.2.3"})

	healthRequest := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthResponse := httptest.NewRecorder()
	handler.ServeHTTP(healthResponse, healthRequest)
	if healthResponse.Code != http.StatusOK || !strings.Contains(healthResponse.Body.String(), `"version":"v1.2.3"`) {
		t.Fatalf("health response = %d %s", healthResponse.Code, healthResponse.Body.String())
	}

	readyRequest := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	readyResponse := httptest.NewRecorder()
	handler.ServeHTTP(readyResponse, readyRequest)
	if readyResponse.Code != http.StatusServiceUnavailable || !strings.Contains(readyResponse.Body.String(), `"status":"not_ready"`) {
		t.Fatalf("ready response = %d %s", readyResponse.Code, readyResponse.Body.String())
	}
}

func TestMetricsEndpointExposesPrometheusCounters(t *testing.T) {
	handler := newTestHandler(nil, nil, Options{})

	healthRequest := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	handler.ServeHTTP(httptest.NewRecorder(), healthRequest)

	metricsRequest := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsResponse := httptest.NewRecorder()
	handler.ServeHTTP(metricsResponse, metricsRequest)
	if metricsResponse.Code != http.StatusOK {
		t.Fatalf("metrics status = %d", metricsResponse.Code)
	}
	if contentType := metricsResponse.Header().Get("Content-Type"); !strings.Contains(contentType, "text/plain") {
		t.Fatalf("metrics Content-Type = %q", contentType)
	}
	for _, metric := range []string{"supplymize_control_plane_http_requests_total 1", `supplymize_plan_requests_total{outcome="success"} 0`} {
		if !strings.Contains(metricsResponse.Body.String(), metric) {
			t.Errorf("metrics response does not contain %q: %s", metric, metricsResponse.Body.String())
		}
	}
}

func TestMiddlewareHandlesMethodsRoutesInvalidIDsAndPanics(t *testing.T) {
	panickingPlanner := plannerFunc(func(context.Context, domain.PlanRequest) (domain.SupplyPlan, error) {
		panic("test panic")
	})
	handler := newTestHandler(panickingPlanner, nil, Options{})

	methodRequest := httptest.NewRequest(http.MethodGet, "/v1/plans", nil)
	methodResponse := httptest.NewRecorder()
	handler.ServeHTTP(methodResponse, methodRequest)
	if methodResponse.Code != http.StatusMethodNotAllowed || methodResponse.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("method response = %d, Allow = %q", methodResponse.Code, methodResponse.Header().Get("Allow"))
	}

	notFoundRequest := httptest.NewRequest(http.MethodGet, "/not-a-route", nil)
	notFoundResponse := httptest.NewRecorder()
	handler.ServeHTTP(notFoundResponse, notFoundRequest)
	if notFoundResponse.Code != http.StatusNotFound {
		t.Fatalf("not found status = %d", notFoundResponse.Code)
	}

	body, _ := json.Marshal(apiValidRequest())
	panicRequest := httptest.NewRequest(http.MethodPost, "/v1/plans", bytes.NewReader(body))
	panicRequest.Header.Set("Content-Type", "application/json")
	panicRequest.Header.Set("X-Request-ID", "not valid due to spaces")
	panicResponse := httptest.NewRecorder()
	handler.ServeHTTP(panicResponse, panicRequest)
	if panicResponse.Code != http.StatusInternalServerError {
		t.Fatalf("panic status = %d, body = %s", panicResponse.Code, panicResponse.Body.String())
	}
	if got := panicResponse.Header().Get("X-Request-ID"); got == "" || got == "not valid due to spaces" {
		t.Fatalf("generated X-Request-ID = %q", got)
	}
}

func newTestHandler(planner Planner, readiness Readiness, options Options) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewHandler(planner, readiness, logger, options)
}

func apiValidRequest() domain.PlanRequest {
	return domain.PlanRequest{
		SKU:     "SKU-001",
		Horizon: 1,
		History: []domain.DemandObservation{{Period: "p1", Units: 8}, {Period: "p2", Units: 9}},
		Network: domain.SupplyNetwork{
			Nodes: []domain.SupplyNode{
				{ID: "factory", Type: "factory", Capacity: 100, Available: 50},
				{ID: "store", Type: "customer", Capacity: 100},
			},
			Lanes: []domain.SupplyLane{{From: "factory", To: "store", Capacity: 100}},
		},
		LostSalePenalty: 10,
	}
}
