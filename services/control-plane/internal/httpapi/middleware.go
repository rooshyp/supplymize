package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"

	"github.com/supplymize/supplymize/services/control-plane/internal/requestid"
)

type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *responseRecorder) WriteHeader(status int) {
	if r.status != 0 {
		return
	}
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.WriteHeader(http.StatusOK)
	}
	written, err := r.ResponseWriter.Write(data)
	r.bytes += written
	return written, err
}

func accessLog(next http.Handler, logger *slog.Logger, metrics *metrics) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		started := time.Now()
		recorder := &responseRecorder{ResponseWriter: response}
		next.ServeHTTP(recorder, request)
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		duration := time.Since(started)
		if metrics != nil {
			metrics.observe(request.URL.Path, status, duration)
		}
		logger.InfoContext(request.Context(), "HTTP request completed",
			"method", request.Method,
			"path", request.URL.Path,
			"status", status,
			"response_bytes", recorder.bytes,
			"duration_ms", duration.Milliseconds(),
			"request_id", requestid.FromContext(request.Context()),
		)
	})
}

func recoverPanic(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.ErrorContext(request.Context(), "panic recovered",
					"panic", recovered,
					"stack", string(debug.Stack()),
					"request_id", requestid.FromContext(request.Context()),
				)
				writeError(response, request, http.StatusInternalServerError, "internal_error", "an unexpected error occurred", nil)
			}
		}()
		next.ServeHTTP(response, request)
	})
}

var fallbackCounter uint64

func withRequestID(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		id := strings.TrimSpace(request.Header.Get("X-Request-ID"))
		if !validRequestID(id) {
			var err error
			id, err = requestid.New("req")
			if err != nil {
				id = fmt.Sprintf("req_fallback_%x_%x", time.Now().UnixNano(), atomic.AddUint64(&fallbackCounter, 1))
				logger.ErrorContext(request.Context(), "could not generate cryptographic request ID", "error", err)
			}
		}
		response.Header().Set("X-Request-ID", id)
		next.ServeHTTP(response, request.WithContext(requestid.WithContext(request.Context(), id)))
	})
}

func validRequestID(id string) bool {
	if len(id) < 1 || len(id) > 128 {
		return false
	}
	for _, character := range id {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			strings.ContainsRune("._:-", character) {
			continue
		}
		return false
	}
	return true
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Cache-Control", "no-store")
		response.Header().Set("X-Content-Type-Options", "nosniff")
		response.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(response, request)
	})
}
