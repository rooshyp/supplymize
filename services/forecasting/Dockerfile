FROM python:3.12-slim AS builder

ENV PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PIP_NO_CACHE_DIR=1

WORKDIR /build
COPY pyproject.toml README.md ./
COPY src ./src
RUN python -m pip wheel --wheel-dir /wheels .

FROM python:3.12-slim AS runtime

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    SUPPLYMIZE_FORECAST_HOST=0.0.0.0 \
    SUPPLYMIZE_FORECAST_PORT=8080

RUN groupadd --system supplymize \
    && useradd --system --gid supplymize --create-home supplymize

COPY --from=builder /wheels /wheels
RUN python -m pip install --no-cache-dir /wheels/* \
    && rm -rf /wheels

USER supplymize
WORKDIR /home/supplymize
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD python -c "import os, urllib.request; port=os.getenv('SUPPLYMIZE_FORECAST_PORT', '8080'); urllib.request.urlopen(f'http://127.0.0.1:{port}/healthz', timeout=2)"

CMD ["supplymize-forecasting"]
