# syntax=docker/dockerfile:1.7
FROM golang:1.23-alpine AS build

ARG VERSION=development
WORKDIR /src

COPY go.mod ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/control-plane \
    ./cmd/control-plane

FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/control-plane /control-plane

USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/control-plane"]
