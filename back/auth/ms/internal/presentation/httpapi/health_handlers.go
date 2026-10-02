package httpapi

import "net/http"

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (a *API) ready(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		methodNotAllowed(w)
		return
	}
	if err := a.service.Ready(r.Context()); err != nil {
		writeError(w, 503, "not_ready", "Auth dependencies unavailable")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ready"})
}

func (a *API) jwks(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		methodNotAllowed(w)
		return
	}
	if jwkProvider, ok := a.service.(interface{ JWKS() map[string]any }); ok {
		w.Header().Set("Cache-Control", "public, max-age=300")
		writeJSON(w, 200, jwkProvider.JWKS())
		return
	}
	writeError(w, 500, "jwks_unavailable", "Signing keys unavailable")
}
