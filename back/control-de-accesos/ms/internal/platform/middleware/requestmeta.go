package middleware

import (
	"context"
	"net"
	"net/http"

	"github.com/google/uuid"
)

type metaKey string

const (
	ipAddressKey     metaKey = "ip_address"
	correlationIDKey metaKey = "correlation_id"
)

// WithRequestMeta stores the caller's IP and a correlation ID (reused from
// an incoming X-Correlation-ID/X-Request-ID header, or generated otherwise)
// into the request context, for audit logging. Apply once, globally.
func WithRequestMeta(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if host, _, err := net.SplitHostPort(ip); err == nil {
			ip = host
		}

		correlationID := r.Header.Get("X-Correlation-ID")
		if correlationID == "" {
			correlationID = r.Header.Get("X-Request-ID")
		}
		if correlationID == "" {
			correlationID = uuid.New().String()
		}

		ctx := context.WithValue(r.Context(), ipAddressKey, ip)
		ctx = context.WithValue(ctx, correlationIDKey, correlationID)
		w.Header().Set("X-Correlation-ID", correlationID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetRequestIP(ctx context.Context) string {
	if ip, ok := ctx.Value(ipAddressKey).(string); ok {
		return ip
	}
	return ""
}

func GetCorrelationID(ctx context.Context) string {
	if id, ok := ctx.Value(correlationIDKey).(string); ok {
		return id
	}
	return ""
}
