use std::{sync::Arc, time::Instant};

use axum::{
    extract::{rejection::JsonRejection, State},
    http::{header, HeaderMap, HeaderValue, StatusCode},
    response::{IntoResponse, Response},
    Json,
};
use serde::Serialize;

use super::{ApiError, AppState};
use crate::domain::OptimizationRequest;

pub(super) async fn optimize(
    State(state): State<AppState>,
    headers: HeaderMap,
    payload: Result<Json<OptimizationRequest>, JsonRejection>,
) -> Result<Response, ApiError> {
    state.metrics.begin();
    let started = Instant::now();

    let request = match payload {
        Ok(Json(request)) => request,
        Err(rejection) => {
            state.metrics.finish(false, started.elapsed());
            return Err(ApiError::extraction(
                rejection.status(),
                rejection.body_text(),
            ));
        }
    };
    let sku = request.sku.clone();
    let optimizer = Arc::clone(&state.optimizer);
    let result = tokio::task::spawn_blocking(move || optimizer.optimize(request)).await;

    match result {
        Ok(Ok(mut response)) => {
            state.metrics.finish(true, started.elapsed());
            if let Some(request_id) = supplied_request_id(&headers) {
                response.request_id = request_id;
            }
            tracing::info!(
                request_id = %response.request_id,
                sku = %sku,
                fulfilled = response.fulfilled_demand,
                unfulfilled = response.unfulfilled_demand,
                total_cost = response.costs.total,
                "optimization completed"
            );
            let mut headers = HeaderMap::new();
            headers.insert(
                "x-request-id",
                HeaderValue::from_str(&response.request_id)
                    .expect("validated request IDs are valid header values"),
            );
            Ok((StatusCode::OK, headers, Json(response)).into_response())
        }
        Ok(Err(error)) => {
            state.metrics.finish(false, started.elapsed());
            Err(error.into())
        }
        Err(join_error) => {
            state.metrics.finish(false, started.elapsed());
            tracing::error!(error = %join_error, "optimizer worker terminated unexpectedly");
            Err(ApiError::internal())
        }
    }
}

fn supplied_request_id(headers: &HeaderMap) -> Option<String> {
    let request_id = headers.get("x-request-id")?.to_str().ok()?.trim();
    if request_id.is_empty()
        || request_id.len() > 128
        || !request_id
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || b"._:-".contains(&byte))
    {
        return None;
    }
    Some(request_id.to_owned())
}

pub(super) async fn health() -> Json<HealthResponse> {
    Json(HealthResponse::healthy())
}

pub(super) async fn ready() -> Json<HealthResponse> {
    // The service has no remote solver or datastore. Successfully constructing
    // AppState is sufficient readiness for this deterministic in-process model.
    Json(HealthResponse::ready())
}

pub(super) async fn metrics(State(state): State<AppState>) -> impl IntoResponse {
    (
        [(
            header::CONTENT_TYPE,
            "text/plain; version=0.0.4; charset=utf-8",
        )],
        state.metrics.render_prometheus(),
    )
}

pub(super) async fn openapi() -> impl IntoResponse {
    (
        [(header::CONTENT_TYPE, "application/yaml; charset=utf-8")],
        include_str!("../../openapi.yaml"),
    )
}

pub(super) async fn service_info() -> Json<ServiceInfo> {
    Json(ServiceInfo {
        name: "supplymize-optimizer",
        version: env!("CARGO_PKG_VERSION"),
        documentation: "/openapi.yaml",
        health: "/healthz",
        readiness: "/readyz",
    })
}

#[derive(Debug, Serialize)]
pub(super) struct HealthResponse {
    status: &'static str,
    service: &'static str,
    version: &'static str,
}

impl HealthResponse {
    fn healthy() -> Self {
        Self {
            status: "ok",
            service: "supplymize-optimizer",
            version: env!("CARGO_PKG_VERSION"),
        }
    }

    fn ready() -> Self {
        Self {
            status: "ready",
            service: "supplymize-optimizer",
            version: env!("CARGO_PKG_VERSION"),
        }
    }
}

#[derive(Debug, Serialize)]
pub(super) struct ServiceInfo {
    name: &'static str,
    version: &'static str,
    documentation: &'static str,
    health: &'static str,
    readiness: &'static str,
}
