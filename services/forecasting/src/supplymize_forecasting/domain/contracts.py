"""Versioned API and domain contracts for demand forecasting."""

from __future__ import annotations

from datetime import date as Date
from typing import Annotated, Literal, Union

from pydantic import (
    AliasChoices,
    BaseModel,
    ConfigDict,
    Field,
    computed_field,
    model_validator,
)


class ContractModel(BaseModel):
    """Shared strict contract behavior for reliable service boundaries."""

    model_config = ConfigDict(
        extra="forbid",
        str_strip_whitespace=True,
        populate_by_name=True,
    )


class DemandObservation(ContractModel):
    """One observed demand value; the date is optional for ordered series."""

    date: Date | None = Field(
        default=None,
        description="Observation date. Either provide a date for every point or for none.",
    )
    demand: float = Field(
        ge=0,
        validation_alias=AliasChoices("demand", "quantity"),
        description="Observed units demanded. The alias 'quantity' is accepted.",
    )
    stockout: bool = Field(
        default=False,
        description="Marks a censored observation that may understate true demand.",
    )


NumericDemand = Annotated[float, Field(ge=0)]
# ``Union`` keeps local tooling on Python 3.9 usable; production targets Python 3.11+.
HistoryEntry = Union[NumericDemand, DemandObservation]  # noqa: UP007


class ImpactSignal(ContractModel):
    """Expected fractional demand impact for one or all forecast periods."""

    impact: float = Field(
        ge=-1,
        le=1,
        description="Expected fractional demand impact; 0.10 represents a 10% uplift.",
    )
    confidence: float = Field(
        default=1,
        ge=0,
        le=1,
        description="Confidence applied to the impact before it enters the ensemble.",
    )
    date: Date | None = Field(default=None, description="Optional target forecast date.")
    step: int | None = Field(
        default=None,
        ge=1,
        le=365,
        description="Optional one-based target forecast step.",
    )
    label: str | None = Field(default=None, min_length=1, max_length=120)

    @model_validator(mode="after")
    def validate_placement(self) -> ImpactSignal:
        if self.date is not None and self.step is not None:
            raise ValueError("a signal may target either date or step, not both")
        return self


class SupplierSpeedSignal(ContractModel):
    """Supplier velocity represented directly or derived from lead-time change.

    Positive ``speed_index`` means faster supply and a positive availability effect;
    negative means slower supply. When lead times are supplied, the index is derived
    as ``(baseline_lead_time_days - lead_time_days) / baseline_lead_time_days``.
    """

    speed_index: float | None = Field(default=None, ge=-1, le=1)
    lead_time_days: float | None = Field(default=None, ge=0, le=3650)
    baseline_lead_time_days: float | None = Field(default=None, gt=0, le=3650)
    confidence: float = Field(default=1, ge=0, le=1)
    date: Date | None = None
    step: int | None = Field(default=None, ge=1, le=365)
    supplier: str | None = Field(default=None, min_length=1, max_length=120)

    @model_validator(mode="after")
    def validate_speed_source_and_placement(self) -> SupplierSpeedSignal:
        raw_lead_time = self.lead_time_days is not None or self.baseline_lead_time_days is not None
        if self.speed_index is not None and raw_lead_time:
            raise ValueError("provide speed_index or lead-time values, not both")
        if self.speed_index is None and not (
            self.lead_time_days is not None and self.baseline_lead_time_days is not None
        ):
            raise ValueError(
                "provide speed_index or both lead_time_days and baseline_lead_time_days"
            )
        if self.date is not None and self.step is not None:
            raise ValueError("a signal may target either date or step, not both")
        return self

    @computed_field
    @property
    def resolved_speed_index(self) -> float:
        """Return the bounded input or lead-time-derived speed index."""

        if self.speed_index is not None:
            return self.speed_index
        assert self.lead_time_days is not None
        assert self.baseline_lead_time_days is not None
        derived = (self.baseline_lead_time_days - self.lead_time_days) / (
            self.baseline_lead_time_days
        )
        return max(-1.0, min(1.0, derived))


class ForecastSignals(ContractModel):
    """Optional external drivers blended with signals inferred from history."""

    trend: float | None = Field(
        default=None,
        ge=-0.25,
        le=0.25,
        description="Optional daily growth rate. If omitted, a robust linear trend is inferred.",
    )
    seasonality: list[Annotated[float, Field(gt=0, le=5)]] | None = Field(
        default=None,
        min_length=2,
        max_length=366,
        description=(
            "Repeating relative factors beginning at forecast step one; factors are normalized "
            "to a mean of 1.0. If omitted, seasonality is inferred from history."
        ),
    )
    weather: list[ImpactSignal] = Field(default_factory=list, max_length=365)
    event: list[ImpactSignal] = Field(default_factory=list, max_length=365)
    supplier_speed: list[SupplierSpeedSignal] = Field(default_factory=list, max_length=365)


class SignalWeights(ContractModel):
    """Auditable weights for the six-candidate forecasting ensemble."""

    history: float = Field(default=0.50, ge=0, le=1)
    trend: float = Field(default=0.15, ge=0, le=1)
    seasonality: float = Field(default=0.15, ge=0, le=1)
    weather: float = Field(default=0.08, ge=0, le=1)
    event: float = Field(default=0.08, ge=0, le=1)
    supplier_speed: float = Field(default=0.04, ge=0, le=1)

    @model_validator(mode="after")
    def validate_sum(self) -> SignalWeights:
        total = sum(
            (
                self.history,
                self.trend,
                self.seasonality,
                self.weather,
                self.event,
                self.supplier_speed,
            )
        )
        if abs(total - 1.0) > 1e-8:
            raise ValueError(f"signal weights must sum to 1.0; received {total:.8f}")
        return self


class ForecastRequest(ContractModel):
    """Demand forecast request accepted by ``POST /v1/forecasts``."""

    sku: str = Field(min_length=1, max_length=128, examples=["SKU-1042"])
    location: str | None = Field(default=None, min_length=1, max_length=128)
    history: list[HistoryEntry] = Field(
        min_length=3,
        max_length=5000,
        description="Oldest-to-newest numeric demand values or dated demand objects.",
    )
    horizon: int = Field(default=14, ge=1, le=365, description="Forecast horizon in days.")
    as_of_date: Date | None = Field(
        default=None,
        description="Date immediately before forecast step one when history is not dated.",
    )
    confidence_level: float = Field(default=0.90, ge=0.50, le=0.99)
    signals: ForecastSignals = Field(default_factory=ForecastSignals)
    weights: SignalWeights = Field(default_factory=SignalWeights)


class ComponentContributions(ContractModel):
    """Weighted candidate values that sum to the point forecast."""

    history: float
    trend: float
    seasonality: float
    weather: float
    event: float
    supplier_speed: float


class ForecastPoint(ContractModel):
    """One explainable point estimate and prediction interval."""

    step: int
    date: Date | None
    demand: float = Field(ge=0)
    lower_bound: float = Field(ge=0)
    upper_bound: float = Field(ge=0)
    components: ComponentContributions


class ForecastResult(ContractModel):
    """Horizon-level forecast result."""

    total_demand: float = Field(ge=0)
    average_daily_demand: float = Field(ge=0)
    total_lower_bound: float = Field(ge=0)
    total_upper_bound: float = Field(ge=0)
    points: list[ForecastPoint]


class ConfidenceSummary(ContractModel):
    """Quality and uncertainty metadata for the forecast."""

    level: float = Field(ge=0.5, le=0.99)
    score: float = Field(ge=0, le=1)
    residual_rmse: float = Field(ge=0)
    method: Literal["normal-residual-with-horizon-growth"]
    interpretation: Literal["low", "moderate", "high"]


class DriverExplanation(ContractModel):
    """Aggregate contribution from one ensemble driver."""

    driver: Literal["history", "trend", "seasonality", "weather", "event", "supplier_speed"]
    weight: float = Field(ge=0, le=1)
    weighted_contribution: float
    adjustment_from_history_level: float
    direction: Literal["down", "neutral", "up"]
    detail: str


class ForecastResponse(ContractModel):
    """Stable, deterministic response for a single SKU forecast."""

    model_version: str
    sku: str
    location: str | None
    history_points_used: int
    forecast: ForecastResult
    confidence: ConfidenceSummary
    explanations: list[DriverExplanation]
    assumptions: list[str]


class HealthResponse(ContractModel):
    status: Literal["ok", "ready", "not_ready"]
    service: str
    version: str


class ModelInfoResponse(ContractModel):
    name: str
    version: str
    method: str
    drivers: list[str]
    deterministic: bool
    minimum_history_points: int
    maximum_horizon: int
