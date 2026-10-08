# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS builder
WORKDIR /src

# Dependencias primero: esta capa se reutiliza (y se cachea en CI/ECR) mientras go.mod/go.sum no cambien.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
# GO_TAGS=dev incluye las herramientas de desarrollo (/dev/*); vacío = binario de producción.
ARG GO_TAGS=""
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -tags "${GO_TAGS}" -o /out/api ./cmd/api

FROM alpine:3.23
# ca-certificates: TLS hacia RDS (DB_SSLMODE=require/verify-full) y Redis (rediss://).
RUN apk add --no-cache ca-certificates \
    && addgroup -S app && adduser -S -H -G app app
COPY --from=builder /out/api /usr/local/bin/api
USER app
EXPOSE 8080
# Mismo endpoint que usa el target group del ALB (/healthz); respeta PORT.
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -qO- "http://127.0.0.1:${PORT:-8080}/healthz" >/dev/null || exit 1
ENTRYPOINT ["/usr/local/bin/api"]
