package gateway

import (
	"net/http"
	"strings"

	"superapp/api-gateway/internal/config"
)

// Route maps a public path prefix to a microservice. Matching is by whole
// path segments: /api/v1/access matches /api/v1/access/qr-codes but not
// /api/v1/accessories.
type Route struct {
	Name     string
	Prefix   string
	Upstream config.Upstream
	// Strip is removed from the path before forwarding ("" = forward as is).
	Strip string
	// Public reports whether method+path skips authentication.
	Public func(method, path string) bool
	// SignTenant sends the validated complex to auth as a signed X-Tenant-ID.
	SignTenant bool
}

func (r Route) matches(path string) bool {
	return path == r.Prefix || strings.HasPrefix(path, r.Prefix+"/")
}

// upstreamPath is the path the microservice receives.
func (r Route) upstreamPath(path string) string {
	if r.Strip == "" {
		return path
	}
	p := strings.TrimPrefix(path, r.Strip)
	if p == "" {
		p = "/"
	}
	return p
}

// authPublic are the auth endpoints that work without an access token: they
// authenticate with credentials or a refresh token in the body.
var authPublic = map[string]bool{
	"/api/v1/auth/login":           true,
	"/api/v1/auth/refresh":         true,
	"/api/v1/auth/logout":          true,
	"/api/v1/auth/register":        true,
	"/api/v1/auth/forgot-password": true,
	"/api/v1/auth/reset-password":  true,
}

// BuildRoutes is the routing table. Each microservice keeps its own paths:
//
//   - auth and control-de-accesos already serve under /api/v1, so the path
//     is forwarded unchanged.
//   - notificaciones serves at the root; the front reaches it through
//     /api/v1/notifications/{proxy+} and the prefix is stripped (same
//     contract as the AWS API Gateway stage the front already uses).
//
// Anything not listed is a 404 from the gateway: unknown paths never reach a
// microservice.
func BuildRoutes(cfg config.Config) []Route {
	auth := func(prefix string) Route {
		return Route{Name: "auth", Prefix: prefix, Upstream: cfg.Auth, SignTenant: true}
	}
	ca := func(prefix string) Route {
		return Route{Name: "control-de-accesos", Prefix: prefix, Upstream: cfg.ControlDeAccesos}
	}

	authRoute := auth("/api/v1/auth")
	authRoute.Public = func(method, path string) bool {
		return method == http.MethodPost && authPublic[strings.TrimRight(path, "/")]
	}

	routes := []Route{
		authRoute,
		auth("/api/v1/users"),
		auth("/api/v1/roles"),
		auth("/api/v1/permissions"),
		auth("/api/v1/sessions"),
		{
			Name: "notificaciones", Prefix: "/api/v1/notifications",
			Upstream: cfg.Notificaciones, Strip: "/api/v1/notifications",
		},
		ca("/api/v1/access"),
		ca("/api/v1/access-grants"),
		ca("/api/v1/access-logs"),
		ca("/api/v1/resident-movements"),
		ca("/api/v1/units"),
		ca("/api/v1/me"),
	}
	if cfg.ExposeDevRoutes {
		dev := ca("/api/v1/dev")
		// /dev/users and /dev/token are how the dev login obtains a token;
		// the rest of /dev still requires one.
		dev.Public = func(method, path string) bool {
			p := strings.TrimRight(path, "/")
			return method == http.MethodGet && (p == "/api/v1/dev/users" || p == "/api/v1/dev/token")
		}
		routes = append(routes, dev)
	}
	return routes
}

func match(routes []Route, path string) (Route, bool) {
	for _, r := range routes {
		if r.matches(path) {
			return r, true
		}
	}
	return Route{}, false
}

// safePath rejects paths a microservice could interpret differently from the
// gateway: dot segments, empty segments, backslashes and encoded separators
// or dots (which would let /api/v1/notifications/%2e%2e/internal/... slip
// past the route table).
func safePath(path, rawPath string) bool {
	if !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "\\\x00") || strings.Contains(path, "//") {
		return false
	}
	if rawPath != "" {
		lower := strings.ToLower(rawPath)
		for _, enc := range []string{"%2f", "%5c", "%2e", "%00"} {
			if strings.Contains(lower, enc) {
				return false
			}
		}
	}
	for _, seg := range strings.Split(path, "/") {
		if seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

// internalPath reports whether the upstream path targets service-to-service
// endpoints, which the gateway never forwards.
func internalPath(upstreamPath string) bool {
	for _, seg := range strings.Split(upstreamPath, "/") {
		if strings.EqualFold(seg, "internal") {
			return true
		}
	}
	return false
}
