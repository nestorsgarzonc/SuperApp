package access

// Critical-flow component tests (Project Delivery 1 — component-level verification).
//
// Components under test, each in isolation:
//   - QR generation:   CreateQRCode / CreateQRBatchRequest / CreateQRCodeRequest validation
//   - Access control:  checkUnitOwnership guards (via CreateQRCode / CancelAccessGrant)
//   - QR validation:   ValidateAccess (grant state machine ACTIVE → USED)
//
// Every dependency (repository, directory, membership checker, QR system,
// auditor) is a test double declared below. The clock is controlled by using
// windows far from "now" (hours), so no assertion depends on scheduling.
// Each test name carries its test-case ID (TC-QR-xx / TC-AC-xx).

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image/png"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"controldeaccesos/internal/audit"
	authmw "controldeaccesos/internal/platform/middleware"
	"controldeaccesos/internal/qrsystem"

	"github.com/google/uuid"
)

// ── test doubles ─────────────────────────────────────────────────────────────

// grantStore is a FAKE repository: an in-memory map that implements the
// conditional "ACTIVE → USED" update the same way the SQL does (WHERE status = ACTIVE).
type grantStore struct {
	Repository // any method not overridden panics: an unexpected call is visible

	mu        sync.Mutex
	grants    map[uuid.UUID]*AccessGrant
	creates   int
	cancelled []uuid.UUID
}

func newGrantStore(gs ...*AccessGrant) *grantStore {
	s := &grantStore{grants: map[uuid.UUID]*AccessGrant{}}
	for _, g := range gs {
		s.grants[g.ID] = g
	}
	return s
}

func (s *grantStore) CreateAccessGrant(_ context.Context, complexID uuid.UUID, req CreateAccessGrantRequest, createdBy uuid.UUID) (*AccessGrant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creates++
	code := req.qrCode
	g := &AccessGrant{ID: uuid.New(), ComplexID: complexID, UnitID: req.UnitID, CreatedBy: createdBy,
		Method: req.Method, VisitorType: req.VisitorType, VisitorName: req.VisitorName,
		QRCodeHash: &code, Status: StatusActive, ValidFrom: req.ValidFrom, ValidUntil: &req.ValidUntil, SingleUse: true}
	s.grants[g.ID] = g
	return g, nil
}

func (s *grantStore) GetAccessGrant(_ context.Context, id uuid.UUID) (*AccessGrant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if g, ok := s.grants[id]; ok {
		return g, nil
	}
	return nil, ErrAccessGrantNotFound
}

func (s *grantStore) GetAccessGrantByQRHash(_ context.Context, code string) (*AccessGrant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, g := range s.grants {
		if g.QRCodeHash != nil && *g.QRCodeHash == code {
			return g, nil
		}
	}
	return nil, ErrAccessGrantNotFound
}

func (s *grantStore) CancelAccessGrant(_ context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancelled = append(s.cancelled, id)
	return nil
}

func (s *grantStore) TryMarkGrantUsed(_ context.Context, id, usedBy uuid.UUID) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.grants[id]
	if !ok || g.Status != StatusActive {
		return false, nil
	}
	g.Status = StatusUsed
	g.UsedBy = &usedBy
	return true, nil
}

// stubDirectory is a STUB: canned answers about where a unit lives.
type stubDirectory struct {
	unitComplex map[uuid.UUID]uuid.UUID // unit → complex; missing = unit does not exist
}

func (d stubDirectory) GetUnitComplexID(_ context.Context, unit uuid.UUID) (uuid.UUID, bool, error) {
	c, ok := d.unitComplex[unit]
	return c, ok, nil
}
func (stubDirectory) GetUnitName(context.Context, uuid.UUID) (string, error) { return "A-101", nil }
func (stubDirectory) GetUserDisplayName(context.Context, uuid.UUID) (string, error) {
	return "Ana", nil
}
func (stubDirectory) GetUnitMemberIDs(context.Context, uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}

// stubMembership is a STUB of the unit-membership checker.
type stubMembership struct{ member bool }

func (m stubMembership) IsUserAssociatedWithUnit(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return m.member, nil
}

// spyAuditor is a SPY: it records every audit entry for later assertions.
type spyAuditor struct {
	mu      sync.Mutex
	entries []audit.Entry
}

func (a *spyAuditor) Write(_ context.Context, e audit.Entry) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = append(a.entries, e)
	return nil
}

func (a *spyAuditor) last() audit.Entry {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.entries) == 0 {
		return audit.Entry{}
	}
	return a.entries[len(a.entries)-1]
}

// stubQR is a STUB of the QR system: a fixed code, or "unavailable".
type stubQR struct {
	code string
	down bool
}

func (q stubQR) Issue(context.Context) (string, error) {
	if q.down {
		return "", &qrsystem.UnavailableError{Cause: qrsystem.CauseCircuitOpen, RetryAfter: time.Second}
	}
	return q.code, nil
}
func (q stubQR) Scan(context.Context) error {
	if q.down {
		return &qrsystem.UnavailableError{Cause: qrsystem.CauseCircuitOpen, RetryAfter: time.Second}
	}
	return nil
}
func (stubQR) Availability() qrsystem.Public { return qrsystem.Public{Available: true, State: "CLOSED"} }

type fixture struct {
	complexID, otherComplex, unit, user uuid.UUID
	store                               *grantStore
	auditor                             *spyAuditor
}

func newFixture() *fixture {
	return &fixture{complexID: uuid.New(), otherComplex: uuid.New(), unit: uuid.New(), user: uuid.New(),
		store: newGrantStore(), auditor: &spyAuditor{}}
}

func (f *fixture) service(member UnitMembershipChecker, qr QRSystem) Service {
	dir := stubDirectory{unitComplex: map[uuid.UUID]uuid.UUID{f.unit: f.complexID}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewService(f.store, dir, member, nil, f.auditor, qr, logger)
}

// guardCtx is the context the auth middleware leaves for a guard at the gate.
func (f *fixture) guardCtx() context.Context {
	ctx := context.WithValue(context.Background(), authmw.UserIDKey, uuid.NewString())
	return context.WithValue(ctx, authmw.ComplexIDKey, f.complexID.String())
}

func ptr[T any](v T) *T { return &v }

func hoursFromNow(h int) time.Time { return time.Now().Add(time.Duration(h) * time.Hour) }

// ── QR generation: input validation (EP + BVA) ───────────────────────────────

func TestTC_QR_01_04_BatchQuantityBoundaries(t *testing.T) {
	cases := []struct {
		id       string
		quantity int
		valid    bool
	}{
		{"TC-QR-01", 0, false},  // just below the lower bound
		{"TC-QR-02", 1, true},   // lower bound
		{"TC-QR-03", 100, true}, // upper bound
		{"TC-QR-04", 101, false},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			req := CreateQRBatchRequest{UnitID: uuid.New(), Quantity: tc.quantity, ValidFrom: hoursFromNow(1), ValidUntil: hoursFromNow(3)}
			_, hasErr := req.validate()["quantity"]
			if hasErr == tc.valid {
				t.Fatalf("quantity=%d: want valid=%v, got errors=%v", tc.quantity, tc.valid, req.validate())
			}
		})
	}
}

func TestTC_QR_05_VisitorNameLengthBoundary(t *testing.T) {
	for _, tc := range []struct {
		n     int
		valid bool
	}{{200, true}, {201, false}} {
		req := CreateQRCodeRequest{UnitID: uuid.New(), VisitorName: ptr(strings.Repeat("a", tc.n)), ValidUntil: hoursFromNow(2)}
		if _, hasErr := req.validate()["visitor_name"]; hasErr == tc.valid {
			t.Fatalf("visitor_name of %d chars: want valid=%v, got %v", tc.n, tc.valid, req.validate())
		}
	}
}

func TestTC_QR_06_ValidityWindowPartitions(t *testing.T) {
	same := hoursFromNow(2)
	cases := []struct {
		name  string
		from  *time.Time
		until time.Time
		valid bool
	}{
		{"future window", ptr(hoursFromNow(1)), hoursFromNow(3), true},
		{"no valid_from (starts now)", nil, hoursFromNow(1), true},
		{"valid_until in the past", nil, hoursFromNow(-1), false},
		{"valid_from == valid_until (boundary)", &same, same, false},
		{"valid_from after valid_until", ptr(hoursFromNow(3)), hoursFromNow(2), false},
		{"valid_until missing", nil, time.Time{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := CreateQRCodeRequest{UnitID: uuid.New(), ValidFrom: tc.from, ValidUntil: tc.until}
			if _, hasErr := req.validate()["valid_until"]; hasErr == tc.valid {
				t.Fatalf("want valid=%v, got %v", tc.valid, req.validate())
			}
		})
	}
}

// ── QR generation: CreateQRCode ──────────────────────────────────────────────

func TestTC_QR_07_CreateQRCodeIssuesCodeAndRendersPNG(t *testing.T) {
	f := newFixture()
	svc := f.service(stubMembership{member: true}, stubQR{code: "QR-CODE-123"})

	resp, err := svc.CreateQRCode(context.Background(), f.complexID, f.user,
		CreateQRCodeRequest{UnitID: f.unit, ValidUntil: hoursFromNow(2)})
	if err != nil {
		t.Fatal(err)
	}
	if resp.QRCodeHash == nil || *resp.QRCodeHash != "QR-CODE-123" {
		t.Fatalf("the code must come from the QR system, got %v", resp.QRCodeHash)
	}
	if resp.Method != MethodQR || resp.Status != StatusActive || resp.VisitorName != "Guest" {
		t.Fatalf("unexpected grant: method=%s status=%s visitor=%q", resp.Method, resp.Status, resp.VisitorName)
	}
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(resp.QRCodeURL, prefix) {
		t.Fatalf("qr_code_url must be a PNG data URL, got %.40q", resp.QRCodeURL)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(resp.QRCodeURL, prefix))
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("payload is not a PNG: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 256 || b.Dy() != 256 {
		t.Fatalf("want a 256x256 image, got %dx%d", b.Dx(), b.Dy())
	}
	if e := f.auditor.last(); e.Action != ActionCreateGrant || e.Result != audit.ResultAllowed {
		t.Fatalf("creation must be audited as ALLOWED, got %+v", e)
	}
}

func TestTC_QR_08_CreateQRCodeFailsFastWhenQRSystemDown(t *testing.T) {
	f := newFixture()
	svc := f.service(stubMembership{member: true}, stubQR{down: true})

	_, err := svc.CreateQRCode(context.Background(), f.complexID, f.user,
		CreateQRCodeRequest{UnitID: f.unit, ValidUntil: hoursFromNow(2)})
	if !errors.Is(err, qrsystem.ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
	if f.store.creates != 0 {
		t.Fatal("no grant may be written without a code from the QR system")
	}
	if e := f.auditor.last(); e.Result != audit.ResultDenied {
		t.Fatalf("failure must be audited as DENIED, got %+v", e)
	}
}

// ── Access control: unit ownership guards (decision table) ───────────────────
//
//   rule | unit exists | unit in caller's complex | caller member of unit | result
//   R1   |     no      |           -              |          -            | ErrUnitNotFound
//   R2   |     yes     |           no             |          -            | ErrUnitComplexMismatch
//   R3   |     yes     |           yes            |          no           | ErrUnitAccessDenied
//   R4   |     yes     |           yes            |          yes          | grant created

func TestTC_AC_01_04_UnitOwnershipDecisionTable(t *testing.T) {
	cases := []struct {
		id       string
		useUnit  func(f *fixture) uuid.UUID
		complex  func(f *fixture) uuid.UUID
		member   bool
		wantErr  error
		wantSave bool
	}{
		{"TC-AC-01/R1 unknown unit", func(*fixture) uuid.UUID { return uuid.New() }, func(f *fixture) uuid.UUID { return f.complexID }, true, ErrUnitNotFound, false},
		{"TC-AC-02/R2 unit of another complex", func(f *fixture) uuid.UUID { return f.unit }, func(f *fixture) uuid.UUID { return f.otherComplex }, true, ErrUnitComplexMismatch, false},
		{"TC-AC-03/R3 not a member of the unit", func(f *fixture) uuid.UUID { return f.unit }, func(f *fixture) uuid.UUID { return f.complexID }, false, ErrUnitAccessDenied, false},
		{"TC-AC-04/R4 member of the unit", func(f *fixture) uuid.UUID { return f.unit }, func(f *fixture) uuid.UUID { return f.complexID }, true, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			f := newFixture()
			svc := f.service(stubMembership{member: tc.member}, stubQR{code: uuid.NewString()})

			_, err := svc.CreateQRCode(context.Background(), tc.complex(f), f.user,
				CreateQRCodeRequest{UnitID: tc.useUnit(f), ValidUntil: hoursFromNow(2)})

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("want %v, got %v", tc.wantErr, err)
			}
			if saved := f.store.creates == 1; saved != tc.wantSave {
				t.Fatalf("grant written=%v, want %v", saved, tc.wantSave)
			}
			if tc.wantErr != nil && f.auditor.last().Result != audit.ResultDenied {
				t.Fatalf("a refused attempt must be audited as DENIED, got %+v", f.auditor.last())
			}
		})
	}
}

func TestTC_AC_05_NilMembershipCheckerFailsClosed(t *testing.T) {
	f := newFixture()
	svc := f.service(nil, stubQR{code: "x"})

	_, err := svc.CreateQRCode(context.Background(), f.complexID, f.user,
		CreateQRCodeRequest{UnitID: f.unit, ValidUntil: hoursFromNow(2)})
	if !errors.Is(err, ErrUnitAccessDenied) || f.store.creates != 0 {
		t.Fatalf("a missing checker must deny, got err=%v creates=%d", err, f.store.creates)
	}
}

func TestTC_AC_06_CancelGrantOfAnotherComplexIsDenied(t *testing.T) {
	f := newFixture()
	foreign := &AccessGrant{ID: uuid.New(), ComplexID: f.otherComplex, UnitID: f.unit, Status: StatusActive}
	f.store = newGrantStore(foreign)
	svc := f.service(stubMembership{member: true}, stubQR{code: "x"})

	err := svc.CancelAccessGrant(context.Background(), f.complexID, f.user, foreign.ID)
	if !errors.Is(err, ErrUnitAccessDenied) {
		t.Fatalf("want ErrUnitAccessDenied, got %v", err)
	}
	if len(f.store.cancelled) != 0 {
		t.Fatal("a cross-tenant cancel must not reach the repository")
	}
	if e := f.auditor.last(); e.Result != audit.ResultDenied || e.Reason != "cross-tenant access attempt" {
		t.Fatalf("want DENIED cross-tenant audit, got %+v", e)
	}
}

// ── QR validation: grant state machine ───────────────────────────────────────
//
//   ACTIVE --validate (window open)--> USED --validate--> (stays USED, ALREADY_USED)
//   CANCELLED / EXPIRED --validate--> refused, state unchanged

func TestTC_QR_09_SingleUseTransitionActiveToUsed(t *testing.T) {
	f := newFixture()
	g := &AccessGrant{ID: uuid.New(), ComplexID: f.complexID, UnitID: f.unit, Method: MethodQR,
		QRCodeHash: ptr("CODE-1"), Status: StatusActive, ValidUntil: ptr(hoursFromNow(2)), SingleUse: true}
	f.store = newGrantStore(g)
	svc := f.service(stubMembership{member: true}, stubQR{code: "x"})
	req := ValidateAccessRequest{Method: MethodQR, QRCode: ptr("CODE-1")}

	first, err := svc.ValidateAccess(f.guardCtx(), req)
	if err != nil || !first.Valid {
		t.Fatalf("first scan must be granted, got %+v err=%v", first, err)
	}
	if g.Status != StatusUsed {
		t.Fatalf("ACTIVE must transition to USED, got %s", g.Status)
	}
	second, err := svc.ValidateAccess(f.guardCtx(), req)
	if err != nil || second.Valid || second.Reason != ReasonAlreadyUsed {
		t.Fatalf("second scan must be refused ALREADY_USED, got %+v err=%v", second, err)
	}
}

func TestTC_QR_10_InvalidStatesAndWindowsAreRefused(t *testing.T) {
	cases := []struct {
		name       string
		status     string
		from, till *time.Time
		reason     string
	}{
		{"cancelled", StatusCancelled, nil, ptr(hoursFromNow(2)), ReasonCancelled},
		{"expired status", StatusExpired, nil, ptr(hoursFromNow(2)), ReasonExpired},
		{"window not started", StatusActive, ptr(hoursFromNow(1)), ptr(hoursFromNow(3)), ReasonNotYetValid},
		{"window ended", StatusActive, ptr(hoursFromNow(-3)), ptr(hoursFromNow(-1)), ReasonExpired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture()
			g := &AccessGrant{ID: uuid.New(), ComplexID: f.complexID, UnitID: f.unit, Method: MethodQR,
				QRCodeHash: ptr("CODE"), Status: tc.status, ValidFrom: tc.from, ValidUntil: tc.till, SingleUse: true}
			f.store = newGrantStore(g)
			svc := f.service(stubMembership{member: true}, stubQR{code: "x"})

			resp, err := svc.ValidateAccess(f.guardCtx(), ValidateAccessRequest{Method: MethodQR, QRCode: ptr("CODE")})
			if err != nil || resp.Valid || resp.Reason != tc.reason {
				t.Fatalf("want refused %s, got %+v err=%v", tc.reason, resp, err)
			}
			if g.Status != tc.status {
				t.Fatalf("a refused validation must not change state (%s → %s)", tc.status, g.Status)
			}
		})
	}
}

func TestTC_QR_11_CodeOfAnotherComplexLooksUnknown(t *testing.T) {
	f := newFixture()
	foreign := &AccessGrant{ID: uuid.New(), ComplexID: f.otherComplex, UnitID: uuid.New(), Method: MethodQR,
		QRCodeHash: ptr("FOREIGN"), Status: StatusActive, ValidUntil: ptr(hoursFromNow(2)), SingleUse: true}
	f.store = newGrantStore(foreign)
	svc := f.service(stubMembership{member: true}, stubQR{code: "x"})

	got, _ := svc.ValidateAccess(f.guardCtx(), ValidateAccessRequest{Method: MethodQR, QRCode: ptr("FOREIGN")})
	unknown, _ := svc.ValidateAccess(f.guardCtx(), ValidateAccessRequest{Method: MethodQR, QRCode: ptr("NEVER-ISSUED")})

	if got.Valid || got.Reason != ReasonNotFound || got.Message != unknown.Message || got.Reason != unknown.Reason {
		t.Fatalf("a foreign code must be indistinguishable from an unknown one: %+v vs %+v", got, unknown)
	}
	if foreign.Status != StatusActive {
		t.Fatal("a foreign complex's grant must not be consumed")
	}
}

func TestTC_QR_12_ValidateExceptionalInputs(t *testing.T) {
	f := newFixture()
	svc := f.service(stubMembership{member: true}, stubQR{code: "x"})

	cases := []struct {
		name string
		ctx  context.Context
		req  ValidateAccessRequest
		want error
	}{
		{"no complex in context", context.Background(), ValidateAccessRequest{Method: MethodQR, QRCode: ptr("c")}, ErrComplexIDRequired},
		{"QR without code", f.guardCtx(), ValidateAccessRequest{Method: MethodQR, QRCode: ptr("")}, ErrQRCodeRequired},
		{"PRE_AUTH without ID", f.guardCtx(), ValidateAccessRequest{Method: MethodPreAuth}, ErrVisitorIDRequired},
		{"unsupported method", f.guardCtx(), ValidateAccessRequest{Method: "NFC"}, ErrUnsupportedMethod},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := svc.ValidateAccess(tc.ctx, tc.req)
			if !errors.Is(err, tc.want) || resp != nil {
				t.Fatalf("want %v, got resp=%+v err=%v", tc.want, resp, err)
			}
		})
	}
	// The QR reader being down must fail fast (guard falls back to PRE_AUTH).
	down := f.service(stubMembership{member: true}, stubQR{down: true})
	if _, err := down.ValidateAccess(f.guardCtx(), ValidateAccessRequest{Method: MethodQR, QRCode: ptr("c")}); !errors.Is(err, qrsystem.ErrUnavailable) {
		t.Fatalf("QR reader down: want ErrUnavailable, got %v", err)
	}
}
