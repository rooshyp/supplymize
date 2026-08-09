# SupplyMize Control Plane

The control plane is the synchronous orchestration boundary for SupplyMize planning. It accepts one validated supply-network request, obtains an AI demand forecast, sends the normalized demand into the optimizer, and returns a single traceable supply plan.

It is implemented with the Go standard library only. That keeps the image small, the dependency surface narrow, and the service easy to operate at the center of a polyglot platform.

## Request flow

```text
POST /v1/plans
      │
      ├─ validate history, signals, network, costs, and topology
      ├─ POST forecast /v1/forecasts
      ├─ normalize forecast totals, confidence, and points
      ├─ POST optimizer /v1/optimize
      └─ return forecast + allocations + cost/carbon trade-offs
```

Every request receives an `X-Request-ID`. A valid caller-provided ID is preserved and propagated to both downstream services; otherwise, the control plane creates a cryptographically random ID. The same ID appears in successful plans, error envelopes, and structured logs.

## API

| Method | Path | Purpose |
| --- | --- | --- |
| `POST` | `/v1/plans` | Build a forecast-backed supply plan |
| `GET` | `/healthz` | Process liveness; does not call dependencies |
| `GET` | `/readyz` | Concurrent forecast and optimizer readiness checks |
| `GET` | `/metrics` | Prometheus-compatible service metrics |

### Create a plan

```bash
curl --fail-with-body http://localhost:8080/v1/plans \
  -H 'Content-Type: application/json' \
  -H 'X-Request-ID: demo-west-001' \
  -d '{
    "sku": "FILTER-HEPA-13",
    "horizon": 2,
    "history": [
      {"period": "2026-W30", "units": 820},
      {"period": "2026-W31", "units": 870},
      {"period": "2026-W32", "units": 910},
      {"period": "2026-W33", "units": 905},
      {"period": "2026-W34", "units": 940},
      {"period": "2026-W35", "units": 960},
      {"period": "2026-W36", "units": 975}
    ],
    "signals": {
      "trend": 0.08,
      "seasonality": [0.94, 1.06],
      "weather": 0.03,
      "event": 0.15,
      "supplier_speed": 0.92
    },
    "network": {
      "nodes": [
        {"id": "plant-sjc", "type": "factory", "capacity": 5000, "available": 2400},
        {"id": "dc-oak", "type": "distribution_center", "capacity": 3000, "available": 350}
      ],
      "lanes": [
        {
          "from": "plant-sjc",
          "to": "dc-oak",
          "capacity": 1800,
          "unit_cost": 2.45,
          "lead_time_periods": 1,
          "carbon_kg_per_unit": 0.31
        }
      ]
    },
    "lost_sale_penalty": 28,
    "carbon_price_per_kg": 0.19
  }'
```

A successful response has the following stable shape:

```json
{
  "plan_id": "plan_f4469d97bd414f22bfeb1b5eef348f6d",
  "request_id": "demo-west-001",
  "sku": "FILTER-HEPA-13",
  "generated_at": "2026-08-08T19:30:00Z",
  "forecast": {
    "model": "ensemble-v2",
    "total_demand": 1940,
    "confidence": 0.92,
    "points": [
      {"period": "2026-W33", "demand": 950, "lower_bound": 880, "upper_bound": 1020},
      {"period": "2026-W34", "demand": 990, "lower_bound": 910, "upper_bound": 1080}
    ]
  },
  "optimization": {
    "allocations": [
      {
        "from": "plant-sjc",
        "to": "dc-oak",
        "quantity": 1800,
        "unit_cost": 2.45,
        "carbon_kg": 558,
        "reason": "lowest feasible landed cost"
      }
    ],
    "fulfilled_demand": 1800,
    "unfulfilled_demand": 140,
    "costs": {
      "procurement": 0,
      "transport": 4410,
      "holding": 0,
      "lost_sales": 3920,
      "carbon": 106.02,
      "total": 8436.02
    },
    "carbon_kg": 558,
    "explanations": ["Lane capacity limits fulfillment to 92.8% of forecast demand."]
  }
}
```

Malformed JSON returns `400`, invalid domain fields return `422` with every detectable field violation, dependency failures return `502` or `503`, and an orchestration deadline returns `504`. Unknown JSON fields are rejected to catch client contract drift early.

## Configuration

All durations use Go duration syntax such as `250ms`, `10s`, or `1m`.

| Variable | Default | Meaning |
| --- | --- | --- |
| `SUPPLYMIZE_ENV` | `development` | Environment label included in logs |
| `SUPPLYMIZE_HTTP_ADDR` | `:8080` | HTTP listen address |
| `SUPPLYMIZE_FORECAST_URL` | `http://forecasting:8001` | Forecast service base URL |
| `SUPPLYMIZE_OPTIMIZER_URL` | `http://optimizer:8002` | Optimizer service base URL |
| `SUPPLYMIZE_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` |
| `SUPPLYMIZE_READ_TIMEOUT` | `5s` | HTTP request/read-header timeout |
| `SUPPLYMIZE_WRITE_TIMEOUT` | `30s` | HTTP response timeout |
| `SUPPLYMIZE_IDLE_TIMEOUT` | `60s` | Keep-alive idle timeout |
| `SUPPLYMIZE_PLAN_TIMEOUT` | `20s` | Entire planning workflow deadline |
| `SUPPLYMIZE_DEPENDENCY_TIMEOUT` | `10s` | Individual downstream HTTP deadline |
| `SUPPLYMIZE_READINESS_TIMEOUT` | `2s` | Readiness probe deadline |
| `SUPPLYMIZE_SHUTDOWN_TIMEOUT` | `15s` | Graceful drain window |
| `SUPPLYMIZE_MAX_REQUEST_BYTES` | `1048576` | Maximum plan body size |

Configuration is validated before the listener opens. In particular, URLs must be absolute HTTP(S) URLs, durations must be positive, the write timeout must cover the plan timeout, and request sizes must remain within safe bounds.

## Local development

Go 1.22 or newer is required.

```bash
export SUPPLYMIZE_FORECAST_URL=http://localhost:8001
export SUPPLYMIZE_OPTIMIZER_URL=http://localhost:8002
go run ./cmd/control-plane
```

Run all verification steps:

```bash
gofmt -w $(find . -name '*.go' -type f)
go test -race ./...
go vet ./...
```

Or use `make test`, `make test-race`, `make cover`, and `make vet`.

## Container

The production image is a static, non-root scratch image.

```bash
docker build --build-arg VERSION=0.1.0 -t supplymize/control-plane:0.1.0 .
docker run --rm -p 8080:8080 \
  -e SUPPLYMIZE_FORECAST_URL=http://host.docker.internal:8001 \
  -e SUPPLYMIZE_OPTIMIZER_URL=http://host.docker.internal:8002 \
  supplymize/control-plane:0.1.0
```

## Package boundaries

```text
cmd/control-plane       composition, lifecycle, and graceful shutdown
internal/domain         stable planning language and validation rules
internal/app            forecast → optimize orchestration and readiness
internal/outbound       isolated HTTP adapters for polyglot dependencies
internal/httpapi        transport, middleware, status mapping, and JSON
internal/config         environment parsing and startup validation
internal/requestid      correlation context and secure identifiers
```

The application layer depends only on interfaces and domain objects. Downstream JSON quirks are contained in the outbound adapters, so a Python forecast model or Rust optimizer can evolve without leaking infrastructure details into the public API.
