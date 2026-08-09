# SupplyMize Optimizer

> Fast, explainable network allocation for resilient and lower-carbon supply chains.

The optimizer is SupplyMize's Rust decision engine. It allocates one SKU from suppliers to warehouse demand while minimizing four business costs:

\[
\min\; C_{transport} + C_{storage} + C_{lost\ sales} + C_{carbon}
\]

Every response includes the chosen lanes, implied truck counts, fulfillment, emissions, cost attribution, capacity utilization, and plain-language reasons behind the result.

## Why this service exists

- **Commercially grounded:** unfulfilled demand remains available as a priced decision, not an infeasible-model failure.
- **Capacity aware:** supplier throughput, warehouse receipts, truck size, and truck availability are hard constraints.
- **Climate aware:** lane emissions are measured in kg CO2e and monetized with an internal carbon price.
- **Auditable:** no opaque heuristic is used. Successive shortest path solves the continuous capacitated min-cost flow globally.
- **Operational:** bounded payloads, timeouts, structured errors, graceful shutdown, health probes, Prometheus metrics, and non-root container execution are included.

## Architecture

```text
HTTP / JSON              Application                 Domain
┌──────────────┐         ┌────────────────────┐      ┌──────────────────────┐
│ Axum routes  │ ──────▶ │ OptimizerService   │ ───▶ │ Nodes, lanes, demand │
│ errors/probes│         │ min-cost-flow core │      │ validation + results │
└──────────────┘         └────────────────────┘      └──────────────────────┘
       │                          │
       └── metrics + tracing      └── explainable cost decomposition
```

The transport layer never contains optimization rules. The application service can therefore be reused by a batch planner, benchmark harness, or future event consumer without HTTP.

## Model

For each lane from supplier \(s\) to warehouse \(w\), the model assigns continuous units \(x_{sw}\). It minimizes:

```text
Σ x × distance × transport_rate
+ Σ x × storage_rate × average_storage_days
+ Σ unmet × lost_sale_penalty
+ Σ x × distance × carbon_rate × carbon_price
```

Subject to:

```text
Σ outbound(supplier) ≤ supplier.capacity_units
Σ inbound(warehouse) ≤ warehouse.capacity_units
x(lane) ≤ lane.truck_capacity_units × lane.max_trucks
fulfilled(warehouse) + unfulfilled(warehouse) = demand(warehouse)
```

Units are continuous and costs are linear. `trucks` is an operational estimate computed as `ceil(units / truck_capacity_units)`; fixed truck dispatch charges and integer lot sizing are intentionally outside this model. Those assumptions are repeated in every response.

## Run locally

Requirements: Rust 1.85+.

```bash
cargo run
curl -fsS http://localhost:8082/healthz
curl -fsS -X POST http://localhost:8082/v1/optimize \
  -H 'content-type: application/json' \
  --data @examples/request.json | jq
```

The complete contract is in [`openapi.yaml`](./openapi.yaml) and is also served at `GET /openapi.yaml`.

## API surface

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/v1/optimize` | Solve and explain a network allocation |
| `GET` | `/healthz` | Liveness probe |
| `GET` | `/readyz` | Readiness probe |
| `GET` | `/metrics` | Prometheus text exposition |
| `GET` | `/openapi.yaml` | OpenAPI 3.1 contract |

Validation failures return HTTP `422` with stable field paths and error codes:

```json
{
  "error": {
    "code": "validation_failed",
    "message": "request contains invalid network data",
    "details": [
      {
        "field": "lanes[0].warehouse_id",
        "code": "unknown_reference",
        "message": "warehouse 'missing' does not exist"
      }
    ]
  }
}
```

## Configuration

| Variable | Default | Meaning |
|---|---:|---|
| `SUPPLYMIZE_OPTIMIZER_ADDR` | `0.0.0.0:8082` | Listen address |
| `SUPPLYMIZE_MAX_BODY_BYTES` | `2097152` | Maximum request body |
| `SUPPLYMIZE_REQUEST_TIMEOUT_SECONDS` | `15` | HTTP request timeout |
| `SUPPLYMIZE_LOG_FORMAT` | text | Set to `json` for structured logs |
| `RUST_LOG` | service + HTTP info | Standard tracing filter |

## Quality gates

```bash
cargo fmt --all --check
cargo test --all-targets
cargo clippy --all-targets --all-features -- -D warnings
```

Build a production image from this directory:

```bash
docker build -t supplymize/optimizer:local .
docker run --rm -p 8082:8082 supplymize/optimizer:local
```

The runtime image uses a dedicated, non-root UID and contains only the release binary and CA certificates.
