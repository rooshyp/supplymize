# Operations guide

## Local stack

```bash
cp .env.example .env
docker compose up --build
```

| Surface | URL |
| --- | --- |
| Planning console | `http://localhost:4173` |
| Control plane | `http://localhost:8080` |
| Forecasting | `http://localhost:8001` |
| Optimizer | `http://localhost:8002` |
| Prometheus (observability profile) | `http://localhost:9090` |
| Grafana (observability profile) | `http://localhost:3000` |

Enable the optional observability stack with:

```bash
docker compose --profile observability up --build
```

## Configuration

Configuration is supplied through environment variables. Start from `.env.example`; never commit a populated `.env` file.

| Variable | Default | Purpose |
| --- | --- | --- |
| `SUPPLYMIZE_HTTP_ADDR` | `:8080` | Public API listen address |
| `SUPPLYMIZE_FORECAST_URL` | `http://forecasting:8001` | Forecast dependency |
| `SUPPLYMIZE_OPTIMIZER_URL` | `http://optimizer:8002` | Optimizer dependency |
| `SUPPLYMIZE_LOG_LEVEL` | `info` | Structured log verbosity |

Each service README documents its additional tuning parameters.

## Service-level indicators

Recommended initial production objectives:

- Planning API availability: **99.9%** over 30 days.
- Planning API p95 latency: **under 1.5 seconds** for the reference network size.
- Forecast completion rate: **99.5%** excluding rejected validation requests.
- Optimizer feasibility reporting: **100%** of accepted requests return either a plan or explicit infeasibility.

## Rollout

1. Run schema and unit tests.
2. Deploy internal services and wait for readiness.
3. Deploy the control plane at zero traffic.
4. Run the example planning request as a smoke test.
5. Shift ingress traffic gradually while watching error rate, latency, and unfulfilled-demand metrics.

All services are stateless, so rollback is a deployment revision change. Contract-breaking changes require a new `/v2` route.
