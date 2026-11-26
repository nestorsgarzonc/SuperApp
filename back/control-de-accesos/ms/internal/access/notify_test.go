package access

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"controldeaccesos/internal/notifier"
)

type chanNotifier struct{ events chan notifier.Event }

func (n chanNotifier) Notify(_ context.Context, e notifier.Event) { n.events <- e }

type membersDirectory struct {
	fakeDirectory
	members []uuid.UUID
}

func (d membersDirectory) GetUnitMemberIDs(context.Context, uuid.UUID) ([]uuid.UUID, error) {
	return d.members, nil
}

func newNotifyService(members []uuid.UUID, complexID uuid.UUID) (*service, chanNotifier) {
	n := chanNotifier{events: make(chan notifier.Event, 4)}
	svc := &service{
		directory: membersDirectory{fakeDirectory{complexID: complexID}, members},
		notifier:  n,
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	return svc, n
}

func waitEvent(t *testing.T, n chanNotifier) notifier.Event {
	t.Helper()
	select {
	case e := <-n.events:
		return e
	case <-time.After(2 * time.Second):
		t.Fatal("expected a notification, got none")
		return notifier.Event{}
	}
}

// Caso GS-E4 · notify excluye al actor
// Técnica: Partición en clases de equivalencia.
// Entrada: —
// Debe hacer / esperado: solo el vecino
func TestGSE4_NotifyAddressesTheUnitResidentsExceptTheActor(t *testing.T) {
	complexID, unitID := uuid.New(), uuid.New()
	actor, neighbour := uuid.New(), uuid.New()
	svc, n := newNotifyService([]uuid.UUID{actor, neighbour}, complexID)
	grant := &AccessGrant{ID: uuid.New(), ComplexID: complexID, UnitID: unitID, VisitorName: "Juan", Method: MethodQR}

	svc.notify(context.Background(), eventGrantCreated, grant, &actor)
	e := waitEvent(t, n)

	if len(e.UserIDs) != 1 || e.UserIDs[0] != neighbour {
		t.Fatalf("want only the neighbour as recipient, got %v", e.UserIDs)
	}
	if e.ComplexID == nil || *e.ComplexID != complexID {
		t.Fatalf("complex_id not set: %v", e.ComplexID)
	}
	if e.Category != notifier.CategoryAccess || e.Title == "" || !strings.Contains(e.Body, "Juan") {
		t.Fatalf("unexpected content: %+v", e)
	}
	if e.Data["grant_id"] != grant.ID.String() {
		t.Fatalf("grant_id missing from data: %v", e.Data)
	}
}

// Caso GS-E5 · notify de validación
// Técnica: Partición en clases de equivalencia.
// Entrada: —
// Debe hacer / esperado: categoría VISITOR, 2 destinatarios
func TestGSE5_NotifyValidatedUsesTheVisitorCategoryForEveryResident(t *testing.T) {
	complexID := uuid.New()
	a, b := uuid.New(), uuid.New()
	svc, n := newNotifyService([]uuid.UUID{a, b}, complexID)
	grant := &AccessGrant{ID: uuid.New(), ComplexID: complexID, UnitID: uuid.New(), VisitorName: "Juan"}

	svc.notify(context.Background(), eventGrantValidated, grant, nil)
	e := waitEvent(t, n)

	if e.Category != notifier.CategoryVisitor || len(e.UserIDs) != 2 {
		t.Fatalf("want VISITOR to both residents, got %+v", e)
	}
}

// Caso GS-E6 · notify sin destinatarios
// Técnica: Partición en clases de equivalencia.
// Entrada: —
// Debe hacer / esperado: no se envía
func TestGSE6_NotifySkipsWhenThereIsNobodyToTell(t *testing.T) {
	complexID := uuid.New()
	actor := uuid.New()
	svc, n := newNotifyService([]uuid.UUID{actor}, complexID)
	grant := &AccessGrant{ID: uuid.New(), ComplexID: complexID, UnitID: uuid.New()}

	svc.notify(context.Background(), eventGrantCancelled, grant, &actor)

	select {
	case e := <-n.events:
		t.Fatalf("no notification expected, got %+v", e)
	case <-time.After(200 * time.Millisecond):
	}
}
