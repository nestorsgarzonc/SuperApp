FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY . .
# GO_TAGS=dev incluye las herramientas de desarrollo (/dev/*); vacío = binario de producción.
ARG GO_TAGS=""
RUN go mod tidy && CGO_ENABLED=0 go build -tags "${GO_TAGS}" -o /out/api ./cmd/api

FROM alpine:3.20
RUN adduser -D -H appuser
COPY --from=builder /out/api /usr/local/bin/api
USER appuser
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/api"]
