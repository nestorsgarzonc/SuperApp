package notifications

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"superapp/notificaciones/internal/platform/apperr"
	"superapp/notificaciones/internal/platform/gatewayauth"
)

type fakeService struct {
	Service

	gotComplex, gotUser uuid.UUID
	gotLimit            int
	gotUnread           bool
	gotGroups           []string
	err                 error
}

func (f *fakeService) GetUserNotifications(_ context.Context, complexID, userID uuid.UUID, q InboxQuery) ([]*Notification, error) {
	f.gotComplex, f.gotUser, f.gotUnread, f.gotLimit, f.gotGroups = complexID, userID, q.UnreadOnly, q.Limit, q.Groups
	return []*Notification{}, f.err
}

func (f *fakeService) MarkAsRead(_ context.Context, _, complexID, userID uuid.UUID) error {
	f.gotComplex, f.gotUser = complexID, userID
	return f.err
}

func newRouter(svc Service) http.Handler {
	r := chi.NewRouter()
	r.Use(gatewayauth.Middleware(""))
	RegisterRoutes(r, NewHandler(svc, slog.New(slog.DiscardHandler)))
	return r
}

type caller struct {
	user, complex uuid.UUID
}

func (c caller) do(t *testing.T, h http.Handler, method, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(gatewayauth.HeaderUserID, c.user.String())
	if c.complex != uuid.Nil {
		req.Header.Set(gatewayauth.HeaderComplexID, c.complex.String())
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	if rec.Body.Len() > 0 {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	}
	return rec, out
}

func TestHandlers_InboxUsesGatewayIdentity(t *testing.T) {
	svc := &fakeService{}
	c := caller{user: uuid.New(), complex: uuid.New()}

	rec, body := c.do(t, newRouter(svc), http.MethodGet, "/users/me/notifications?unread=true&limit=10&group=ASSEMBLIES,VOTES", "")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, true, body["success"])
	assert.Equal(t, c.user, svc.gotUser)
	assert.Equal(t, c.complex, svc.gotComplex)
	assert.True(t, svc.gotUnread)
	assert.Equal(t, 10, svc.gotLimit)
	assert.Equal(t, []string{"ASSEMBLIES", "VOTES"}, svc.gotGroups)
}

func TestHandlers_InboxRequiresComplex(t *testing.T) {
	rec, _ := caller{user: uuid.New()}.do(t, newRouter(&fakeService{}), http.MethodGet, "/users/me/notifications", "")
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestHandlers_MarkAsRead(t *testing.T) {
	c := caller{user: uuid.New(), complex: uuid.New()}

	rec, _ := c.do(t, newRouter(&fakeService{}), http.MethodPatch, "/notifications/not-a-uuid/read", "")
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	rec, _ = c.do(t, newRouter(&fakeService{}), http.MethodPatch, "/notifications/"+uuid.NewString()+"/read", "")
	assert.Equal(t, http.StatusNoContent, rec.Code)

	missing := &fakeService{err: apperr.NotFound("Notification", "x")}
	rec, _ = c.do(t, newRouter(missing), http.MethodPatch, "/notifications/"+uuid.NewString()+"/read", "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandlers_InternalErrorsDoNotLeakCause(t *testing.T) {
	svc := &fakeService{err: apperr.Internal(assertErr("pq: relation \"notifications\" does not exist"))}
	c := caller{user: uuid.New(), complex: uuid.New()}

	rec, _ := c.do(t, newRouter(svc), http.MethodGet, "/users/me/notifications", "")

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.NotContains(t, rec.Body.String(), "relation")
}

type assertErr string

func (e assertErr) Error() string { return string(e) }

type dispatchService struct {
	Service
	got DispatchInput
	err error
}

func (d *dispatchService) Dispatch(_ context.Context, in DispatchInput) error {
	d.got = in
	return d.err
}

func postInternal(t *testing.T, svc Service, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	RegisterInternalRoutes(r, NewHandler(svc, slog.New(slog.DiscardHandler)))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/internal/notifications", strings.NewReader(body)))
	return rec
}

func TestDispatch_AcceptsEvent(t *testing.T) {
	svc := &dispatchService{}
	user := uuid.New()
	rec := postInternal(t, svc, `{"event_id":"evt-1","source":"control-de-accesos","user_ids":["`+user.String()+`"],"category":"VISITOR","title":"t","body":"b"}`)

	require.Equal(t, http.StatusAccepted, rec.Code)
	assert.Equal(t, "evt-1", svc.got.EventID)
	assert.Equal(t, []uuid.UUID{user}, svc.got.UserIDs)
	assert.Equal(t, "VISITOR", svc.got.Category)
}

func TestDispatch_Errors(t *testing.T) {
	assert.Equal(t, http.StatusUnprocessableEntity, postInternal(t, &dispatchService{}, `{not json`).Code)
	assert.Equal(t, http.StatusUnprocessableEntity,
		postInternal(t, &dispatchService{err: apperr.Field("category", "Unknown category")}, `{}`).Code)
}
