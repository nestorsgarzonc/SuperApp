package access

// Dobles de prueba compartidos por los componentes C1..C4 del paquete access.
// Se reutilizan fakeRepo, fakeDirectory, fakeQR, chanNotifier, membersDirectory
// y newTestService de service_hardening_test.go / notify_test.go (sin duplicar).

import (
	"context"
	"io"
	"log/slog"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"controldeaccesos/internal/audit"
	"controldeaccesos/internal/notifier"
	authmw "controldeaccesos/internal/platform/middleware"
	"controldeaccesos/internal/qrsystem"
)

// discardLogger es un DUMMY: llena la firma de NewService; nunca se verifica.
func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// ctxWith arma el contexto que deja el middleware de autenticacion. uuid.Nil
// significa "no poner esa clave" (usuario o complejo ausente).
func ctxWith(user, complex uuid.UUID) context.Context {
	ctx := context.Background()
	if user != uuid.Nil {
		ctx = context.WithValue(ctx, authmw.UserIDKey, user.String())
	}
	if complex != uuid.Nil {
		ctx = context.WithValue(ctx, authmw.ComplexIDKey, complex.String())
	}
	return ctx
}

// ── memRepo: FAKE del Repository ─────────────────────────────────────────────
//
// Implementacion funcional en memoria. Supuestos (replican repository.go):
//   - TryMarkGrantUsed es atomico y solo transiciona desde ACTIVE (repository.go:274-288).
//   - CancelAccessGrant solo cancela ACTIVE; si no, ErrAccessGrantNotFound (repository.go:254-271).
//   - GetActiveAccessGrantsByVisitorID filtra complejo, cedula y status=ACTIVE,
//     ordenado por created_at DESC (repository.go:221-227).
//   - Las lecturas devuelven COPIAS (como un SELECT), no punteros al almacen.
//
// errOn[metodo] fuerza un error en ese metodo; calls registra cada invocacion.
type memRepo struct {
	Repository // lo no implementado provoca panic (nil interface)

	mu          sync.Mutex
	grants      map[uuid.UUID]*AccessGrant
	seq         int
	errOn       map[string]error
	calls       map[string]int
	loseTryMark bool // TryMarkGrantUsed devuelve (false, nil) sin tocar el estado

	createReqs []CreateAccessGrantRequest
	txReqs     [][]CreateAccessGrantRequest
	forUser    []forUserCall
	usedByArgs []uuid.UUID
}

func newMemRepo() *memRepo {
	return &memRepo{
		grants: map[uuid.UUID]*AccessGrant{},
		errOn:  map[string]error{},
		calls:  map[string]int{},
	}
}

// begin registra la llamada y devuelve el error inyectado (con el lock tomado).
func (r *memRepo) begin(name string) error {
	r.mu.Lock()
	r.calls[name]++
	return r.errOn[name]
}

func (r *memRepo) nextTime() time.Time {
	r.seq++
	return time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(r.seq) * time.Second)
}

// put inserta un pase tal cual (fixture); no cuenta como llamada.
func (r *memRepo) put(g AccessGrant) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if g.ID == uuid.Nil {
		g.ID = uuid.New()
	}
	if g.CreatedAt.IsZero() {
		g.CreatedAt = r.nextTime()
	}
	cp := g
	r.grants[g.ID] = &cp
}

// snapshot devuelve el estado almacenado del pase (para el oraculo por estado).
func (r *memRepo) snapshot(id uuid.UUID) AccessGrant {
	r.mu.Lock()
	defer r.mu.Unlock()
	return *r.grants[id]
}

func (r *memRepo) count(name string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls[name]
}

func (r *memRepo) total() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, c := range r.calls {
		n += c
	}
	return n
}

func (r *memRepo) build(complexID uuid.UUID, req CreateAccessGrantRequest, by uuid.UUID) *AccessGrant {
	id := uuid.New()
	from := time.Now()
	if req.ValidFrom != nil {
		from = *req.ValidFrom
	}
	until := req.ValidUntil
	g := &AccessGrant{
		ID: id, ComplexID: complexID, UnitID: req.UnitID, CreatedBy: by,
		Method: req.Method, VisitorType: req.VisitorType, VisitorName: req.VisitorName,
		VisitorIDNumber: req.VisitorIDNumber, Details: req.Details,
		Status: StatusActive, ValidFrom: &from, ValidUntil: &until,
		CreatedAt: r.nextTime(), SingleUse: true,
	}
	if req.Method == MethodQR {
		h := req.qrCode
		if h == "" {
			h = "derived-" + id.String()
		}
		g.QRCodeHash = &h
	}
	return g
}

func (r *memRepo) CreateAccessGrant(_ context.Context, complexID uuid.UUID, req CreateAccessGrantRequest, by uuid.UUID) (*AccessGrant, error) {
	if err := r.begin("CreateAccessGrant"); err != nil {
		r.mu.Unlock()
		return nil, err
	}
	defer r.mu.Unlock()
	r.createReqs = append(r.createReqs, req)
	g := r.build(complexID, req, by)
	cp := *g
	r.grants[g.ID] = &cp
	return g, nil
}

func (r *memRepo) CreateAccessGrantsTx(_ context.Context, complexID, by uuid.UUID, reqs []CreateAccessGrantRequest) ([]*AccessGrant, error) {
	if err := r.begin("CreateAccessGrantsTx"); err != nil {
		r.mu.Unlock()
		return nil, err
	}
	defer r.mu.Unlock()
	r.txReqs = append(r.txReqs, reqs)
	out := make([]*AccessGrant, 0, len(reqs))
	for _, req := range reqs {
		g := r.build(complexID, req, by)
		cp := *g
		r.grants[g.ID] = &cp
		out = append(out, g)
	}
	return out, nil
}

func (r *memRepo) GetAccessGrant(_ context.Context, id uuid.UUID) (*AccessGrant, error) {
	if err := r.begin("GetAccessGrant"); err != nil {
		r.mu.Unlock()
		return nil, err
	}
	defer r.mu.Unlock()
	g, ok := r.grants[id]
	if !ok {
		return nil, ErrAccessGrantNotFound
	}
	cp := *g
	return &cp, nil
}

func (r *memRepo) GetAccessGrantByQRHash(_ context.Context, hash string) (*AccessGrant, error) {
	if err := r.begin("GetAccessGrantByQRHash"); err != nil {
		r.mu.Unlock()
		return nil, err
	}
	defer r.mu.Unlock()
	for _, g := range r.grants {
		if g.QRCodeHash != nil && *g.QRCodeHash == hash {
			cp := *g
			return &cp, nil
		}
	}
	return nil, ErrAccessGrantNotFound
}

func (r *memRepo) GetActiveAccessGrantsByVisitorID(_ context.Context, complexID uuid.UUID, idNumber string) ([]*AccessGrant, error) {
	if err := r.begin("GetActiveAccessGrantsByVisitorID"); err != nil {
		r.mu.Unlock()
		return nil, err
	}
	defer r.mu.Unlock()
	var out []*AccessGrant
	for _, g := range r.grants {
		if g.ComplexID == complexID && g.VisitorIDNumber != nil && *g.VisitorIDNumber == idNumber && g.Status == StatusActive {
			cp := *g
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (r *memRepo) CancelAccessGrant(_ context.Context, id uuid.UUID) error {
	if err := r.begin("CancelAccessGrant"); err != nil {
		r.mu.Unlock()
		return err
	}
	defer r.mu.Unlock()
	g, ok := r.grants[id]
	if !ok || g.Status != StatusActive {
		return ErrAccessGrantNotFound
	}
	g.Status = StatusCancelled
	return nil
}

func (r *memRepo) TryMarkGrantUsed(_ context.Context, id, usedBy uuid.UUID) (bool, error) {
	if err := r.begin("TryMarkGrantUsed"); err != nil {
		r.mu.Unlock()
		return false, err
	}
	defer r.mu.Unlock()
	r.usedByArgs = append(r.usedByArgs, usedBy)
	if r.loseTryMark {
		return false, nil
	}
	g, ok := r.grants[id]
	if !ok || g.Status != StatusActive {
		return false, nil
	}
	now := time.Now()
	g.Status, g.UsedAt, g.UsedBy = StatusUsed, &now, &usedBy
	return true, nil
}

func (r *memRepo) GetComplexAccessGrantsForUser(_ context.Context, complexID, userID uuid.UUID, method string) ([]*AccessGrant, error) {
	if err := r.begin("GetComplexAccessGrantsForUser"); err != nil {
		r.mu.Unlock()
		return nil, err
	}
	defer r.mu.Unlock()
	r.forUser = append(r.forUser, forUserCall{complexID, userID, method})
	return nil, nil
}

// ── noCallsRepo: MOCK ESTRICTO del Repository ────────────────────────────────
//
// La expectativa verificada ES "no se toca el repositorio": cualquier metodo
// hace t.Fatalf. Supuesto: solo se usa en el hilo del test (rutas sincronas).
type noCallsRepo struct{ t *testing.T }

func (r noCallsRepo) fail(m string) {
	r.t.Helper()
	r.t.Fatalf("el repositorio NO debia llamarse, pero se llamo a %s", m)
}
func (r noCallsRepo) CreateAccessGrant(context.Context, uuid.UUID, CreateAccessGrantRequest, uuid.UUID) (*AccessGrant, error) {
	r.fail("CreateAccessGrant")
	return nil, nil
}
func (r noCallsRepo) GetAccessGrant(context.Context, uuid.UUID) (*AccessGrant, error) {
	r.fail("GetAccessGrant")
	return nil, nil
}
func (r noCallsRepo) GetAccessGrantByQRHash(context.Context, string) (*AccessGrant, error) {
	r.fail("GetAccessGrantByQRHash")
	return nil, nil
}
func (r noCallsRepo) GetUnitAccessGrants(context.Context, uuid.UUID, *string) ([]*AccessGrant, error) {
	r.fail("GetUnitAccessGrants")
	return nil, nil
}
func (r noCallsRepo) GetComplexAccessGrants(context.Context, uuid.UUID, *string, *string) ([]*AccessGrant, error) {
	r.fail("GetComplexAccessGrants")
	return nil, nil
}
func (r noCallsRepo) GetComplexAccessGrantsForUser(context.Context, uuid.UUID, uuid.UUID, string) ([]*AccessGrant, error) {
	r.fail("GetComplexAccessGrantsForUser")
	return nil, nil
}
func (r noCallsRepo) CreateAccessGrantsTx(context.Context, uuid.UUID, uuid.UUID, []CreateAccessGrantRequest) ([]*AccessGrant, error) {
	r.fail("CreateAccessGrantsTx")
	return nil, nil
}
func (r noCallsRepo) GrantSummary(context.Context, uuid.UUID) (*GrantSummary, error) {
	r.fail("GrantSummary")
	return nil, nil
}
func (r noCallsRepo) GetActiveAccessGrantsByVisitorID(context.Context, uuid.UUID, string) ([]*AccessGrant, error) {
	r.fail("GetActiveAccessGrantsByVisitorID")
	return nil, nil
}
func (r noCallsRepo) CancelAccessGrant(context.Context, uuid.UUID) error {
	r.fail("CancelAccessGrant")
	return nil
}
func (r noCallsRepo) TryMarkGrantUsed(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	r.fail("TryMarkGrantUsed")
	return false, nil
}

// ── stubDirectory: STUB del DirectoryRepository ──────────────────────────────
//
// Respuestas fijas (controla entradas indirectas). Inmutable => seguro entre
// goroutines (el notificador lo consulta en segundo plano). Valor cero:
// la unidad existe y pertenece a complexID.
type stubDirectory struct {
	complexID   uuid.UUID
	missing     bool  // GetUnitComplexID -> found=false
	lookupErr   error // GetUnitComplexID -> error
	unitName    string
	unitErr     error
	displayName string
	displayErr  error
	members     []uuid.UUID
	membersErr  error
}

func (d stubDirectory) GetUnitComplexID(context.Context, uuid.UUID) (uuid.UUID, bool, error) {
	if d.lookupErr != nil {
		return uuid.Nil, false, d.lookupErr
	}
	return d.complexID, !d.missing, nil
}
func (d stubDirectory) GetUnitName(context.Context, uuid.UUID) (string, error) {
	return d.unitName, d.unitErr
}
func (d stubDirectory) GetUserDisplayName(context.Context, uuid.UUID) (string, error) {
	return d.displayName, d.displayErr
}
func (d stubDirectory) GetUnitMemberIDs(context.Context, uuid.UUID) ([]uuid.UUID, error) {
	return d.members, d.membersErr
}

// ── stubMembership: STUB + SPY del UnitMembershipChecker ─────────────────────
type stubMembership struct {
	mu    sync.Mutex
	ok    bool
	err   error
	calls []membershipCall
}

type membershipCall struct{ user, unit uuid.UUID }

func (m *stubMembership) IsUserAssociatedWithUnit(_ context.Context, user, unit uuid.UUID) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, membershipCall{user, unit})
	return m.ok, m.err
}

func (m *stubMembership) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

// ── stubQR: STUB + SPY del QRSystem ──────────────────────────────────────────
//
// Entrega los codigos de `codes` en orden (luego uuid aleatorio). issueErr /
// scanErr simulan el sistema caido. No reemplaza al breaker real (C6).
type stubQR struct {
	mu       sync.Mutex
	codes    []string
	issueErr error
	scanErr  error
	avail    qrsystem.Public
	issued   int
	scanned  int
}

func (q *stubQR) Issue(context.Context) (string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.issueErr != nil {
		return "", q.issueErr
	}
	i := q.issued
	q.issued++
	if i < len(q.codes) {
		return q.codes[i], nil
	}
	return uuid.NewString(), nil
}
func (q *stubQR) Scan(context.Context) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.scanned++
	return q.scanErr
}
func (q *stubQR) Availability() qrsystem.Public { return q.avail }

func (q *stubQR) counts() (issued, scanned int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.issued, q.scanned
}

// ── spyAuditor: SPY del Auditor ──────────────────────────────────────────────
//
// Registra cada entrada; err simula un fallo de escritura (best-effort).
type spyAuditor struct {
	mu      sync.Mutex
	entries []audit.Entry
	err     error
}

func (a *spyAuditor) Write(_ context.Context, e audit.Entry) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = append(a.entries, e)
	return a.err
}

func (a *spyAuditor) all() []audit.Entry {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]audit.Entry(nil), a.entries...)
}

// only exige exactamente una entrada y la devuelve.
func (a *spyAuditor) only(t *testing.T) audit.Entry {
	t.Helper()
	es := a.all()
	if len(es) != 1 {
		t.Fatalf("se esperaba 1 entrada de auditoria, hubo %d: %+v", len(es), es)
	}
	return es[0]
}

// ── world: ensamblado comun del SUT ──────────────────────────────────────────
type world struct {
	complexID, userID, unitID uuid.UUID
	repo                      *memRepo
	dir                       stubDirectory
	mem                       *stubMembership
	qr                        *stubQR
	aud                       *spyAuditor
	notif                     *chanNotifier
	svc                       *service
}

func newWorld() *world {
	w := &world{
		complexID: uuid.New(), userID: uuid.New(), unitID: uuid.New(),
		repo: newMemRepo(),
		mem:  &stubMembership{ok: true},
		qr:   &stubQR{avail: qrsystem.Public{Available: true, State: "CLOSED"}},
		aud:  &spyAuditor{},
	}
	w.dir = stubDirectory{complexID: w.complexID, unitName: "A-101", displayName: "Ana"}
	w.build()
	return w
}

// build (re)crea el servicio con los dobles actuales del world.
func (w *world) build() {
	w.svc = &service{
		repo:        w.repo,
		directory:   w.dir,
		unitChecker: w.mem,
		qr:          w.qr,
		auditor:     w.aud,
		logger:      discardLogger(),
	}
	if w.notif != nil {
		w.svc.notifier = *w.notif
	}
}

func (w *world) withNotifier() *chanNotifier {
	n := chanNotifier{events: make(chan notifier.Event, 8)}
	w.notif = &n
	w.build()
	return w.notif
}

func (w *world) ctx() context.Context { return ctxWith(w.userID, w.complexID) }

// grant crea y guarda un pase ACTIVE, de un solo uso, vigente (±1 h), en el
// complejo y unidad del world; mod permite ajustarlo.
func (w *world) grant(mod func(*AccessGrant)) AccessGrant {
	id := uuid.New()
	hash := "qr-" + id.String()
	from, until := time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
	g := AccessGrant{
		ID: id, ComplexID: w.complexID, UnitID: w.unitID, CreatedBy: w.userID,
		Method: MethodQR, VisitorType: VisitorTypeVisitor, VisitorName: "Juan",
		QRCodeHash: &hash, Status: StatusActive, ValidFrom: &from, ValidUntil: &until,
		SingleUse: true,
	}
	if mod != nil {
		mod(&g)
	}
	w.repo.put(g)
	return g
}

func strp(s string) *string { return &s }
