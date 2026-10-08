package writer

import (
	"net/http"
	"superapp/auth/internal/presentation/reader"
)

func (a *Handlers) logout(w http.ResponseWriter, r *http.Request) {
	request, ok := reader.LogoutRequest(w, r)
	if !ok {
		return
	}
	if len(request.RefreshToken) > 512 {
		w.WriteHeader(204)
		return
	}
	_, err := a.Logout.Execute(r.Context(), request)
	a.Respond(w, err, 204, nil)
}
