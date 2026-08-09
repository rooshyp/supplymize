mod error;
mod handlers;
mod metrics;

use std::{sync::Arc, time::Duration};

use axum::{
    extract::DefaultBodyLimit,
    http::StatusCode,
    routing::{get, post},
    Router,
};
pub use error::ApiError;
use handlers::{health, metrics, openapi, optimize, ready, service_info};
pub use metrics::AppMetrics;
use tower_http::{timeout::TimeoutLayer, trace::TraceLayer};

use crate::application::OptimizerService;

#[derive(Clone, Debug)]
pub struct AppState {
    pub optimizer: Arc<OptimizerService>,
    pub metrics: Arc<AppMetrics>,
}

impl Default for AppState {
    fn default() -> Self {
        Self {
            optimizer: Arc::new(OptimizerService),
            metrics: Arc::new(AppMetrics::default()),
        }
    }
}

pub fn router(state: AppState, body_limit_bytes: usize, request_timeout: Duration) -> Router {
    Router::new()
        .route("/", get(service_info))
        .route("/v1/optimize", post(optimize))
        .route("/healthz", get(health))
        .route("/readyz", get(ready))
        .route("/metrics", get(metrics))
        .route("/openapi.yaml", get(openapi))
        .layer(DefaultBodyLimit::max(body_limit_bytes))
        .layer(TimeoutLayer::with_status_code(
            StatusCode::REQUEST_TIMEOUT,
            request_timeout,
        ))
        .layer(TraceLayer::new_for_http())
        .with_state(state)
}
