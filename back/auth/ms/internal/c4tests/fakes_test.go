package c4tests

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"superapp/auth/internal/core/domain"
	"superapp/auth/internal/core/ports/repository"
	authlogin "superapp/auth/internal/core/usecases/auth/login"
	authlogout "superapp/auth/internal/core/usecases/auth/logout"
	authrefresh "superapp/auth/internal/core/usecases/auth/refresh"
	authregister "superapp/auth/internal/core/usecases/auth/register"
	"superapp/auth/internal/core/usecases/contracts"
)

// ---------------------------------------------------------------------------
// memRepo: repositorio en memoria (fake). Imita las reglas del repositorio
// PostgreSQL: correo unico, rotacion de un solo uso del refresh token y
// revocacion de la sesion completa cuando se reutiliza un token.
// ---------------------------------------------------------------------------

type tokenRec struct {
	sessionID string
	expires   time.Time
	consumed  bool
	revoked   bool
}

type sessRec struct {
	userID  string
	expires time.Time
	revoked bool
}

type memRepo struct {
	repository.AuthorizationRepository // metodos no usados: nil

	mu       sync.Mutex
	byEmail  map[string]*domain.User
	byID     map[string]*domain.User
	hashes   map[string]string // email -> hash de contrasena
	tokens   map[string]*tokenRec
	sessions map[string]*sessRec
	perms    map[string]bool

	// inyeccion de fallos
	errUserByEmail   error
	errSetLastLogin  error
	errCreateSession error
	errRotate        error
	errCreateUser    error

	// espias
	createdEmail       string
	createdHash        string
	createUserCalls    int
	setLastLoginCalls  int
	createSessionCalls int
	rotateCalls        int
	permCalls          int
	lastPerm           repository.PermissionAllowedParams
}

func newMemRepo() *memRepo {
	return &memRepo{
		byEmail:  map[string]*domain.User{},
		byID:     map[string]*domain.User{},
		hashes:   map[string]string{},
		tokens:   map[string]*tokenRec{},
		sessions: map[string]*sessRec{},
		perms:    map[string]bool{},
	}
}

var userSeq int64

func (r *memRepo) seedUser(email, password, status string) *domain.User {
	n := atomic.AddInt64(&userSeq, 1)
	u := &domain.User{ID: fmt.Sprintf("user-%d", n), Email: email, FullName: "Usuario Prueba", Status: status}
	r.byEmail[email] = u
	r.byID[u.ID] = u
	r.hashes[email] = "hash:" + password
	return u
}

func (r *memRepo) CreateUser(_ context.Context, p repository.CreateUserParams) (*domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.createUserCalls++
	if r.errCreateUser != nil {
		return nil, r.errCreateUser
	}
	if _, ok := r.byEmail[p.Email]; ok {
		return nil, domain.ErrEmailTaken
	}
	r.createdEmail, r.createdHash = p.Email, p.PasswordHash
	u := &domain.User{ID: p.ID, Email: p.Email, FullName: p.FullName, Status: "ACTIVE"}
	r.byEmail[p.Email] = u
	r.byID[p.ID] = u
	r.hashes[p.Email] = p.PasswordHash
	return u, nil
}

func (r *memRepo) UserByEmail(_ context.Context, email string) (*domain.User, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.errUserByEmail != nil {
		return nil, "", r.errUserByEmail
	}
	u, ok := r.byEmail[email]
	if !ok {
		return nil, "", nil
	}
	return u, r.hashes[email], nil
}

func (r *memRepo) UserByID(_ context.Context, id string) (*domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return u, nil
}

func (r *memRepo) UpdateUser(context.Context, repository.UpdateUserParams) error { return nil }

func (r *memRepo) SetLastLogin(context.Context, string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setLastLoginCalls++
	return r.errSetLastLogin
}

func (r *memRepo) CreateSession(_ context.Context, p repository.CreateSessionParams) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.createSessionCalls++
	if r.errCreateSession != nil {
		return r.errCreateSession
	}
	r.sessions[p.SessionID] = &sessRec{userID: p.UserID, expires: p.ExpiresAt}
	r.tokens[p.RefreshHash] = &tokenRec{sessionID: p.SessionID, expires: p.ExpiresAt}
	return nil
}

func (r *memRepo) revokeSessionLocked(sid string) {
	if s, ok := r.sessions[sid]; ok {
		s.revoked = true
	}
	for _, t := range r.tokens {
		if t.sessionID == sid {
			t.revoked = true
		}
	}
}

func (r *memRepo) RotateRefresh(_ context.Context, p repository.RotateRefreshParams) (repository.RotateRefreshResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rotateCalls++
	if r.errRotate != nil {
		return repository.RotateRefreshResult{}, r.errRotate
	}
	old, ok := r.tokens[p.OldHash]
	if !ok {
		return repository.RotateRefreshResult{}, domain.ErrInvalidToken
	}
	sess := r.sessions[old.sessionID]
	user := r.byID[sess.userID]
	if old.consumed || old.revoked || sess.revoked || !old.expires.After(p.Now) || user.Status != "ACTIVE" {
		if old.consumed || old.revoked {
			// reutilizacion: se asume robo y se revoca toda la sesion
			r.revokeSessionLocked(old.sessionID)
		}
		return repository.RotateRefreshResult{}, domain.ErrInvalidToken
	}
	old.consumed = true
	r.tokens[p.NewHash] = &tokenRec{sessionID: old.sessionID, expires: p.ExpiresAt}
	sess.expires = p.ExpiresAt
	return repository.RotateRefreshResult{SessionID: old.sessionID, UserID: sess.userID}, nil
}

func (r *memRepo) RevokeRefresh(_ context.Context, hash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.tokens[hash]; ok {
		r.revokeSessionLocked(t.sessionID)
	}
	return nil // un token desconocido no es error (igual que el UPDATE sin filas)
}

func (r *memRepo) RevokeAllSessions(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for sid, s := range r.sessions {
		if s.userID == userID {
			r.revokeSessionLocked(sid)
		}
	}
	return nil
}

func (r *memRepo) RevokeSession(context.Context, repository.RevokeSessionParams) error { return nil }
func (r *memRepo) Sessions(context.Context, string) ([]domain.Session, error)          { return nil, nil }

func permKey(user, code string, tenant *string) string {
	t := ""
	if tenant != nil {
		t = *tenant
	}
	return user + "|" + code + "|" + t
}

func (r *memRepo) grant(user, code string, tenant *string) {
	r.perms[permKey(user, code, tenant)] = true
}

func (r *memRepo) PermissionAllowed(_ context.Context, p repository.PermissionAllowedParams) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.permCalls++
	r.lastPerm = p
	return r.perms[permKey(p.UserID, p.PermissionCode, p.TenantID)], nil
}

func (r *memRepo) activeSessions(userID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, s := range r.sessions {
		if s.userID == userID && !s.revoked {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// Dobles de contrasenas y tokens
// ---------------------------------------------------------------------------

// fakePasswords: stub de hash/verificacion con espia de Burn.
type fakePasswords struct{ burned int }

func (p *fakePasswords) Hash(password string) (string, error) { return "hash:" + password, nil }
func (p *fakePasswords) Verify(encoded, password string) error {
	if encoded == "hash:"+password {
		return nil
	}
	return errors.New("password mismatch")
}
func (p *fakePasswords) Burn(string) { p.burned++ }

// fakeSigner: stub del emisor de access tokens.
type fakeSigner struct{}

func (fakeSigner) Issue(userID, sessionID string) (string, time.Time, error) {
	return "access." + userID + "." + sessionID, time.Now().Add(15 * time.Minute), nil
}

// ---------------------------------------------------------------------------
// Entorno de prueba con los cuatro casos de uso reales
// ---------------------------------------------------------------------------

type env struct {
	repo     *memRepo
	pw       *fakePasswords
	login    *authlogin.UseCase
	register *authregister.UseCase
	refresh  *authrefresh.UseCase
	logout   *authlogout.UseCase
}

func newEnv(refreshTTL time.Duration) *env {
	repo := newMemRepo()
	pw := &fakePasswords{}
	var seq int64
	ids := func() string { return fmt.Sprintf("id-%d", atomic.AddInt64(&seq, 1)) }
	cfg := contracts.Config{AccessTTL: 15 * time.Minute, RefreshTTL: refreshTTL}
	return &env{
		repo:     repo,
		pw:       pw,
		login:    authlogin.New(repo, repo, pw, fakeSigner{}, ids, cfg),
		register: authregister.New(repo, pw, ids),
		refresh:  authrefresh.New(repo, fakeSigner{}, ids, cfg),
		logout:   authlogout.New(repo),
	}
}

const (
	okEmail    = "ana@example.test"
	okPassword = "Correct#Pass12"
)

// loggedIn crea un usuario activo, inicia sesion y devuelve el refresh token.
func (e *env) loggedIn(t interface{ Fatalf(string, ...any) }) (userID, refreshToken string) {
	u := e.repo.seedUser(okEmail, okPassword, "ACTIVE")
	res, err := e.login.Execute(context.Background(), authlogin.Requester{Email: okEmail, Password: okPassword})
	if err != nil {
		t.Fatalf("login de preparacion fallo: %v", err)
	}
	return u.ID, res.Tokens.RefreshToken
}

func repeat(s string, n int) string { return strings.Repeat(s, n) }
