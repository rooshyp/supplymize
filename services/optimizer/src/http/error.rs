use axum::{http::StatusCode, response::IntoResponse, Json};
use serde::Serialize;

use crate::{application::OptimizationError, domain::ValidationIssue};

#[derive(Debug)]
pub struct ApiError {
    status: StatusCode,
    code: &'static str,
    message: String,
    details: Vec<ValidationIssue>,
}

impl ApiError {
    pub(super) fn extraction(status: StatusCode, message: impl Into<String>) -> Self {
        let code = if status == StatusCode::PAYLOAD_TOO_LARGE {
            "payload_too_large"
        } else {
            "invalid_json"
        };
        Self {
            status,
            code,
            message: message.into(),
            details: Vec::new(),
        }
    }

    pub(super) fn internal() -> Self {
        Self {
            status: StatusCode::INTERNAL_SERVER_ERROR,
            code: "internal_error",
            message: "the optimization could not be completed".to_owned(),
            details: Vec::new(),
        }
    }
}

impl From<OptimizationError> for ApiError {
    fn from(error: OptimizationError) -> Self {
        match error {
            OptimizationError::Validation(details) => Self {
                status: StatusCode::UNPROCESSABLE_ENTITY,
                code: "validation_failed",
                message: "request contains invalid network data".to_owned(),
                details,
            },
            OptimizationError::Solver(message) => {
                tracing::error!(error = %message, "optimization solver failed");
                Self::internal()
            }
        }
    }
}

impl IntoResponse for ApiError {
    fn into_response(self) -> axum::response::Response {
        let body = ErrorEnvelope {
            error: ErrorBody {
                code: self.code,
                message: self.message,
                details: self.details,
            },
        };
        (self.status, Json(body)).into_response()
    }
}

#[derive(Debug, Serialize)]
struct ErrorEnvelope {
    error: ErrorBody,
}

#[derive(Debug, Serialize)]
struct ErrorBody {
    code: &'static str,
    message: String,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    details: Vec<ValidationIssue>,
}
