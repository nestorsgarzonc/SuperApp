FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/access ./cmd/access

FROM alpine:3.21
RUN addgroup -S access && adduser -S access -G access && apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/access /app/access
USER access
EXPOSE 4000
ENTRYPOINT ["/app/access"]
