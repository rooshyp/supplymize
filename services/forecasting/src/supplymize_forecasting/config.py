"""Validated runtime configuration loaded from environment variables."""

from functools import lru_cache
from typing import Literal

from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    """Service configuration.

    Every setting can be overridden with the ``SUPPLYMIZE_FORECAST_`` prefix,
    for example ``SUPPLYMIZE_FORECAST_LOG_LEVEL=DEBUG``.
    """

    model_config = SettingsConfigDict(
        env_prefix="SUPPLYMIZE_FORECAST_",
        env_file=".env",
        env_file_encoding="utf-8",
        extra="ignore",
        case_sensitive=False,
    )

    service_name: str = "supplymize-forecasting"
    service_version: str = "1.0.0"
    environment: str = "development"
    host: str = "127.0.0.1"
    port: int = Field(default=8080, ge=1, le=65535)
    log_level: Literal["DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL"] = "INFO"

    minimum_history_points: int = Field(default=7, ge=3, le=365)
    maximum_history_points: int = Field(default=3650, ge=30, le=20_000)
    recency_window: int = Field(default=56, ge=7, le=730)
    inferred_seasonality_period: int = Field(default=7, ge=2, le=366)
    maximum_daily_trend_rate: float = Field(default=0.25, gt=0, le=1)
    stockout_uplift: float = Field(default=0.10, ge=0, le=1)
    uncertainty_floor_ratio: float = Field(default=0.05, ge=0, le=0.5)
    decimal_places: int = Field(default=3, ge=0, le=6)


@lru_cache(maxsize=1)
def get_settings() -> Settings:
    """Return a process-wide immutable-by-convention settings instance."""

    return Settings()
