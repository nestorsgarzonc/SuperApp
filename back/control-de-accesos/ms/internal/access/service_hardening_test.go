package access

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"controldeaccesos/internal/qrsystem"

	"github.com/google/uuid"
)

// ── fakes ────────────────────────────────────────────────────────────────────

type fakeRepo struct {
	Repository // lo no usado en estas pruebas provoca panic: así se nota

	mu            sync.Mutex
	txCalls       int
	txReqs        [][]CreateAccessGrantRequest
	singleCreates int
	forUserCalls  []forUserCall
	complexCalls  int
}

type forUserCall struct {
	complexID, userID uuid.UUID
	method            string
}

func (f *fakeRepo) CreateAccessGrant(_ context.Context, _ uuid.UUID, _ CreateAccessGrantRequest, _ uuid.UUID) (*AccessGrant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.singleCreates++
	return &AccessGrant{ID: uuid.New()}, nil
}

func (f *fakeRepo) CreateAccessGrantsTx(_ context.Context, _, _ uuid.UUID, reqs []CreateAccessGrantRequest) ([]*AccessGrant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.txCalls++
	f.txReqs = append(f.txReqs, reqs)
	out := make([]*AccessGrant, len(reqs))
	for i := range reqs {
		out[i] = &AccessGrant{ID: uuid.New(), QRCodeHash: &reqs[i].qrCode}
	}
	return out, nil
}

func (f *fakeRepo) GetComplexAccessGrantsForUser(_ context.Context, complexID, userID uuid.UUID, method string) ([]*AccessGrant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forUserCalls = append(f.forUserCalls, forUserCall{complexID, userID, method})
	return nil, nil
}

func (f *fakeRepo) GetComplexAccessGrants(context.Context, uuid.UUID, *string, *string) ([]*AccessGrant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.complexCalls++
	return nil, nil
}

type fakeDirectory struct{ complexID uuid.UUID }

func (d fakeDirectory) GetUnitComplexID(context.Context, uuid.UUID) (uuid.UUID, bool, error) {
	return d.complexID, true, nil
}
func (fakeDirectory) GetUnitName(context.Context, uuid.UUID) (string, error) { return "A-101", nil }
func (fakeDirectory) GetUserDisplayName(context.Context, uuid.UUID) (string, error) {
	return "Ana", nil
}

type memberOfEverything struct{}

func (memberOfEverything) IsUserAssociatedWithUnit(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return true, nil
}

// fakeQR succeeds failAfter times and then reports the system unavailable.
type fakeQR struct {
	mu        sync.Mutex
	issued    int
	failAfter int // -1: nunca falla
}

func (q *fakeQR) Issue(context.Context) (string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.failAfter >= 0 && q.issued >= q.failAfter {
		return "", &qrsystem.UnavailableError{Cause: qrsystem.CauseCircuitOpen, RetryAfter: time.Second}
	}
	q.issued++
	return uuid.NewString(), nil
}
func (q *fakeQR) Scan(context.Context) error { return nil }
func (q *fakeQR) Availability() qrsystem.Public {
	return qrsystem.Public{Available: true, State: "CLOSED"}
}

func newTestService(repo Repository, qr QRSystem, complexID uuid.UUID) Service {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewService(repo, fakeDirectory{complexID: complexID}, memberOfEverything{}, nil, nil, qr, logger)
}

func batchReq(unit uuid.UUID, n int) CreateQRBatchRequest {
	return CreateQRBatchRequest{
		UnitID: unit, Quantity: n,
		ValidFrom:  time.Now(),
		ValidUntil: time.Now().Add(2 * time.Hour),
	}
}

// ── C5: el lote es todo-o-nada ───────────────────────────────────────────────

func TestQRBatchWritesNothingWhenTheQRSystemFailsMidway(t *testing.T) {
	complexID, user, unit := uuid.New(), uuid.New(), uuid.New()
	repo := &fakeRepo{}
	qr := &fakeQR{failAfter: 3} // emite 3 códigos y el 4.º falla
	svc := newTestService(repo, qr, complexID)

	_, err := svc.CreateQRBatch(context.Background(), complexID, user, batchReq(unit, 10))

	if !errors.Is(err, qrsystem.ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
	if repo.txCalls != 0 || repo.singleCreates != 0 {
		t.Fatalf("a failed batch must write NOTHING (tx=%d, single=%d)", repo.txCalls, repo.singleCreates)
	}
}

func TestQRBatchInsertsEverythingInOneTransaction(t *testing.T) {
	complexID, user, unit := uuid.New(), uuid.New(), uuid.New()
	repo := &fakeRepo{}
	svc := newTestService(repo, &fakeQR{failAfter: -1}, complexID)

	grants, err := svc.CreateQRBatch(context.Background(), complexID, user, batchReq(unit, 25))
	if err != nil || len(grants) != 25 {
		t.Fatalf("want 25 grants, got %d (err=%v)", len(grants), err)
	}
	if repo.txCalls != 1 || repo.singleCreates != 0 {
		t.Fatalf("all inserts must go through ONE transaction (tx=%d, single=%d)", repo.txCalls, repo.singleCreates)
	}
	seen := map[string]bool{}
	for _, r := range repo.txReqs[0] {
		if r.qrCode == "" || seen[r.qrCode] {
			t.Fatalf("every grant needs its own code issued by the QR system, got %q", r.qrCode)
		}
		seen[r.qrCode] = true
	}
}

// ── C1: nadie lista los pases de unidades ajenas ─────────────────────────────

func TestListingQRAndPreAuthIsScopedToTheCallersUnits(t *testing.T) {
	complexID, user := uuid.New(), uuid.New()
	repo := &fakeRepo{}
	svc := newTestService(repo, &fakeQR{failAfter: -1}, complexID)

	if _, err := svc.GetQRCodes(context.Background(), complexID, user); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetPreAuthorizations(context.Background(), complexID, user); err != nil {
		t.Fatal(err)
	}

	if repo.complexCalls != 0 {
		t.Fatal("listing must never fall back to the complex-wide query (it exposed neighbours' QR codes)")
	}
	if len(repo.forUserCalls) != 2 {
		t.Fatalf("want 2 user-scoped queries, got %d", len(repo.forUserCalls))
	}
	for _, c := range repo.forUserCalls {
		if c.userID != user || c.complexID != complexID {
			t.Fatalf("query must be scoped to the caller and their complex, got %+v", c)
		}
	}
	if repo.forUserCalls[0].method != MethodQR || repo.forUserCalls[1].method != MethodPreAuth {
		t.Fatalf("wrong methods: %+v", repo.forUserCalls)
	}
}
