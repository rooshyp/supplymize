from __future__ import annotations

from datetime import date

import pytest
from pydantic import ValidationError

from supplymize_forecasting.application.service import ForecastService
from supplymize_forecasting.config import Settings
from supplymize_forecasting.domain.contracts import ForecastRequest, SupplierSpeedSignal
from supplymize_forecasting.domain.forecaster import ForecastInputError


@pytest.fixture
def settings() -> Settings:
    return Settings(
        minimum_history_points=7,
        maximum_history_points=3650,
        recency_window=56,
        inferred_seasonality_period=7,
        decimal_places=3,
    )


@pytest.fixture
def service(settings: Settings) -> ForecastService:
    return ForecastService(settings)


def test_constant_series_is_deterministic_and_explainable(service: ForecastService) -> None:
    request = ForecastRequest(sku="SKU-100", history=[100] * 21, horizon=5)

    first = service.create_forecast(request)
    second = service.create_forecast(request)

    assert first == second
    assert first.forecast.total_demand == pytest.approx(500)
    assert len(first.forecast.points) == 5
    assert [explanation.driver for explanation in first.explanations] == [
        "history",
        "trend",
        "seasonality",
        "weather",
        "event",
        "supplier_speed",
    ]
    for point in first.forecast.points:
        assert point.demand == pytest.approx(sum(point.components.model_dump().values()))
        assert point.lower_bound <= point.demand <= point.upper_bound


def test_external_signals_move_only_the_targeted_period(service: ForecastService) -> None:
    baseline = service.create_forecast(
        ForecastRequest(sku="SKU-SIGNAL", history=[100] * 21, horizon=3)
    )
    signaled = service.create_forecast(
        ForecastRequest.model_validate(
            {
                "sku": "SKU-SIGNAL",
                "history": [100] * 21,
                "horizon": 3,
                "signals": {
                    "weather": [{"impact": 0.5, "confidence": 1, "step": 2}],
                    "event": [{"impact": -0.25, "confidence": 1, "step": 3}],
                    "supplier_speed": [
                        {
                            "lead_time_days": 5,
                            "baseline_lead_time_days": 10,
                            "confidence": 1,
                            "step": 1,
                        }
                    ],
                },
            }
        )
    )

    baseline_points = baseline.forecast.points
    signaled_points = signaled.forecast.points
    assert signaled_points[0].demand == pytest.approx(baseline_points[0].demand + 2)
    assert signaled_points[1].demand == pytest.approx(baseline_points[1].demand + 4)
    assert signaled_points[2].demand == pytest.approx(baseline_points[2].demand - 2)
    weather_explanation = next(item for item in signaled.explanations if item.driver == "weather")
    event_explanation = next(item for item in signaled.explanations if item.driver == "event")
    assert weather_explanation.direction == "up"
    assert event_explanation.direction == "down"


def test_supplied_trend_and_seasonality_are_normalized(service: ForecastService) -> None:
    response = service.create_forecast(
        ForecastRequest.model_validate(
            {
                "sku": "SKU-PATTERN",
                "history": [100] * 21,
                "horizon": 4,
                "signals": {"trend": 0.02, "seasonality": [0.5, 1.5]},
            }
        )
    )

    first, second, third, fourth = response.forecast.points
    assert first.components.seasonality < second.components.seasonality
    assert first.components.seasonality == third.components.seasonality
    assert second.components.seasonality == fourth.components.seasonality
    assert first.components.trend < fourth.components.trend


def test_interval_width_grows_with_horizon(service: ForecastService) -> None:
    response = service.create_forecast(
        ForecastRequest(sku="SKU-INTERVAL", history=[100] * 21, horizon=20)
    )
    widths = [point.upper_bound - point.lower_bound for point in response.forecast.points]

    assert widths[-1] > widths[0]
    assert response.confidence.residual_rmse == 0
    assert response.confidence.level == 0.9


def test_dated_history_is_sorted_and_dates_anchor_output(service: ForecastService) -> None:
    request = ForecastRequest.model_validate(
        {
            "sku": "SKU-DATED",
            "history": [
                {"date": f"2026-01-{day:02d}", "quantity": 10 + day}
                for day in (7, 1, 6, 2, 5, 3, 4)
            ],
            "horizon": 2,
        }
    )
    response = service.create_forecast(request)

    assert response.forecast.points[0].date == date(2026, 1, 8)
    assert response.forecast.points[1].date == date(2026, 1, 9)


def test_mixed_dated_and_numeric_history_is_rejected(service: ForecastService) -> None:
    request = ForecastRequest.model_validate(
        {
            "sku": "SKU-MIXED",
            "history": [{"date": "2026-01-01", "demand": 10}, 11, 12, 13, 14, 15, 16],
            "horizon": 2,
        }
    )

    with pytest.raises(ForecastInputError, match="every observation or for none"):
        service.create_forecast(request)


def test_dated_signal_requires_an_anchor(service: ForecastService) -> None:
    request = ForecastRequest.model_validate(
        {
            "sku": "SKU-NO-ANCHOR",
            "history": [10] * 7,
            "horizon": 2,
            "signals": {"weather": [{"impact": 0.2, "date": "2026-01-01"}]},
        }
    )

    with pytest.raises(ForecastInputError, match="dated signals require"):
        service.create_forecast(request)


def test_supplier_speed_contract_requires_one_source() -> None:
    with pytest.raises(ValidationError, match="provide speed_index or both"):
        SupplierSpeedSignal()

    with pytest.raises(ValidationError, match="not both"):
        SupplierSpeedSignal(
            speed_index=0.2,
            lead_time_days=5,
            baseline_lead_time_days=7,
        )
