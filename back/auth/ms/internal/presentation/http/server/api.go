package httpapi

import (
	"net/http"
	"superapp/auth/internal/presentation/http/reader"
	shared "superapp/auth/internal/presentation/http/shared"
	"superapp/auth/internal/presentation/http/writer"
)

type API struct {
	mux    *http.ServeMux
	reader *reader.Routes
	writer *writer.Routes
}

func New(service shared.Service, secret []byte) *API {
	common := shared.New(service, secret)
	a := &API{mux: http.NewServeMux(), reader: reader.New(common), writer: writer.New(common)}
	a.mux.HandleFunc("/", a.route)
	return a
}
func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) { a.mux.ServeHTTP(w, r) }
func (a *API) route(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/v1/" {
		shared.WriteError(w, http.StatusNotFound, "not_found", "Route not found")
		return
	}
	if a.reader.Handle(w, r) {
		return
	}
	if a.writer.Handle(w, r) {
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost && r.Method != http.MethodPatch && r.Method != http.MethodPut && r.Method != http.MethodDelete {
		shared.MethodNotAllowed(w)
		return
	}
	shared.WriteError(w, http.StatusNotFound, "not_found", "Route not found")
}
