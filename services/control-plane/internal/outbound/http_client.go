package outbound

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/supplymize/supplymize/services/control-plane/internal/requestid"
)

const maxResponseBytes = 4 << 20

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type remoteError struct {
	Service    string
	StatusCode int
	Message    string
	Retryable  bool
	Err        error
}

func (e *remoteError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("%s returned HTTP %d: %s", e.Service, e.StatusCode, e.Message)
	}
	return fmt.Sprintf("%s request failed: %v", e.Service, e.Err)
}

func (e *remoteError) Unwrap() error { return e.Err }

func (e *remoteError) RetryableError() bool { return e.Retryable }

type jsonClient struct {
	service   string
	baseURL   *url.URL
	http      HTTPDoer
	logger    *slog.Logger
	userAgent string
}

func newJSONClient(service, rawBaseURL string, httpClient HTTPDoer, logger *slog.Logger) (*jsonClient, error) {
	baseURL, err := url.Parse(strings.TrimSpace(rawBaseURL))
	if err != nil {
		return nil, fmt.Errorf("parse %s URL: %w", service, err)
	}
	if (baseURL.Scheme != "http" && baseURL.Scheme != "https") || baseURL.Host == "" {
		return nil, fmt.Errorf("%s URL must be an absolute HTTP(S) URL", service)
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &jsonClient{
		service:   service,
		baseURL:   baseURL,
		http:      httpClient,
		logger:    logger.With("component", service+"_client"),
		userAgent: "supplymize-control-plane/1",
	}, nil
}

func (c *jsonClient) do(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		payload, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("encode %s request: %w", c.service, err)
		}
		body = bytes.NewReader(payload)
	}

	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/" + strings.TrimLeft(path, "/")
	endpoint.RawQuery = ""
	endpoint.Fragment = ""
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return fmt.Errorf("build %s request: %w", c.service, err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", c.userAgent)
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if id := requestid.FromContext(ctx); id != "" {
		request.Header.Set("X-Request-ID", id)
	}

	started := time.Now()
	response, err := c.http.Do(request)
	if err != nil {
		return &remoteError{Service: c.service, Message: "request failed", Retryable: !errors.Is(err, context.Canceled), Err: err}
	}
	defer response.Body.Close()
	c.logger.DebugContext(ctx, "dependency request completed",
		"method", method,
		"path", endpoint.Path,
		"status", response.StatusCode,
		"duration_ms", time.Since(started).Milliseconds(),
		"request_id", requestid.FromContext(ctx),
	)

	limited := io.LimitReader(response.Body, maxResponseBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return &remoteError{Service: c.service, Message: "read response", Retryable: true, Err: err}
	}
	if len(data) > maxResponseBytes {
		return &remoteError{Service: c.service, Message: "response exceeded size limit", Retryable: false}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return &remoteError{
			Service:    c.service,
			StatusCode: response.StatusCode,
			Message:    errorMessage(data),
			Retryable:  retryableStatus(response.StatusCode),
		}
	}
	if output == nil {
		return nil
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return &remoteError{Service: c.service, StatusCode: response.StatusCode, Message: "empty response", Retryable: false}
	}
	if err := json.Unmarshal(data, output); err != nil {
		return &remoteError{Service: c.service, StatusCode: response.StatusCode, Message: "invalid JSON response", Retryable: false, Err: err}
	}
	return nil
}

func errorMessage(data []byte) string {
	var envelope struct {
		Message string `json:"message"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &envelope); err == nil {
		if message := strings.TrimSpace(envelope.Error.Message); message != "" {
			return message
		}
		if message := strings.TrimSpace(envelope.Message); message != "" {
			return message
		}
	}
	return http.StatusText(http.StatusBadGateway)
}

func retryableStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooEarly || status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}
