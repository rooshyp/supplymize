package httpapi

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

// metrics is a deliberately small, dependency-free Prometheus collector. The
// control plane stays stdlib-only while still exposing the service-level
// signals needed by the reference observability stack.
type metrics struct {
	requestsTotal   atomic.Uint64
	requestDuration atomic.Uint64
	planSuccess     atomic.Uint64
	planFailure     atomic.Uint64
}

func newMetrics() *metrics {
	return &metrics{}
}

func (m *metrics) observe(path string, status int, duration time.Duration) {
	m.requestsTotal.Add(1)
	m.requestDuration.Add(uint64(duration.Nanoseconds()))
	if path != "/v1/plans" {
		return
	}
	if status >= http.StatusOK && status < http.StatusBadRequest {
		m.planSuccess.Add(1)
		return
	}
	m.planFailure.Add(1)
}

func (m *metrics) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if !allowMethod(response, request, http.MethodGet) {
		return
	}
	response.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	response.WriteHeader(http.StatusOK)
	durationSeconds := float64(m.requestDuration.Load()) / float64(time.Second)
	_, _ = fmt.Fprintf(response, `# HELP supplymize_control_plane_http_requests_total HTTP requests handled by the control plane.
# TYPE supplymize_control_plane_http_requests_total counter
supplymize_control_plane_http_requests_total %d
# HELP supplymize_control_plane_http_request_duration_seconds_sum Cumulative HTTP request duration in seconds.
# TYPE supplymize_control_plane_http_request_duration_seconds_sum counter
supplymize_control_plane_http_request_duration_seconds_sum %.9f
# HELP supplymize_plan_requests_total Planning requests by outcome.
# TYPE supplymize_plan_requests_total counter
supplymize_plan_requests_total{outcome="success"} %d
supplymize_plan_requests_total{outcome="failure"} %d
`, m.requestsTotal.Load(), durationSeconds, m.planSuccess.Load(), m.planFailure.Load())
}
