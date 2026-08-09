# API guide

The Go control plane is the public API. Forecasting and optimization endpoints are internal contracts that remain directly callable for model development and solver benchmarking.

## Create a plan

```bash
curl --request POST http://localhost:8080/v1/plans \
  --header 'Content-Type: application/json' \
  --header 'X-Request-ID: demo-2026-08-08' \
  --data @examples/plan-request.json
```

A successful response combines the forecast, optimized allocation, cost/carbon breakdown, assumptions, and an end-to-end request ID. See [`contracts/openapi/control-plane.yaml`](../contracts/openapi/control-plane.yaml) for the complete schema.

## Forecast demand directly

```bash
curl --request POST http://localhost:8001/v1/forecasts \
  --header 'Content-Type: application/json' \
  --data @examples/forecast-request.json
```

The forecast response includes daily points, total and average demand, lower/upper uncertainty bounds, component contributions, and human-readable explanations. Signals are multipliers or dated effects; absent signals remain neutral and appear in assumptions.

## Optimize allocation directly

```bash
curl --request POST http://localhost:8002/v1/optimize \
  --header 'Content-Type: application/json' \
  --data @examples/optimization-request.json
```

The optimizer returns a feasible allocation when possible. When capacity is insufficient it still returns the best allocation, explicit unfulfilled demand, the lost-sales penalty, and binding constraints.

## Error envelope

Public errors use a stable machine-readable shape:

```json
{
  "error": {
    "code": "validation_failed",
    "message": "request validation failed with 1 violation(s)",
    "details": [
      {"field": "history", "message": "must contain at least 7 observations"}
    ]
  },
  "request_id": "demo-2026-08-08"
}
```

Consumers should branch on `error.code`, not the human-readable message.

## Operations endpoints

| Endpoint | Meaning |
| --- | --- |
| `GET /healthz` | Process is alive |
| `GET /readyz` | Process can accept planning traffic |
| `GET /metrics` | Prometheus text exposition |
