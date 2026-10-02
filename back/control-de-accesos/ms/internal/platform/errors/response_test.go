package errors

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func decode(t *testing.T, body string) (error, int) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	var dst struct {
		Name string `json:"name"`
	}
	err := DecodeJSONLenient(rec, req, &dst)
	if err != nil {
		return err, Wrap(err).HTTPStatus()
	}
	return nil, 0
}

func TestOversizedBodyIsRejectedWith413(t *testing.T) {
	big := `{"name":"` + strings.Repeat("x", int(MaxRequestBodyBytes)+10) + `"}`
	err, status := decode(t, big)
	if err == nil || status != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413, got status=%d err=%v", status, err)
	}
}

func TestNormalAndMalformedBodies(t *testing.T) {
	if err, _ := decode(t, `{"name":"Ana"}`); err != nil {
		t.Fatalf("a normal body must decode, got %v", err)
	}
	if err, status := decode(t, `{"name":`); err == nil || status != http.StatusBadRequest {
		t.Fatalf("malformed JSON must be 400, got status=%d err=%v", status, err)
	}
}
