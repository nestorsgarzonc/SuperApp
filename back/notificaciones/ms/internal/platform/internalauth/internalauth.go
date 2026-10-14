// Package internalauth protege las rutas /internal/*: solo las llaman otros
// servicios de la red privada, con el secreto compartido INTERNAL_SECRET
// (contrato: docs/internal-apis.md). El gateway nunca las enruta.
package internalauth

import (
	"crypto/subtle"
	"net/http"

	"superapp/notificaciones/internal/platform/apperr"
)

const HeaderSecret = "X-Internal-Secret"

// Middleware rechaza con 401 las peticiones sin el secreto. Un secreto vacío
// (solo permitido con ENV=development) deja pasar todo.
func Middleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if secret != "" && subtle.ConstantTimeCompare([]byte(r.Header.Get(HeaderSecret)), []byte(secret)) != 1 {
				apperr.RespondError(w, nil, apperr.Auth("Internal secret required"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
