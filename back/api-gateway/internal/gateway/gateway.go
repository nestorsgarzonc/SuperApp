// Package gateway is the single entry point between the front and the
// microservices. The microservices do not validate tokens: they trust the
// identity headers the gateway injects (protected by X-Gateway-Secret), so
// every rule in this file is a security rule.
package gateway

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"superapp/api-gateway/internal/apierr"
	"superapp/api-gateway/internal/authn"
	"superapp/api-gateway/internal/config"
	"superapp/api-gateway/internal/ratelimit"
)

const (
	HeaderGatewaySecret = "X-Gateway-Secret"
	HeaderUserID        = "X-User-ID"
	HeaderComplexID     = "X-Complex-ID"
	HeaderComplexRole   = "X-Complex-Role"
	HeaderRequestID     = "X-Request-ID"
	HeaderCorrelationID = "X-Correlation-ID"

	// Signed tenant context understood by the auth service.
	HeaderTenantID        = "X-Tenant-ID"
	HeaderTenantTimestamp = "X-Gateway-Timestamp"
	HeaderTenantSignature = "X-Gateway-Signature"
)

// spoofable are identity/trust headers a client must never be able to set.
// Prefix entries end with "-".
var spoofable = []string{
	HeaderUserID, HeaderComplexRole, HeaderTenantID, HeaderCorrelationID,
	"X-Gateway-", "X-Internal-", "X-Forwarded-", "Forwarded", "X-Real-Ip", "True-Client-Ip",
}

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

// Verifier checks access tokens.
type Verifier interface {
	Verify(ctx context.Context, raw string) (authn.Identity, error)
}

// RoleResolver returns the role a user holds in a complex.
type RoleResolver interface {
	Role(ctx context.Context, userID, complexID string) (role string, member bool, err error)
}

type Gateway struct {
	cfg      config.Config
	routes   []Route
	verifier Verifier
	roles    RoleResolver
	limiter  ratelimit.Limiter
	log      *slog.Logger
	proxies  map[string]*httputil.ReverseProxy
	cors     corsPolicy
	now      func() time.Time
}

func New(cfg config.Config, verifier Verifier, roles RoleResolver, limiter ratelimit.Limiter, log *slog.Logger) *Gateway {
	g := &Gateway{
		cfg: cfg, routes: BuildRoutes(cfg), verifier: verifier, roles: roles, limiter: limiter, log: log,
		proxies: map[string]*httputil.ReverseProxy{}, cors: newCORS(cfg.CORSAllowedOrigins), now: time.Now,
	}
	transport := &http.Transport{
		Proxy:                 nil, // upstreams live on the private network
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   50,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: cfg.UpstreamTimeout,
	}
	for _, r := range g.routes {
		if _, ok := g.proxies[r.Name]; !ok {
			g.proxies[r.Name] = g.newProxy(r.Upstream, transport)
		}
	}
	return g
}

type ctxKey int

const (
	upstreamPathKey ctxKey = iota
	clientKey
)

// client is who called the gateway, captured before headers are sanitised.
type client struct{ ip, proto string }

func (g *Gateway) newProxy(up config.Upstream, transport http.RoundTripper) *httputil.ReverseProxy {
	target := up.URL
	return &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(pr *httputil.ProxyRequest) {
			// Rewrite starts from a copy of the inbound request without
			// hop-by-hop and X-Forwarded-* headers; the identity headers were
			// already sanitised and injected by ServeHTTP.
			path, _ := pr.In.Context().Value(upstreamPathKey).(string)
			pr.Out.URL.Scheme = target.Scheme
			pr.Out.URL.Host = target.Host
			pr.Out.URL.Path = strings.TrimRight(target.Path, "/") + path
			pr.Out.URL.RawPath = ""
			pr.Out.Host = target.Host
			// CORS is answered by the gateway; the services must not add their own.
			pr.Out.Header.Del("Origin")
			if c, ok := pr.In.Context().Value(clientKey).(client); ok {
				pr.Out.Header.Set("X-Forwarded-For", c.ip)
				pr.Out.Header.Set("X-Real-Ip", c.ip)
				pr.Out.Header.Set("X-Forwarded-Proto", c.proto)
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Del(HeaderRequestID) // the gateway's own id is already set
			for k := range resp.Header {
				if strings.HasPrefix(k, "Access-Control-") {
					resp.Header.Del(k)
				}
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			var tooLarge *http.MaxBytesError
			switch {
			case errors.As(err, &tooLarge):
				apierr.Write(w, apierr.PayloadTooLarge)
			case r.Context().Err() != nil && errors.Is(r.Context().Err(), context.Canceled):
				// The client went away; nobody is listening for an answer.
			case errors.Is(err, context.DeadlineExceeded) || isTimeout(err):
				apierr.Write(w, apierr.UpstreamTimeout)
			default:
				g.log.Warn("upstream unavailable", "upstream", target.Host, "error", err, "request_id", r.Header.Get(HeaderRequestID))
				apierr.Write(w, apierr.Unavailable)
			}
		},
	}
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := g.now()
	rec := &recorder{ResponseWriter: w, status: http.StatusOK}
	reqID := r.Header.Get(HeaderRequestID)
	if !requestIDPattern.MatchString(reqID) {
		reqID = uuid.NewString()
	}
	rec.Header().Set(HeaderRequestID, reqID)

	route, userID := g.serve(rec, r, reqID)

	g.log.Info("request",
		"method", r.Method, "path", r.URL.Path, "route", route, "status", rec.status,
		"duration_ms", g.now().Sub(start).Milliseconds(), "request_id", reqID, "user_id", userID)
}

func (g *Gateway) serve(w http.ResponseWriter, r *http.Request, reqID string) (routeName, userID string) {
	if g.cors.apply(w, r) {
		return "cors-preflight", ""
	}
	if r.URL.Path == "/healthz" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
		return "healthz", ""
	}

	// Who is calling, read before X-Forwarded-* is discarded as spoofable.
	caller := client{ip: clientIP(r, g.cfg.TrustForwardedFor), proto: forwardedProto(r, g.cfg.TrustForwardedFor)}

	// 1. Strip every identity/trust header the client sent, before anything
	// else. X-Complex-ID is the client's choice and is validated below.
	for name := range r.Header {
		for _, s := range spoofable {
			if strings.EqualFold(name, s) || (strings.HasSuffix(s, "-") && strings.HasPrefix(strings.ToLower(name), strings.ToLower(s))) {
				r.Header.Del(name)
				break
			}
		}
	}
	r.Header.Set(HeaderRequestID, reqID)
	r.Header.Set(HeaderCorrelationID, reqID)

	// 2. Route. Unknown, malformed or internal paths stop here.
	if !safePath(r.URL.Path, r.URL.RawPath) {
		apierr.Write(w, apierr.NotFound)
		return "", ""
	}
	route, ok := match(g.routes, r.URL.Path)
	if !ok {
		apierr.Write(w, apierr.NotFound)
		return "", ""
	}
	upstreamPath := route.upstreamPath(r.URL.Path)
	if internalPath(upstreamPath) {
		apierr.Write(w, apierr.NotFound)
		return route.Name, ""
	}

	// 3. Body size.
	if r.ContentLength > g.cfg.MaxBodyBytes {
		apierr.Write(w, apierr.PayloadTooLarge)
		return route.Name, ""
	}
	r.Body = http.MaxBytesReader(w, r.Body, g.cfg.MaxBodyBytes)

	ctx, cancel := context.WithTimeout(r.Context(), g.cfg.UpstreamTimeout)
	defer cancel()
	ctx = context.WithValue(ctx, upstreamPathKey, upstreamPath)
	ctx = context.WithValue(ctx, clientKey, caller)

	// 4a. Public route: rate limit by IP, forward without any identity.
	if route.Public != nil && route.Public(r.Method, r.URL.Path) {
		r.Header.Del(HeaderComplexID)
		if !g.allow(w, r, "ip:"+caller.ip, g.cfg.PublicRatePerMin) {
			return route.Name, ""
		}
		g.proxies[route.Name].ServeHTTP(w, r.WithContext(ctx))
		return route.Name, ""
	}

	// 4b. Authenticated route.
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || strings.TrimSpace(token) == "" {
		apierr.Write(w, apierr.Unauthenticated)
		return route.Name, ""
	}
	id, err := g.verifier.Verify(ctx, strings.TrimSpace(token))
	if err != nil {
		apierr.Write(w, apierr.Unauthenticated)
		return route.Name, ""
	}
	if !g.allow(w, r, "user:"+id.UserID, g.cfg.UserRatePerMin) {
		return route.Name, id.UserID
	}

	// 5. Complex: only a complex the user really belongs to is forwarded,
	// always together with the role resolved from the memberships.
	complexID := ""
	if raw := strings.TrimSpace(r.Header.Get(HeaderComplexID)); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			apierr.Write(w, apierr.BadComplexID)
			return route.Name, id.UserID
		}
		complexID = parsed.String()
		role, member, err := g.roles.Role(ctx, id.UserID, complexID)
		switch {
		case err != nil:
			g.log.Warn("role lookup failed", "error", err, "request_id", reqID)
			apierr.Write(w, apierr.Unavailable)
			return route.Name, id.UserID
		case !member:
			apierr.Write(w, apierr.NotMember)
			return route.Name, id.UserID
		}
		r.Header.Set(HeaderComplexID, complexID)
		r.Header.Set(HeaderComplexRole, role)
	}

	// 6. Trusted identity for the microservice.
	r.Header.Set(HeaderUserID, id.UserID)
	if route.Upstream.Secret != "" {
		r.Header.Set(HeaderGatewaySecret, route.Upstream.Secret)
	}
	if route.SignTenant && complexID != "" && g.cfg.AuthTenantSecret != "" {
		ts := strconv.FormatInt(g.now().Unix(), 10)
		r.Header.Set(HeaderTenantID, complexID)
		r.Header.Set(HeaderTenantTimestamp, ts)
		r.Header.Set(HeaderTenantSignature, SignTenant(g.cfg.AuthTenantSecret, id.UserID, complexID, ts))
	}

	g.proxies[route.Name].ServeHTTP(w, r.WithContext(ctx))
	return route.Name, id.UserID
}

// SignTenant is the signature the auth service verifies:
// hex(HMAC-SHA256(secret, userId + "\n" + tenantId + "\n" + unixSeconds)).
func SignTenant(secret, userID, tenantID, timestamp string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(userID + "\n" + tenantID + "\n" + timestamp))
	return hex.EncodeToString(mac.Sum(nil))
}

// allow applies a per-minute limit. If the limiter backend fails the request
// is let through (availability over strictness) and the failure is logged.
func (g *Gateway) allow(w http.ResponseWriter, r *http.Request, key string, perMin int) bool {
	ok, retry, err := g.limiter.Allow(r.Context(), key, perMin, time.Minute)
	if err != nil {
		g.log.Warn("rate limiter unavailable, allowing request", "error", err)
		return true
	}
	if !ok {
		apierr.WriteRetryAfter(w, retry)
		return false
	}
	return true
}

// clientIP is the peer address, or the rightmost X-Forwarded-For entry (the
// one appended by our own proxy) when the gateway sits behind a trusted proxy.
func clientIP(r *http.Request, trustForwarded bool) string {
	if trustForwarded {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if ip := strings.TrimSpace(parts[len(parts)-1]); net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func forwardedProto(r *http.Request, trustForwarded bool) string {
	if trustForwarded {
		if p := r.Header.Get("X-Forwarded-Proto"); p == "http" || p == "https" {
			return p
		}
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

// recorder captures the status for the access log. Unwrap keeps Flush and
// friends reachable through http.ResponseController (used by ReverseProxy).
type recorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *recorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status, r.wroteHeader = code, true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(b []byte) (int, error) {
	r.wroteHeader = true
	return r.ResponseWriter.Write(b)
}

func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
