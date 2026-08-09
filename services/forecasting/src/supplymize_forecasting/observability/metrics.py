"""Small thread-safe Prometheus exposition without a runtime client dependency."""

from __future__ import annotations

import threading
from collections import defaultdict


class MetricsRegistry:
    """Capture bounded-cardinality HTTP and forecasting metrics."""

    def __init__(self) -> None:
        self._lock = threading.Lock()
        self._http_requests: defaultdict[tuple[str, str, str], float] = defaultdict(float)
        self._http_duration_sum: defaultdict[tuple[str, str], float] = defaultdict(float)
        self._http_duration_count: defaultdict[tuple[str, str], float] = defaultdict(float)
        self._forecasts: defaultdict[str, float] = defaultdict(float)
        self._forecast_points = 0.0

    def record_http(
        self,
        *,
        method: str,
        route: str,
        status_code: int,
        duration_seconds: float,
    ) -> None:
        key = (method, route, str(status_code))
        duration_key = (method, route)
        with self._lock:
            self._http_requests[key] += 1
            self._http_duration_sum[duration_key] += duration_seconds
            self._http_duration_count[duration_key] += 1

    def record_forecast(self, *, outcome: str, points: int = 0) -> None:
        with self._lock:
            self._forecasts[outcome] += 1
            self._forecast_points += points

    def render(self) -> str:
        """Render an atomic snapshot in Prometheus text format."""

        with self._lock:
            requests = dict(self._http_requests)
            duration_sum = dict(self._http_duration_sum)
            duration_count = dict(self._http_duration_count)
            forecasts = dict(self._forecasts)
            forecast_points = self._forecast_points

        lines = [
            "# HELP supplymize_forecast_http_requests_total HTTP requests handled.",
            "# TYPE supplymize_forecast_http_requests_total counter",
        ]
        for (method, route, status), value in sorted(requests.items()):
            labels = self._labels(method=method, route=route, status=status)
            lines.append(f"supplymize_forecast_http_requests_total{{{labels}}} {value:g}")

        lines.extend(
            [
                "# HELP supplymize_forecast_http_request_duration_seconds HTTP request duration.",
                "# TYPE supplymize_forecast_http_request_duration_seconds summary",
            ]
        )
        for (method, route), value in sorted(duration_count.items()):
            labels = self._labels(method=method, route=route)
            lines.append(
                f"supplymize_forecast_http_request_duration_seconds_count{{{labels}}} {value:g}"
            )
            lines.append(
                "supplymize_forecast_http_request_duration_seconds_sum"
                f"{{{labels}}} {duration_sum[(method, route)]:.9f}"
            )

        lines.extend(
            [
                "# HELP supplymize_forecasts_total Forecast operations by outcome.",
                "# TYPE supplymize_forecasts_total counter",
            ]
        )
        for outcome, value in sorted(forecasts.items()):
            labels = self._labels(outcome=outcome)
            lines.append(f"supplymize_forecasts_total{{{labels}}} {value:g}")
        lines.extend(
            [
                "# HELP supplymize_forecast_points_total Forecast points produced.",
                "# TYPE supplymize_forecast_points_total counter",
                f"supplymize_forecast_points_total {forecast_points:g}",
                "",
            ]
        )
        return "\n".join(lines)

    @classmethod
    def _labels(cls, **labels: str) -> str:
        return ",".join(f'{name}="{cls._escape(value)}"' for name, value in sorted(labels.items()))

    @staticmethod
    def _escape(value: str) -> str:
        return value.replace("\\", "\\\\").replace("\n", "\\n").replace('"', '\\"')
