"""FastAPI application factory."""

from __future__ import annotations

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

from fastapi import FastAPI

from supplymize_forecasting.api.middleware import RequestContextMiddleware, configure_logging
from supplymize_forecasting.api.routes import build_router
from supplymize_forecasting.application.service import ForecastService
from supplymize_forecasting.config import Settings, get_settings
from supplymize_forecasting.observability.metrics import MetricsRegistry


def create_app(settings: Settings | None = None) -> FastAPI:
    """Create an independently configured application instance."""

    resolved_settings = settings or get_settings()
    configure_logging(resolved_settings.log_level)
    metrics = MetricsRegistry()
    service = ForecastService(resolved_settings)

    @asynccontextmanager
    async def lifespan(app: FastAPI) -> AsyncIterator[None]:
        app.state.ready = True
        yield
        app.state.ready = False

    app = FastAPI(
        title="SupplyMize Forecasting API",
        summary="Explainable multi-signal demand forecasting for resilient supply chains.",
        description=(
            "A deterministic forecasting boundary that blends demand history, trend, "
            "seasonality, weather, events, and supplier speed with auditable weights."
        ),
        version=resolved_settings.service_version,
        license_info={"name": "MIT"},
        docs_url="/docs",
        redoc_url="/redoc",
        openapi_url="/openapi.json",
        lifespan=lifespan,
    )
    app.state.ready = False
    app.state.settings = resolved_settings
    app.state.metrics = metrics
    app.state.forecast_service = service
    app.add_middleware(RequestContextMiddleware, metrics=metrics)
    app.include_router(build_router(settings=resolved_settings, service=service, metrics=metrics))
    return app
