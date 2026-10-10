package notifier

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
)

var logger = slog.New(slog.DiscardHandler)

func TestNotify_SendsEventWithSecret(t *testing.T) {
	var got Event
	var path, secret string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, secret = r.URL.Path, r.Header.Get("X-Internal-Secret")
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	c := New(srv.URL+"/", "s3cret", logger)
	user := uuid.New()
	c.Notify(context.Background(), Event{UserIDs: []uuid.UUID{user}, Category: CategoryVisitor, Title: "t", Body: "b"})
	_ = c.Close()

	if path != "/internal/notifications" || secret != "s3cret" {
		t.Fatalf("path=%q secret=%q", path, secret)
	}
	if got.Source != Source || got.EventID == "" || len(got.UserIDs) != 1 || got.UserIDs[0] != user {
		t.Fatalf("unexpected event: %+v", got)
	}
}

func TestNotify_RetriesServerErrorsWithSameEventID(t *testing.T) {
	var calls atomic.Int32
	ids := make(chan string, maxAttempts)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var e Event
		_ = json.NewDecoder(r.Body).Decode(&e)
		ids <- e.EventID
		if calls.Add(1) < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	c := New(srv.URL, "", logger)
	c.Notify(context.Background(), Event{UserIDs: []uuid.UUID{uuid.New()}, Category: CategoryAccess, Title: "t", Body: "b"})
	_ = c.Close()

	if n := calls.Load(); n != 2 {
		t.Fatalf("calls = %d, want 2", n)
	}
	if a, b := <-ids, <-ids; a != b {
		t.Fatalf("retry changed event_id: %s != %s", a, b)
	}
}

func TestNotify_DoesNotRetryRejectedEvents(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))
	defer srv.Close()

	c := New(srv.URL, "", logger)
	c.Notify(context.Background(), Event{UserIDs: []uuid.UUID{uuid.New()}, Category: "NOPE", Title: "t", Body: "b"})
	_ = c.Close()
	if n := calls.Load(); n != 1 {
		t.Fatalf("calls = %d, want 1", n)
	}
}

func TestNotify_NoopWithoutURLOrRecipients(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer srv.Close()

	New("", "", logger).Notify(context.Background(), Event{UserIDs: []uuid.UUID{uuid.New()}})
	c := New(srv.URL, "", logger)
	c.Notify(context.Background(), Event{})
	_ = c.Close()
	if n := calls.Load(); n != 0 {
		t.Fatalf("calls = %d, want 0", n)
	}
}
