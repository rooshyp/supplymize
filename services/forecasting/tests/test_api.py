from __future__ import annotations

from fastapi.testclient import TestClient

from supplymize_forecasting.api.app import create_app
from supplymize_forecasting.config import Settings


def build_client() -> TestClient:
    settings = Settings(
        environment="test",
        minimum_history_points=7,
        log_level="CRITICAL",
    )
    return TestClient(create_app(settings))


def test_health_readiness_and_model_info() -> None:
    with build_client() as client:
        health = client.get("/healthz")
        ready = client.get("/readyz")
        model_info = client.get("/v1/model/info")

    assert health.status_code == 200
    assert health.json()["status"] == "ok"
    assert ready.status_code == 200
    assert ready.json()["status"] == "ready"
    assert model_info.status_code == 200
    assert model_info.json()["deterministic"] is True
    assert "supplier_speed" in model_info.json()["drivers"]


def test_forecast_endpoint_exposes_integration_contract() -> None:
    payload = {
        "sku": "SKU-API-42",
        "location": "LAX-01",
        "history": [95, 101, 99, 104, 108, 107, 110, 112, 111, 115, 117, 116, 120, 122],
        "horizon": 3,
        "as_of_date": "2026-08-08",
        "signals": {
            "trend": 0.01,
            "seasonality": [0.9, 1.0, 1.1, 1.0, 1.05, 1.15, 0.8],
            "weather": [{"impact": 0.12, "confidence": 0.8, "step": 2}],
            "event": [{"impact": 0.20, "confidence": 0.9, "step": 3, "label": "Expo"}],
            "supplier_speed": [{"speed_index": -0.10, "confidence": 0.9}],
        },
    }

    with build_client() as client:
        response = client.post("/v1/forecasts", json=payload, headers={"x-request-id": "test-42"})

    assert response.status_code == 200
    assert response.headers["x-request-id"] == "test-42"
    body = response.json()
    assert body["sku"] == payload["sku"]
    assert body["forecast"]["total_demand"] > 0
    assert len(body["forecast"]["points"]) == payload["horizon"]
    assert 0 <= body["confidence"]["score"] <= 1
    assert len(body["explanations"]) == 6
    assert body["forecast"]["points"][0]["date"] == "2026-08-09"


def test_invalid_coherent_series_returns_actionable_422() -> None:
    payload = {
        "sku": "SKU-BAD",
        "history": [
            {"date": "2026-01-01", "demand": 10},
            {"date": "2026-01-01", "demand": 11},
            {"date": "2026-01-02", "demand": 12},
            {"date": "2026-01-03", "demand": 13},
            {"date": "2026-01-04", "demand": 14},
            {"date": "2026-01-05", "demand": 15},
            {"date": "2026-01-06", "demand": 16},
        ],
        "horizon": 2,
    }

    with build_client() as client:
        response = client.post("/v1/forecasts", json=payload)

    assert response.status_code == 422
    assert response.json()["detail"]["code"] == "invalid_forecast_input"
    assert "duplicate" in response.json()["detail"]["message"]


def test_metrics_are_prometheus_compatible() -> None:
    with build_client() as client:
        client.get("/healthz")
        client.post(
            "/v1/forecasts",
            json={"sku": "SKU-METRICS", "history": [10] * 14, "horizon": 2},
        )
        response = client.get("/metrics")

    assert response.status_code == 200
    assert "text/plain" in response.headers["content-type"]
    assert 'supplymize_forecasts_total{outcome="success"} 1' in response.text
    assert "supplymize_forecast_points_total 2" in response.text
