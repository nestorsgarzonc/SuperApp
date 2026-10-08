FROM golang:1.27.1-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/auth ./cmd

FROM alpine:3.23
RUN addgroup -S auth && adduser -S auth -G auth && apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/auth /app/auth
USER auth
EXPOSE 8081
ENTRYPOINT ["/app/auth"]
