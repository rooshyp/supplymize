"""Deterministic, explainable multi-signal demand forecasting model."""

from __future__ import annotations

import math
from dataclasses import dataclass
from datetime import date, timedelta
from statistics import NormalDist

from supplymize_forecasting.config import Settings
from supplymize_forecasting.domain.contracts import (
    ComponentContributions,
    ConfidenceSummary,
    DriverExplanation,
    ForecastPoint,
    ForecastRequest,
    ForecastResponse,
    ForecastResult,
    ImpactSignal,
    SupplierSpeedSignal,
)


class ForecastInputError(ValueError):
    """Raised when valid JSON cannot form a coherent forecasting series."""


@dataclass(frozen=True)
class PreparedForecast:
    """Transport-independent, normalized model input."""

    request: ForecastRequest
    values: tuple[float, ...]
    positions: tuple[float, ...]
    dates: tuple[date, ...] | None
    anchor_date: date | None
    stockout_count: int


@dataclass(frozen=True)
class _PointRaw:
    point: ForecastPoint
    contributions: dict[str, float]
    adjustments: dict[str, float]


class WeightedDemandForecaster:
    """A transparent weighted ensemble with no external model dependency.

    Six candidate forecasts share one recent-history level. Trend and seasonality
    are inferred when not supplied, while weather, event, and supplier effects are
    explicit. The configured weights combine the candidates into one point estimate.
    """

    _DRIVERS = ("history", "trend", "seasonality", "weather", "event", "supplier_speed")

    def __init__(self, settings: Settings) -> None:
        self._settings = settings

    def forecast(self, prepared: PreparedForecast) -> ForecastResponse:
        """Create a deterministic forecast and complete explanation payload."""

        request = prepared.request
        level = self._recent_level(prepared.values)
        inferred_slope, intercept = self._linear_fit(prepared.positions, prepared.values, level)
        inferred_factors = self._infer_seasonality(
            prepared.values,
            prepared.positions,
            prepared.dates,
            inferred_slope,
            intercept,
        )
        residual_rmse = self._residual_rmse(
            prepared,
            inferred_slope,
            intercept,
            inferred_factors,
        )
        interval_sigma = max(
            residual_rmse,
            level * self._settings.uncertainty_floor_ratio,
        )
        z_score = NormalDist().inv_cdf((1 + request.confidence_level) / 2)

        raw_points: list[_PointRaw] = []
        for step in range(1, request.horizon + 1):
            point_date = (
                prepared.anchor_date + timedelta(days=step)
                if prepared.anchor_date is not None
                else None
            )
            delta = self._forecast_delta(prepared, step, point_date)
            raw_points.append(
                self._forecast_point(
                    prepared=prepared,
                    step=step,
                    point_date=point_date,
                    delta=delta,
                    level=level,
                    inferred_slope=inferred_slope,
                    inferred_factors=inferred_factors,
                    interval_sigma=interval_sigma,
                    z_score=z_score,
                )
            )

        points = [raw.point for raw in raw_points]
        total_demand = self._round(sum(point.point.demand for point in raw_points))
        total_lower = self._round(sum(point.point.lower_bound for point in raw_points))
        total_upper = self._round(sum(point.point.upper_bound for point in raw_points))
        confidence_score = self._confidence_score(
            values=prepared.values,
            residual_rmse=residual_rmse,
            request=request,
        )

        return ForecastResponse(
            model_version=self._settings.service_version,
            sku=request.sku,
            location=request.location,
            history_points_used=len(prepared.values),
            forecast=ForecastResult(
                total_demand=total_demand,
                average_daily_demand=self._round(total_demand / request.horizon),
                total_lower_bound=min(total_demand, total_lower),
                total_upper_bound=max(total_demand, total_upper),
                points=points,
            ),
            confidence=ConfidenceSummary(
                level=request.confidence_level,
                score=self._round(confidence_score),
                residual_rmse=self._round(residual_rmse),
                method="normal-residual-with-horizon-growth",
                interpretation=self._confidence_label(confidence_score),
            ),
            explanations=self._build_explanations(
                prepared=prepared,
                raw_points=raw_points,
                level=level,
                inferred_slope=inferred_slope,
                inferred_factors=inferred_factors,
            ),
            assumptions=self._build_assumptions(prepared),
        )

    def _forecast_point(
        self,
        *,
        prepared: PreparedForecast,
        step: int,
        point_date: date | None,
        delta: float,
        level: float,
        inferred_slope: float,
        inferred_factors: dict[int, float],
        interval_sigma: float,
        z_score: float,
    ) -> _PointRaw:
        request = prepared.request
        signals = request.signals
        weights = request.weights

        if signals.trend is None:
            trend_candidate = max(0.0, level + inferred_slope * delta)
        else:
            trend_candidate = max(0.0, level * ((1 + signals.trend) ** delta))

        seasonal_factor = self._seasonal_factor(
            prepared,
            step,
            point_date,
            inferred_factors,
        )
        weather_impact, weather_uncertainty = self._impact_for_period(
            signals.weather, step, point_date
        )
        event_impact, event_uncertainty = self._impact_for_period(signals.event, step, point_date)
        supplier_impact, supplier_uncertainty = self._supplier_impact_for_period(
            signals.supplier_speed, step, point_date
        )

        candidates = {
            "history": level,
            "trend": trend_candidate,
            "seasonality": max(0.0, level * seasonal_factor),
            "weather": max(0.0, level * (1 + weather_impact)),
            "event": max(0.0, level * (1 + event_impact)),
            "supplier_speed": max(0.0, level * (1 + supplier_impact)),
        }
        weight_values = {
            "history": weights.history,
            "trend": weights.trend,
            "seasonality": weights.seasonality,
            "weather": weights.weather,
            "event": weights.event,
            "supplier_speed": weights.supplier_speed,
        }
        raw_contributions = {
            driver: weight_values[driver] * candidates[driver] for driver in self._DRIVERS
        }
        raw_adjustments = {
            driver: weight_values[driver] * (candidates[driver] - level) for driver in self._DRIVERS
        }
        rounded_contributions = {
            driver: self._round(raw_contributions[driver]) for driver in self._DRIVERS
        }
        demand = self._round(sum(rounded_contributions.values()))

        horizon_growth = math.sqrt(1 + delta / max(len(prepared.values), 1))
        signal_uncertainty = level * (
            weights.weather * weather_uncertainty
            + weights.event * event_uncertainty
            + weights.supplier_speed * supplier_uncertainty
        )
        half_width = z_score * interval_sigma * horizon_growth + signal_uncertainty
        lower_bound = min(demand, self._round(max(0.0, demand - half_width)))
        upper_bound = max(demand, self._round(demand + half_width))

        return _PointRaw(
            point=ForecastPoint(
                step=step,
                date=point_date,
                demand=demand,
                lower_bound=lower_bound,
                upper_bound=upper_bound,
                components=ComponentContributions(**rounded_contributions),
            ),
            contributions=raw_contributions,
            adjustments=raw_adjustments,
        )

    def _recent_level(self, values: tuple[float, ...]) -> float:
        window = values[-self._settings.recency_window :]
        recency_weights = range(1, len(window) + 1)
        weighted_sum = sum(
            value * weight
            for value, weight in zip(window, recency_weights)  # noqa: B905
        )
        return weighted_sum / sum(recency_weights)

    def _linear_fit(
        self,
        positions: tuple[float, ...],
        values: tuple[float, ...],
        level: float,
    ) -> tuple[float, float]:
        window_size = min(len(values), self._settings.recency_window)
        x_values = positions[-window_size:]
        y_values = values[-window_size:]
        x_mean = sum(x_values) / window_size
        y_mean = sum(y_values) / window_size
        denominator = sum((x - x_mean) ** 2 for x in x_values)
        slope = (
            sum(
                (x - x_mean) * (y - y_mean)
                for x, y in zip(x_values, y_values)  # noqa: B905
            )
            / denominator
            if denominator > 0
            else 0.0
        )
        maximum_slope = level * self._settings.maximum_daily_trend_rate
        slope = max(-maximum_slope, min(maximum_slope, slope))
        intercept = y_mean - slope * x_mean
        return slope, intercept

    def _infer_seasonality(
        self,
        values: tuple[float, ...],
        positions: tuple[float, ...],
        dates: tuple[date, ...] | None,
        slope: float,
        intercept: float,
    ) -> dict[int, float]:
        period = self._settings.inferred_seasonality_period
        if len(values) < period * 2:
            return {slot: 1.0 for slot in range(period)}

        ratios: dict[int, list[float]] = {slot: [] for slot in range(period)}
        for index, (position, value) in enumerate(zip(positions, values)):  # noqa: B905
            fitted = max(intercept + slope * position, 1e-9)
            slot = dates[index].toordinal() % period if dates is not None else index % period
            ratios[slot].append(value / fitted)

        factors: dict[int, float] = {}
        for slot, slot_ratios in ratios.items():
            if not slot_ratios:
                factors[slot] = 1.0
                continue
            raw_factor = sum(slot_ratios) / len(slot_ratios)
            shrinkage = len(slot_ratios) / (len(slot_ratios) + 2)
            factors[slot] = max(0.5, min(1.5, 1 + shrinkage * (raw_factor - 1)))

        mean_factor = sum(factors.values()) / period
        if mean_factor <= 0:
            return {slot: 1.0 for slot in range(period)}
        return {slot: factor / mean_factor for slot, factor in factors.items()}

    def _seasonal_factor(
        self,
        prepared: PreparedForecast,
        step: int,
        point_date: date | None,
        inferred_factors: dict[int, float],
    ) -> float:
        supplied = prepared.request.signals.seasonality
        if supplied is not None:
            normalized_mean = sum(supplied) / len(supplied)
            return supplied[(step - 1) % len(supplied)] / normalized_mean

        period = self._settings.inferred_seasonality_period
        if point_date is not None and prepared.dates is not None:
            slot = point_date.toordinal() % period
        else:
            slot = (len(prepared.values) + step - 1) % period
        return inferred_factors.get(slot, 1.0)

    def _residual_rmse(
        self,
        prepared: PreparedForecast,
        slope: float,
        intercept: float,
        inferred_factors: dict[int, float],
    ) -> float:
        period = self._settings.inferred_seasonality_period
        residuals: list[float] = []
        for index, (position, observed) in enumerate(
            zip(prepared.positions, prepared.values)  # noqa: B905
        ):
            slot = (
                prepared.dates[index].toordinal() % period
                if prepared.dates is not None
                else index % period
            )
            fitted = max(0.0, intercept + slope * position) * inferred_factors.get(slot, 1.0)
            residuals.append(observed - fitted)
        degrees_of_freedom = max(1, len(residuals) - 2)
        return math.sqrt(sum(residual**2 for residual in residuals) / degrees_of_freedom)

    @staticmethod
    def _matches_period(
        signal_date: date | None,
        signal_step: int | None,
        step: int,
        point_date: date | None,
    ) -> bool:
        if signal_date is None and signal_step is None:
            return True
        if signal_step is not None:
            return signal_step == step
        return point_date is not None and signal_date == point_date

    def _impact_for_period(
        self,
        signals: list[ImpactSignal],
        step: int,
        point_date: date | None,
    ) -> tuple[float, float]:
        impact = 0.0
        uncertainty = 0.0
        for signal in signals:
            if self._matches_period(signal.date, signal.step, step, point_date):
                impact += signal.impact * signal.confidence
                uncertainty += abs(signal.impact) * (1 - signal.confidence)
        return max(-1.0, min(1.0, impact)), min(1.0, uncertainty)

    def _supplier_impact_for_period(
        self,
        signals: list[SupplierSpeedSignal],
        step: int,
        point_date: date | None,
    ) -> tuple[float, float]:
        impact = 0.0
        uncertainty = 0.0
        for signal in signals:
            if self._matches_period(signal.date, signal.step, step, point_date):
                speed = signal.resolved_speed_index
                impact += speed * signal.confidence
                uncertainty += abs(speed) * (1 - signal.confidence)
        return max(-1.0, min(1.0, impact)), min(1.0, uncertainty)

    @staticmethod
    def _forecast_delta(
        prepared: PreparedForecast,
        step: int,
        point_date: date | None,
    ) -> float:
        if prepared.dates is not None and point_date is not None:
            return float((point_date - prepared.dates[-1]).days)
        return float(step)

    def _confidence_score(
        self,
        *,
        values: tuple[float, ...],
        residual_rmse: float,
        request: ForecastRequest,
    ) -> float:
        mean_demand = sum(values) / len(values)
        relative_fit = 1 - residual_rmse / (max(mean_demand, 1.0) + residual_rmse)
        sample_strength = len(values) / (
            len(values) + 2 * self._settings.inferred_seasonality_period
        )
        external_uncertainty = [
            1 - signal.confidence
            for signal in (
                *request.signals.weather,
                *request.signals.event,
                *request.signals.supplier_speed,
            )
        ]
        signal_strength = (
            1 - sum(external_uncertainty) / len(external_uncertainty)
            if external_uncertainty
            else 1.0
        )
        score = 0.15 + 0.85 * relative_fit * sample_strength * signal_strength
        return max(0.0, min(0.99, score))

    @staticmethod
    def _confidence_label(score: float) -> str:
        if score >= 0.75:
            return "high"
        if score >= 0.50:
            return "moderate"
        return "low"

    def _build_explanations(
        self,
        *,
        prepared: PreparedForecast,
        raw_points: list[_PointRaw],
        level: float,
        inferred_slope: float,
        inferred_factors: dict[int, float],
    ) -> list[DriverExplanation]:
        request = prepared.request
        weights = request.weights.model_dump()
        contributions = {
            driver: sum(point.contributions[driver] for point in raw_points)
            for driver in self._DRIVERS
        }
        adjustments = {
            driver: sum(point.adjustments[driver] for point in raw_points)
            for driver in self._DRIVERS
        }
        active_weather = len(request.signals.weather)
        active_events = len(request.signals.event)
        active_suppliers = len(request.signals.supplier_speed)
        seasonal_range = max(inferred_factors.values()) - min(inferred_factors.values())
        details = {
            "history": (
                f"Recency-weighted demand level is {self._round(level)} units across "
                f"{len(prepared.values)} observations."
            ),
            "trend": (
                f"Uses the supplied daily growth rate of {request.signals.trend:.2%}."
                if request.signals.trend is not None
                else f"Inferred a bounded linear change of {self._round(inferred_slope)} units/day."
            ),
            "seasonality": (
                f"Uses {len(request.signals.seasonality)} supplied repeating factors."
                if request.signals.seasonality is not None
                else (
                    f"Inferred a {self._settings.inferred_seasonality_period}-period pattern "
                    f"with factor spread {self._round(seasonal_range)}."
                )
            ),
            "weather": f"Applied {active_weather} weather impact signal(s), confidence-scaled.",
            "event": f"Applied {active_events} event impact signal(s), confidence-scaled.",
            "supplier_speed": (
                f"Applied {active_suppliers} supplier-speed signal(s) as an availability proxy."
            ),
        }
        neutral_threshold = max(1e-9, level * request.horizon * 1e-6)
        explanations: list[DriverExplanation] = []
        for driver in self._DRIVERS:
            adjustment = adjustments[driver]
            direction = (
                "up"
                if adjustment > neutral_threshold
                else "down"
                if adjustment < -neutral_threshold
                else "neutral"
            )
            explanations.append(
                DriverExplanation(
                    driver=driver,
                    weight=weights[driver],
                    weighted_contribution=self._round(contributions[driver]),
                    adjustment_from_history_level=self._round(adjustment),
                    direction=direction,
                    detail=details[driver],
                )
            )
        return explanations

    def _build_assumptions(self, prepared: PreparedForecast) -> list[str]:
        assumptions = [
            "Demand history is ordered oldest to newest; dated history is sorted before fitting.",
            (
                "Weather, event, and supplier effects are fractional changes to the recent "
                "demand level."
            ),
            (
                "Supplier speed is an availability proxy, not a claim that supply creates "
                "consumer demand."
            ),
            (
                "Prediction intervals use fitted residual error, a "
                f"{self._settings.uncertainty_floor_ratio:.0%} uncertainty floor, and "
                "horizon growth."
            ),
            (
                "The model makes no network calls and returns identical results for identical "
                "inputs and configuration."
            ),
        ]
        if prepared.stockout_count:
            assumptions.append(
                f"Adjusted {prepared.stockout_count} stockout-censored observation(s) upward by "
                f"{self._settings.stockout_uplift:.0%}."
            )
        if prepared.dates is None and prepared.anchor_date is None:
            assumptions.append(
                "No date anchor was supplied, so forecast points are identified by step only."
            )
        return assumptions

    def _round(self, value: float) -> float:
        return round(value, self._settings.decimal_places)
