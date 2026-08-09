<div align="center">

# SupplyMize Forecasting

**Explainable demand intelligence for resilient supply chains**

`FastAPI` · `Pydantic v2` · `Python 3.11+` · `Prometheus` · `Docker`

</div>

---

The forecasting service turns recent SKU demand and known operational signals into a
deterministic, auditable daily forecast. It is deliberately useful without an external model,
weather provider, feature store, or network connection, making it a dependable baseline for
local development and production fallback paths.

## Service contract

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/v1/forecasts` | Create a demand forecast with points, intervals, and explanations |
| `GET` | `/v1/model/info` | Inspect the active model and supported drivers |
| `GET` | `/healthz` | Kubernetes/container liveness probe |
| `GET` | `/readyz` | Readiness probe |
| `GET` | `/metrics` | Prometheus text exposition |
| `GET` | `/docs` | Interactive OpenAPI documentation |

### Request

`history` accepts either a compact numeric series (oldest first) or dated objects. If history is
not dated, supply `as_of_date` when calendar dates are needed in the result.

```json
{
  "sku": "SKU-1042",
  "location": "LAX-01",
  "history": [95, 101, 99, 104, 108, 107, 110, 112, 111, 115, 117, 116, 120, 122],
  "horizon": 7,
  "as_of_date": "2026-08-08",
  "confidence_level": 0.9,
  "signals": {
    "trend": 0.01,
    "seasonality": [0.9, 1.0, 1.1, 1.0, 1.05, 1.15, 0.8],
    "weather": [
      {"step": 2, "impact": 0.12, "confidence": 0.8, "label": "Heat wave"}
    ],
    "event": [
      {"step": 3, "impact": 0.2, "confidence": 0.9, "label": "Regional expo"}
    ],
    "supplier_speed": [
      {"lead_time_days": 8, "baseline_lead_time_days": 10, "confidence": 0.9}
    ]
  }
}
```

An impact of `0.12` means a 12% candidate uplift before confidence and ensemble weighting. A
signal with no `date` or `step` applies to the entire horizon. Supplier speed may be supplied as
`speed_index` (`-1` to `1`) or derived from current and baseline lead times.

### Response shape

```json
{
  "model_version": "1.0.0",
  "sku": "SKU-1042",
  "location": "LAX-01",
  "history_points_used": 14,
  "forecast": {
    "total_demand": 843.216,
    "average_daily_demand": 120.459,
    "total_lower_bound": 744.101,
    "total_upper_bound": 942.331,
    "points": [
      {
        "step": 1,
        "date": "2026-08-09",
        "demand": 116.732,
        "lower_bound": 103.21,
        "upper_bound": 130.254,
        "components": {
          "history": 57.9,
          "trend": 17.7,
          "seasonality": 15.8,
          "weather": 9.3,
          "event": 9.3,
          "supplier_speed": 6.732
        }
      }
    ]
  },
  "confidence": {
    "level": 0.9,
    "score": 0.68,
    "residual_rmse": 4.21,
    "method": "normal-residual-with-horizon-growth",
    "interpretation": "moderate"
  },
  "explanations": [],
  "assumptions": []
}
```

The sample is abbreviated; use `/docs` or `/openapi.json` as the authoritative schema.

## Forecasting method

The model creates six interpretable candidates around a recency-weighted history level and then
forms a weighted ensemble:

```text
forecast = 0.50 × history level
         + 0.15 × trend candidate
         + 0.15 × seasonal candidate
         + 0.08 × weather candidate
         + 0.08 × event candidate
         + 0.04 × supplier-speed candidate
```

- **History** uses linearly increasing recency weights over the configured window.
- **Trend** uses bounded least-squares daily change, or a supplied compound daily growth rate.
- **Seasonality** infers a shrunk weekly pattern after two cycles, or normalizes supplied factors.
- **Weather and events** apply date-, step-, or horizon-level fractional impacts, scaled by confidence.
- **Supplier speed** converts lead-time improvement into a bounded availability proxy.
- **Intervals** use fitted residual RMSE, a configurable uncertainty floor, signal uncertainty, and
  square-root horizon growth.

Every point exposes the weighted values of all candidates. These components add to `demand`, and
the horizon explanation reports each driver's adjustment relative to the history level. Identical
requests and configuration produce identical forecast payloads.

## Architecture

```text
HTTP / OpenAPI                api/
       │                      routes, middleware, lifecycle
       ▼
Use-case orchestration       application/
       │                      normalization and coherent-series validation
       ▼
Forecasting domain           domain/
                              contracts, ensemble, intervals, explanations

Cross-cutting                config.py + observability/
                              environment validation, logs, metrics
```

The domain performs no I/O. That keeps the algorithm deterministic and lets future adapters add
feature stores or upstream providers without coupling them to the forecasting math.

## Run locally

```bash
python -m venv .venv
source .venv/bin/activate
python -m pip install -e '.[dev]'
supplymize-forecasting
```

Then open [http://127.0.0.1:8080/docs](http://127.0.0.1:8080/docs), or call the API directly:

```bash
curl --fail-with-body http://127.0.0.1:8080/v1/forecasts \
  --header 'content-type: application/json' \
  --header 'x-request-id: local-demo-1' \
  --data '{"sku":"SKU-DEMO","history":[18,20,19,22,23,24,22,25,26,24,27,29,28,30],"horizon":7}'
```

### Configuration

Settings use the `SUPPLYMIZE_FORECAST_` prefix and are validated at startup.

| Variable | Default | Notes |
|---|---:|---|
| `SUPPLYMIZE_FORECAST_HOST` | `127.0.0.1` | Bind address; the container overrides it to `0.0.0.0` |
| `SUPPLYMIZE_FORECAST_PORT` | `8080` | HTTP port |
| `SUPPLYMIZE_FORECAST_LOG_LEVEL` | `INFO` | JSON log level |
| `SUPPLYMIZE_FORECAST_MINIMUM_HISTORY_POINTS` | `7` | Minimum observations |
| `SUPPLYMIZE_FORECAST_RECENCY_WINDOW` | `56` | Maximum observations in level/trend fit |
| `SUPPLYMIZE_FORECAST_INFERRED_SEASONALITY_PERIOD` | `7` | Default inferred period |
| `SUPPLYMIZE_FORECAST_STOCKOUT_UPLIFT` | `0.10` | Censored-observation adjustment |
| `SUPPLYMIZE_FORECAST_UNCERTAINTY_FLOOR_RATIO` | `0.05` | Minimum interval standard deviation |

## Quality checks

```bash
ruff check .
ruff format --check .
pytest --cov=supplymize_forecasting --cov-report=term-missing
```

Tests cover determinism, contribution arithmetic, targeted signals, interval growth, date
normalization, invalid series, health/readiness, metrics, and the public integration contract.

## Container

The image is multi-stage, contains only the installed wheel and runtime dependencies, runs as a
non-root user, and includes a liveness check.

```bash
docker build -t supplymize/forecasting:local .
docker run --rm -p 8080:8080 supplymize/forecasting:local
```

## Operational boundaries

- This deterministic ensemble is a strong baseline and fallback, not a substitute for a validated
  probabilistic model on high-volume or strongly intermittent series.
- A stockout marker applies a configurable uplift because observed sales are a lower bound; it
  does not reconstruct lost sales.
- Supplier speed models availability pressure. It is intentionally disclosed separately from
  consumer-demand drivers.
- Custom weights must be non-negative and sum exactly to `1.0`.
