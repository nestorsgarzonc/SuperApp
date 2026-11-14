package gateway

import (
	"net/http"
	"strings"
)

const (
	corsMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
	corsHeaders = "Authorization, Content-Type, X-Complex-ID, X-Request-ID"
)

// corsPolicy answers CORS for every service in one place. The API
// authenticates with a Bearer header, not cookies, so credentials are never
// allowed and another site cannot ride the user's session.
type corsPolicy struct {
	any     bool
	origins map[string]bool
}

func newCORS(allowed []string) corsPolicy {
	p := corsPolicy{origins: map[string]bool{}}
	for _, o := range allowed {
		if o == "*" {
			p.any = true
		}
		p.origins[o] = true
	}
	return p
}

// apply sets CORS headers and reports whether it fully answered a preflight.
func (p corsPolicy) apply(w http.ResponseWriter, r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	h := w.Header()
	h.Add("Vary", "Origin")
	allowed := p.any || p.origins[strings.TrimRight(origin, "/")]
	preflight := r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != ""
	if allowed {
		if p.any {
			h.Set("Access-Control-Allow-Origin", "*")
		} else {
			h.Set("Access-Control-Allow-Origin", origin)
		}
		h.Set("Access-Control-Expose-Headers", "X-Request-ID, Retry-After")
	}
	if !preflight {
		return false
	}
	if allowed {
		h.Set("Access-Control-Allow-Methods", corsMethods)
		h.Set("Access-Control-Allow-Headers", corsHeaders)
		h.Set("Access-Control-Max-Age", "600")
	}
	// A disallowed origin gets a 204 without Allow-* headers: the browser blocks it.
	w.WriteHeader(http.StatusNoContent)
	return true
}
