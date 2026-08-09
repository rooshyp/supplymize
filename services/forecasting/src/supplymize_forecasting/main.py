"""ASGI entrypoint and local server command."""

from __future__ import annotations

import uvicorn

from supplymize_forecasting.api.app import create_app
from supplymize_forecasting.config import get_settings

app = create_app()


def run() -> None:
    """Run the service using validated environment configuration."""

    settings = get_settings()
    uvicorn.run(
        "supplymize_forecasting.main:app",
        host=settings.host,
        port=settings.port,
        log_level=settings.log_level.lower(),
        access_log=False,
    )


if __name__ == "__main__":
    run()
