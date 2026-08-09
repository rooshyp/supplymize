"""Application service that validates, normalizes, and executes forecasts."""

from __future__ import annotations

from datetime import date, timedelta

from supplymize_forecasting.config import Settings
from supplymize_forecasting.domain.contracts import (
    DemandObservation,
    ForecastRequest,
    ForecastResponse,
    ImpactSignal,
    SupplierSpeedSignal,
)
from supplymize_forecasting.domain.forecaster import (
    ForecastInputError,
    PreparedForecast,
    WeightedDemandForecaster,
)


class ForecastService:
    """Use-case boundary for creating a demand forecast."""

    def __init__(
        self,
        settings: Settings,
        forecaster: WeightedDemandForecaster | None = None,
    ) -> None:
        self._settings = settings
        self._forecaster = forecaster or WeightedDemandForecaster(settings)

    def create_forecast(self, request: ForecastRequest) -> ForecastResponse:
        """Normalize a request and delegate to the deterministic domain model."""

        prepared = self._prepare(request)
        return self._forecaster.forecast(prepared)

    def _prepare(self, request: ForecastRequest) -> PreparedForecast:
        history_length = len(request.history)
        if history_length < self._settings.minimum_history_points:
            raise ForecastInputError(
                f"at least {self._settings.minimum_history_points} history points are required"
            )
        if history_length > self._settings.maximum_history_points:
            raise ForecastInputError(
                f"at most {self._settings.maximum_history_points} history points are supported"
            )

        records: list[tuple[date | None, float, bool]] = []
        for item in request.history:
            if isinstance(item, DemandObservation):
                records.append((item.date, item.demand, item.stockout))
            else:
                records.append((None, float(item), False))

        dated_count = sum(item_date is not None for item_date, _, _ in records)
        if dated_count not in (0, history_length):
            raise ForecastInputError(
                "history dates must be provided for every observation or for none"
            )

        dates: tuple[date, ...] | None = None
        if dated_count:
            records.sort(key=lambda record: record[0] or date.min)
            sorted_dates = tuple(record[0] for record in records)
            if len(set(sorted_dates)) != len(sorted_dates):
                raise ForecastInputError("history contains duplicate observation dates")
            dates = tuple(item_date for item_date in sorted_dates if item_date is not None)

        stockout_count = sum(stockout for _, _, stockout in records)
        values = tuple(
            value * (1 + self._settings.stockout_uplift) if stockout else value
            for _, value, stockout in records
        )
        if dates is None:
            positions = tuple(float(index) for index in range(history_length))
        else:
            first_date = dates[0]
            positions = tuple(float((item_date - first_date).days) for item_date in dates)

        latest_history_date = dates[-1] if dates is not None else None
        if (
            latest_history_date is not None
            and request.as_of_date is not None
            and request.as_of_date < latest_history_date
        ):
            raise ForecastInputError("as_of_date cannot be before the latest history date")
        anchor_date = request.as_of_date or latest_history_date
        self._validate_signal_targets(request, anchor_date)

        return PreparedForecast(
            request=request,
            values=values,
            positions=positions,
            dates=dates,
            anchor_date=anchor_date,
            stockout_count=stockout_count,
        )

    @staticmethod
    def _validate_signal_targets(request: ForecastRequest, anchor_date: date | None) -> None:
        impact_signals: tuple[ImpactSignal, ...] = (
            *request.signals.weather,
            *request.signals.event,
        )
        supplier_signals: tuple[SupplierSpeedSignal, ...] = tuple(request.signals.supplier_speed)
        all_signals: tuple[ImpactSignal | SupplierSpeedSignal, ...] = (
            *impact_signals,
            *supplier_signals,
        )

        for signal in all_signals:
            if signal.step is not None and signal.step > request.horizon:
                raise ForecastInputError(
                    f"signal step {signal.step} falls outside the {request.horizon}-day horizon"
                )
            if signal.date is None:
                continue
            if anchor_date is None:
                raise ForecastInputError(
                    "dated signals require dated history or an explicit as_of_date"
                )
            first_forecast_date = anchor_date + timedelta(days=1)
            last_forecast_date = anchor_date + timedelta(days=request.horizon)
            if not first_forecast_date <= signal.date <= last_forecast_date:
                raise ForecastInputError(
                    f"signal date {signal.date.isoformat()} falls outside forecast dates "
                    f"{first_forecast_date.isoformat()} through {last_forecast_date.isoformat()}"
                )
