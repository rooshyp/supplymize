"""Cross-cutting request identity, structured logging, and metrics."""

from __future__ import annotations

import json
import logging
import time
import uuid
from datetime import datetime, timezone

from fastapi import Request, Response
from starlette.middleware.base import BaseHTTPMiddleware, RequestResponseEndpoint
from starlette.types import ASGIApp

from supplymize_forecasting.observability.metrics import MetricsRegistry


class JsonFormatter(logging.Formatter):
    """Emit compact, machine-readable service logs."""

    def format(self, record: logging.LogRecord) -> str:
        payload: dict[str, object] = {
            # ``timezone.utc`` also lets Python 3.9 developer environments run the test suite.
            "timestamp": datetime.now(timezone.utc).isoformat(),  # noqa: UP017
            "level": record.levelname,
            "logger": record.name,
            "message": record.getMessage(),
        }
        for key in ("request_id", "method", "route", "status_code", "duration_ms"):
            if hasattr(record, key):
                payload[key] = getattr(record, key)
        if record.exc_info:
            payload["exception"] = self.formatException(record.exc_info)
        return json.dumps(payload, separators=(",", ":"), default=str)


def configure_logging(level: str) -> None:
    """Configure the root logger once for consistent container output."""

    root = logging.getLogger()
    root.setLevel(level)
    if not any(isinstance(handler.formatter, JsonFormatter) for handler in root.handlers):
        handler = logging.StreamHandler()
        handler.setFormatter(JsonFormatter())
        root.handlers.clear()
        root.addHandler(handler)


class RequestContextMiddleware(BaseHTTPMiddleware):
    """Attach request IDs and publish low-cardinality request telemetry."""

    def __init__(self, app: ASGIApp, metrics: MetricsRegistry) -> None:
        super().__init__(app)
        self._metrics = metrics
        self._logger = logging.getLogger("supplymize_forecasting.http")

    async def dispatch(
        self,
        request: Request,
        call_next: RequestResponseEndpoint,
    ) -> Response:
        supplied_request_id = request.headers.get("x-request-id", "")[:128]
        request_id = supplied_request_id or str(uuid.uuid4())
        request.state.request_id = request_id
        started = time.perf_counter()
        status_code = 500
        try:
            response = await call_next(request)
            status_code = response.status_code
        finally:
            duration = time.perf_counter() - started
            route_object = request.scope.get("route")
            route = getattr(route_object, "path", "unmatched")
            self._metrics.record_http(
                method=request.method,
                route=route,
                status_code=status_code,
                duration_seconds=duration,
            )
            self._logger.info(
                "request_complete",
                extra={
                    "request_id": request_id,
                    "method": request.method,
                    "route": route,
                    "status_code": status_code,
                    "duration_ms": round(duration * 1000, 3),
                },
            )
        response.headers["x-request-id"] = request_id
        return response
