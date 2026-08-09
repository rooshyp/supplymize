package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/supplymize/supplymize/services/control-plane/internal/app"
	"github.com/supplymize/supplymize/services/control-plane/internal/domain"
	"github.com/supplymize/supplymize/services/control-plane/internal/requestid"
)

type Planner interface {
	Plan(context.Context, domain.PlanRequest) (domain.SupplyPlan, error)
}

type Readiness interface {
	Check(context.Context) map[string]error
}

type Options struct {
	PlanTimeout      time.Duration
	ReadinessTimeout time.Duration
	MaxRequestBytes  int64
	Version          string
}

func NewHandler(planner Planner, readiness Readiness, logger *slog.Logger, options Options) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	api := &api{
		planner:          planner,
		readiness:        readiness,
		logger:           logger,
		planTimeout:      options.PlanTimeout,
		readinessTimeout: options.ReadinessTimeout,
		maxRequestBytes:  options.MaxRequestBytes,
		version:          options.Version,
	}
	if api.planTimeout <= 0 {
		api.planTimeout = 20 * time.Second
	}
	if api.readinessTimeout <= 0 {
		api.readinessTimeout = 2 * time.Second
	}
	if api.maxRequestBytes <= 0 {
		api.maxRequestBytes = 1 << 20
	}
	if strings.TrimSpace(api.version) == "" {
		api.version = "development"
	}

	mux := http.NewServeMux()
	metricStore := newMetrics()
	mux.HandleFunc("/healthz", api.health)
	mux.HandleFunc("/readyz", api.ready)
	mux.Handle("/metrics", metricStore)
	mux.HandleFunc("/v1/plans", api.plans)
	mux.HandleFunc("/", api.notFound)

	handler := securityHeaders(mux)
	handler = recoverPanic(handler, logger)
	handler = accessLog(handler, logger, metricStore)
	handler = withRequestID(handler, logger)
	return handler
}

type api struct {
	planner          Planner
	readiness        Readiness
	logger           *slog.Logger
	planTimeout      time.Duration
	readinessTimeout time.Duration
	maxRequestBytes  int64
	version          string
}

type errorEnvelope struct {
	Error     apiError `json:"error"`
	RequestID string   `json:"request_id"`
}

type apiError struct {
	Code    string                  `json:"code"`
	Message string                  `json:"message"`
	Details []domain.FieldViolation `json:"details,omitempty"`
}

func (a *api) health(response http.ResponseWriter, request *http.Request) {
	if !allowMethod(response, request, http.MethodGet) {
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"status":  "ok",
		"service": "control-plane",
		"version": a.version,
	})
}

type readinessCheck struct {
	Status string `json:"status"`
}

func (a *api) ready(response http.ResponseWriter, request *http.Request) {
	if !allowMethod(response, request, http.MethodGet) {
		return
	}
	if a.readiness == nil {
		writeError(response, request, http.StatusServiceUnavailable, "not_ready", "readiness checks are not configured", nil)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), a.readinessTimeout)
	defer cancel()
	results := a.readiness.Check(ctx)
	checks := make(map[string]readinessCheck, len(results))
	ready := true
	for name, err := range results {
		status := "up"
		if err != nil {
			status = "down"
			ready = false
			a.logger.WarnContext(request.Context(), "readiness dependency failed",
				"dependency", name,
				"error", err,
				"request_id", requestid.FromContext(request.Context()),
			)
		}
		checks[name] = readinessCheck{Status: status}
	}
	statusCode, status := http.StatusOK, "ready"
	if !ready {
		statusCode, status = http.StatusServiceUnavailable, "not_ready"
	}
	writeJSON(response, statusCode, map[string]any{
		"status":  status,
		"service": "control-plane",
		"checks":  checks,
	})
}

func (a *api) plans(response http.ResponseWriter, request *http.Request) {
	if !allowMethod(response, request, http.MethodPost) {
		return
	}
	if a.planner == nil {
		writeError(response, request, http.StatusServiceUnavailable, "not_ready", "planner is not configured", nil)
		return
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(response, request, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be application/json", nil)
		return
	}

	request.Body = http.MaxBytesReader(response, request.Body, a.maxRequestBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var input domain.PlanRequest
	if err := decoder.Decode(&input); err != nil {
		a.writeDecodeError(response, request, err)
		return
	}
	if err := ensureSingleValue(decoder); err != nil {
		a.writeDecodeError(response, request, err)
		return
	}

	ctx, cancel := context.WithTimeout(request.Context(), a.planTimeout)
	defer cancel()
	plan, err := a.planner.Plan(ctx, input)
	if err != nil {
		a.writePlanError(response, request, err)
		return
	}
	writeJSON(response, http.StatusOK, plan)
}

func (a *api) writeDecodeError(response http.ResponseWriter, request *http.Request, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(response, request, http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds the configured limit", nil)
		return
	}
	writeError(response, request, http.StatusBadRequest, "invalid_json", "body must contain one valid JSON object matching the API contract", nil)
}

func (a *api) writePlanError(response http.ResponseWriter, request *http.Request, err error) {
	var validation *domain.ValidationError
	if errors.As(err, &validation) {
		writeError(response, request, http.StatusUnprocessableEntity, "validation_failed", "one or more fields are invalid", validation.Violations)
		return
	}
	if errors.Is(err, context.DeadlineExceeded) {
		writeError(response, request, http.StatusGatewayTimeout, "planning_timeout", "planning did not finish before the deadline", nil)
		return
	}
	if errors.Is(err, context.Canceled) {
		writeError(response, request, http.StatusRequestTimeout, "request_canceled", "request was canceled", nil)
		return
	}
	var dependency *app.DependencyError
	if errors.As(err, &dependency) {
		status := http.StatusBadGateway
		code := "dependency_failure"
		if dependency.Retryable {
			status = http.StatusServiceUnavailable
			code = "dependency_unavailable"
		}
		a.logger.WarnContext(request.Context(), "planning dependency failed",
			"stage", dependency.Stage,
			"retryable", dependency.Retryable,
			"error", dependency.Err,
			"request_id", requestid.FromContext(request.Context()),
		)
		writeError(response, request, status, code, "a planning dependency could not complete the request", nil)
		return
	}
	a.logger.ErrorContext(request.Context(), "planning request failed",
		"error", err,
		"request_id", requestid.FromContext(request.Context()),
	)
	writeError(response, request, http.StatusInternalServerError, "internal_error", "an unexpected error occurred", nil)
}

func (a *api) notFound(response http.ResponseWriter, request *http.Request) {
	writeError(response, request, http.StatusNotFound, "not_found", "route not found", nil)
}

func ensureSingleValue(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("body contains multiple JSON values")
		}
		return err
	}
	return nil
}

func allowMethod(response http.ResponseWriter, request *http.Request, method string) bool {
	if request.Method == method {
		return true
	}
	response.Header().Set("Allow", method)
	writeError(response, request, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	return false
}

func writeError(response http.ResponseWriter, request *http.Request, status int, code, message string, details []domain.FieldViolation) {
	writeJSON(response, status, errorEnvelope{
		Error: apiError{
			Code:    code,
			Message: message,
			Details: details,
		},
		RequestID: requestid.FromContext(request.Context()),
	})
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
