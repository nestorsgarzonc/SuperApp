// Package cors deja que cualquier front (GitHub Pages, localhost, S3) llame al
// ms desde el navegador: el gateway reenvía todo tal cual con proxy
// integration, así que los headers de CORS los pone el ms. Se permite `*` sin
// credentials: la API se autentica con el header Authorization (Bearer), no con
// cookies, así que otra página no puede reutilizar la sesión del usuario.
package cors

import "net/http"

const (
	allowMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
	allowHeaders = "Authorization, Content-Type, X-Complex-ID"
)

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") == "" {
			next.ServeHTTP(w, r)
			return
		}
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			h.Set("Access-Control-Allow-Methods", allowMethods)
			h.Set("Access-Control-Allow-Headers", allowHeaders)
			h.Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
