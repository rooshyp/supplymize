FROM rust:1.85-slim-bookworm AS builder

WORKDIR /build
COPY . .
RUN cargo test --release --all-targets \
    && cargo build --release --bin supplymize-optimizer

FROM debian:bookworm-slim AS runtime

RUN apt-get update \
    && apt-get install --yes --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 supplymize \
    && useradd --uid 10001 --gid supplymize --no-create-home --shell /usr/sbin/nologin supplymize

COPY --from=builder /build/target/release/supplymize-optimizer /usr/local/bin/supplymize-optimizer

USER 10001:10001
EXPOSE 8082
ENV SUPPLYMIZE_OPTIMIZER_ADDR=0.0.0.0:8082 \
    SUPPLYMIZE_LOG_FORMAT=json \
    RUST_LOG=supplymize_optimizer=info,tower_http=info

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl --fail --silent --show-error "http://127.0.0.1:${SUPPLYMIZE_OPTIMIZER_ADDR##*:}/healthz" || exit 1

ENTRYPOINT ["/usr/local/bin/supplymize-optimizer"]
