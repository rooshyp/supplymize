# SupplyMize API contracts

These snapshots are the reviewed boundaries between clients and the three backend services.

| Contract | Owner | Source of truth |
| --- | --- | --- |
| [`control-plane.yaml`](control-plane.yaml) | Go control plane | Public planning API and domain types |
| [`forecasting.json`](forecasting.json) | Python forecasting | Generated from FastAPI/Pydantic with `create_app().openapi()` |
| [`optimizer.yaml`](optimizer.yaml) | Rust optimizer | Maintained with the Axum request and response domain types |

Contract-breaking changes require a new versioned route. Regenerate the forecasting snapshot after changing a Pydantic API model, and update examples and adapter tests with every reviewed contract change.
