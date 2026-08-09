use std::time::Duration;

use axum::{
    body::{to_bytes, Body},
    http::{Request, StatusCode},
};
use serde_json::{json, Value};
use supplymize_optimizer::{router, AppState};
use tower::ServiceExt;

fn test_app() -> axum::Router {
    router(AppState::default(), 1024 * 1024, Duration::from_secs(5))
}

fn valid_payload() -> Value {
    json!({
        "sku": "FILTER-HEPA-01",
        "currency": "USD",
        "carbon_price_per_kg": 2.0,
        "lost_sale_penalty": 20.0,
        "average_storage_days": 1.0,
        "suppliers": [{
            "id": "supplier-west",
            "capacity_units": 60.0
        }],
        "warehouses": [{
            "id": "dc-seattle",
            "capacity_units": 100.0,
            "storage_cost_per_unit_day": 0.5
        }],
        "lanes": [{
            "id": "west-to-seattle",
            "supplier_id": "supplier-west",
            "warehouse_id": "dc-seattle",
            "distance_km": 100.0,
            "transport_cost_per_unit_km": 0.01,
            "carbon_kg_per_unit_km": 0.001,
            "truck_capacity_units": 50.0,
            "max_trucks": 2
        }],
        "demand": [{
            "warehouse_id": "dc-seattle",
            "units": 100.0
        }]
    })
}

#[tokio::test]
async fn health_and_readiness_are_available() {
    for path in ["/healthz", "/readyz"] {
        let response = test_app()
            .oneshot(Request::get(path).body(Body::empty()).expect("request"))
            .await
            .expect("response");
        assert_eq!(response.status(), StatusCode::OK);
    }
}

#[tokio::test]
async fn optimize_returns_contract_fields_and_request_id_header() {
    let response = test_app()
        .oneshot(
            Request::post("/v1/optimize")
                .header("content-type", "application/json")
                .header("x-request-id", "integration-smoke-001")
                .body(Body::from(valid_payload().to_string()))
                .expect("request"),
        )
        .await
        .expect("response");

    assert_eq!(response.status(), StatusCode::OK);
    assert_eq!(
        response
            .headers()
            .get("x-request-id")
            .and_then(|value| value.to_str().ok()),
        Some("integration-smoke-001")
    );
    let bytes = to_bytes(response.into_body(), 1024 * 1024)
        .await
        .expect("response body");
    let body: Value = serde_json::from_slice(&bytes).expect("JSON response");
    assert_eq!(body["request_id"], "integration-smoke-001");
    assert_eq!(body["fulfilled_demand"], 60.0);
    assert_eq!(body["unfulfilled_demand"], 40.0);
    assert_eq!(body["costs"]["total"], 902.0);
    assert_eq!(body["carbon_kg"], 6.0);
    assert!(body["explanations"]
        .as_array()
        .is_some_and(|v| !v.is_empty()));
}

#[tokio::test]
async fn validation_errors_are_structured_and_metrics_count_them() {
    let state = AppState::default();
    let app = router(state, 1024 * 1024, Duration::from_secs(5));
    let mut payload = valid_payload();
    payload["lanes"][0]["warehouse_id"] = json!("missing");

    let response = app
        .clone()
        .oneshot(
            Request::post("/v1/optimize")
                .header("content-type", "application/json")
                .body(Body::from(payload.to_string()))
                .expect("request"),
        )
        .await
        .expect("response");
    assert_eq!(response.status(), StatusCode::UNPROCESSABLE_ENTITY);

    let bytes = to_bytes(response.into_body(), 1024 * 1024)
        .await
        .expect("response body");
    let body: Value = serde_json::from_slice(&bytes).expect("JSON response");
    assert_eq!(body["error"]["code"], "validation_failed");
    assert!(body["error"]["details"]
        .as_array()
        .is_some_and(|v| !v.is_empty()));

    let metrics = app
        .oneshot(
            Request::get("/metrics")
                .body(Body::empty())
                .expect("request"),
        )
        .await
        .expect("metrics response");
    let text = String::from_utf8(
        to_bytes(metrics.into_body(), 1024 * 1024)
            .await
            .expect("metrics body")
            .to_vec(),
    )
    .expect("UTF-8 metrics");
    assert!(text.contains("supplymize_optimizer_requests_failed_total 1"));
}

#[tokio::test]
async fn malformed_json_is_a_bad_request() {
    let response = test_app()
        .oneshot(
            Request::post("/v1/optimize")
                .header("content-type", "application/json")
                .body(Body::from("{"))
                .expect("request"),
        )
        .await
        .expect("response");
    assert_eq!(response.status(), StatusCode::BAD_REQUEST);
}
