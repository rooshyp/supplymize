"""FastAPI route definitions for the forecasting boundary."""

from __future__ import annotations

from fastapi import APIRouter, HTTPException, Request, status
from fastapi.responses import PlainTextResponse

from supplymize_forecasting.application.service import ForecastService
from supplymize_forecasting.config import Settings
from supplymize_forecasting.domain.contracts import (
    ForecastRequest,
    ForecastResponse,
    HealthResponse,
    ModelInfoResponse,
)
from supplymize_forecasting.domain.forecaster import ForecastInputError
from supplymize_forecasting.observability.metrics import MetricsRegistry


def build_router(
    *,
    settings: Settings,
    service: ForecastService,
    metrics: MetricsRegistry,
) -> APIRouter:
    """Build a router with explicit dependencies for straightforward testing."""

    router = APIRouter()

    @router.get(
        "/healthz",
        response_model=HealthResponse,
        tags=["operations"],
        summary="Liveness probe",
    )
    def health() -> HealthResponse:
        return HealthResponse(
            status="ok",
            service=settings.service_name,
            version=settings.service_version,
        )

    @router.get(
        "/readyz",
        response_model=HealthResponse,
        responses={503: {"model": HealthResponse}},
        tags=["operations"],
        summary="Readiness probe",
    )
    def readiness(request: Request) -> HealthResponse:
        ready = bool(getattr(request.app.state, "ready", False))
        if not ready:
            raise HTTPException(
                status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
                detail={
                    "status": "not_ready",
                    "service": settings.service_name,
                    "version": settings.service_version,
                },
            )
        return HealthResponse(
            status="ready",
            service=settings.service_name,
            version=settings.service_version,
        )

    @router.get(
        "/metrics",
        response_class=PlainTextResponse,
        include_in_schema=False,
    )
    def prometheus_metrics() -> PlainTextResponse:
        return PlainTextResponse(
            metrics.render(),
            media_type="text/plain; version=0.0.4; charset=utf-8",
        )

    @router.get(
        "/v1/model/info",
        response_model=ModelInfoResponse,
        tags=["forecasting"],
        summary="Describe the active deterministic model",
    )
    def model_info() -> ModelInfoResponse:
        return ModelInfoResponse(
            name="weighted-multi-signal-demand-ensemble",
            version=settings.service_version,
            method="recency level + trend + seasonality + confidence-scaled external signals",
            drivers=[
                "history",
                "trend",
                "seasonality",
                "weather",
                "event",
                "supplier_speed",
            ],
            deterministic=True,
            minimum_history_points=settings.minimum_history_points,
            maximum_horizon=365,
        )

    @router.post(
        "/v1/forecasts",
        response_model=ForecastResponse,
        status_code=status.HTTP_200_OK,
        tags=["forecasting"],
        summary="Create an explainable demand forecast",
        responses={
            422: {
                "description": "The request schema or forecasting series is invalid.",
            }
        },
    )
    def create_forecast(request: ForecastRequest) -> ForecastResponse:
        try:
            response = service.create_forecast(request)
        except ForecastInputError as exc:
            metrics.record_forecast(outcome="rejected")
            raise HTTPException(
                status_code=status.HTTP_422_UNPROCESSABLE_CONTENT,
                detail={"code": "invalid_forecast_input", "message": str(exc)},
            ) from exc
        except Exception:
            metrics.record_forecast(outcome="error")
            raise
        metrics.record_forecast(outcome="success", points=request.horizon)
        return response

    return router
