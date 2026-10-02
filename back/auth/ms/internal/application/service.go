package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"superapp/auth/internal/domain"
)

type Config struct{ AccessTTL, RefreshTTL time.Duration }
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}
type Service struct {
	repo      Repository
	passwords PasswordHasher
	tokens    TokenIssuer
	ids       IDGenerator
	cfg       Config
	now       func() time.Time
}

func New(repo Repository, passwords PasswordHasher, tokens TokenIssuer, ids IDGenerator, cfg Config) *Service {
	return &Service{repo: repo, passwords: passwords, tokens: tokens, ids: ids, cfg: cfg, now: time.Now}
}
func (s *Service) Ready(ctx context.Context) error { return s.repo.Ping(ctx) }
func (s *Service) Register(ctx context.Context, email, password, name string) (*domain.User, error) {
	email = domain.NormalizeEmail(email)
	parsed, parseErr := mail.ParseAddress(email)
	if parseErr != nil || parsed.Address != email || strings.TrimSpace(name) == "" || len(name) > 200 || len(email) > 320 {
		return nil, fmt.Errorf("%w: valid email and full name are required", domain.ErrInvalidArgument)
	}
	if len(password) < 12 || len(password) > 72 {
		return nil, fmt.Errorf("%w: password must be between 12 and 72 bytes", domain.ErrInvalidArgument)
	}
	hash, err := s.passwords.Hash(password)
	if err != nil {
		return nil, err
	}
	return s.repo.CreateUser(ctx, s.ids.New(), email, hash, strings.TrimSpace(name))
}
func (s *Service) Login(ctx context.Context, email, password, device, ua, ip string) (*domain.User, *TokenPair, error) {
	u, hash, err := s.repo.UserByEmail(ctx, domain.NormalizeEmail(email))
	if err != nil {
		return nil, nil, err
	}
	if u == nil {
		s.passwords.Burn(password)
		return nil, nil, domain.ErrInvalidCredentials
	}
	if err = s.passwords.Verify(hash, password); err != nil {
		return nil, nil, domain.ErrInvalidCredentials
	}
	if u.Status != "ACTIVE" {
		return nil, nil, domain.ErrInvalidCredentials
	}
	if err = s.repo.SetLastLogin(ctx, u.ID); err != nil {
		return nil, nil, err
	}
	pair, err := s.newSession(ctx, u, device, ua, ip)
	return u, pair, err
}
func (s *Service) newSession(ctx context.Context, u *domain.User, device, ua, ip string) (*TokenPair, error) {
	sessionID, err := newID()
	if err != nil {
		return nil, err
	}
	raw, hash, err := newRefresh()
	if err != nil {
		return nil, err
	}
	now := s.now()
	refreshExpiry := now.Add(s.cfg.RefreshTTL)
	access, accessExpiry, err := s.tokens.Issue(u.ID, u.Email, sessionID)
	if err != nil {
		return nil, err
	}
	if err = s.repo.CreateSession(ctx, sessionID, u.ID, hash, refreshExpiry, device, ua, ip); err != nil {
		return nil, err
	}
	return &TokenPair{access, raw, int64(accessExpiry.Sub(now).Seconds())}, nil
}
func (s *Service) Refresh(ctx context.Context, raw string) (*TokenPair, error) {
	oldHash := hashRefresh(raw)
	newRaw, newHash, err := newRefresh()
	if err != nil {
		return nil, err
	}
	now := s.now()
	sessionID, userID, email, err := s.repo.RotateRefresh(ctx, oldHash, newHash, s.ids.New(), now.Add(s.cfg.RefreshTTL), now)
	if err != nil {
		return nil, err
	}
	access, expiry, err := s.tokens.Issue(userID, email, sessionID)
	if err != nil {
		return nil, err
	}
	return &TokenPair{access, newRaw, int64(expiry.Sub(now).Seconds())}, nil
}
func (s *Service) Logout(ctx context.Context, raw string) error {
	return s.repo.RevokeRefresh(ctx, hashRefresh(raw))
}
func (s *Service) LogoutAll(ctx context.Context, userID string) error {
	return s.repo.RevokeAllSessions(ctx, userID)
}
func (s *Service) Authenticate(raw string) (*TokenClaims, error) { return s.tokens.Verify(raw) }
func (s *Service) JWKS() map[string]any                          { return s.tokens.JWKS() }
func (s *Service) User(ctx context.Context, id string) (*domain.User, error) {
	return s.repo.UserByID(ctx, id)
}
func (s *Service) UpdateUser(ctx context.Context, id, email, name string) error {
	email = domain.NormalizeEmail(email)
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || strings.TrimSpace(name) == "" || len(name) > 200 || len(email) > 320 {
		return fmt.Errorf("%w: valid email and full name are required", domain.ErrInvalidArgument)
	}
	return s.repo.UpdateUser(ctx, id, email, strings.TrimSpace(name))
}
func (s *Service) Sessions(ctx context.Context, id string) ([]domain.Session, error) {
	return s.repo.Sessions(ctx, id)
}
func (s *Service) RevokeSession(ctx context.Context, userID, sessionID string) error {
	return s.repo.RevokeSession(ctx, userID, sessionID)
}
func (s *Service) HasPermission(ctx context.Context, userID, permission string, tenant *string) (bool, error) {
	return s.repo.PermissionAllowed(ctx, userID, permission, tenant)
}
func (s *Service) EffectivePermissions(ctx context.Context, userID string, tenant *string) ([]string, error) {
	return s.repo.EffectivePermissions(ctx, userID, tenant)
}
func (s *Service) EffectiveRoles(ctx context.Context, userID string, tenant *string) ([]domain.Role, error) {
	return s.repo.EffectiveRoles(ctx, userID, tenant)
}
func (s *Service) CreateRole(ctx context.Context, r domain.Role) error {
	if r.Scope != "SYSTEM" && r.Scope != "TENANT" {
		return fmt.Errorf("%w: scope must be SYSTEM or TENANT", domain.ErrInvalidArgument)
	}
	if (r.Scope == "TENANT") != (r.TenantID != nil) {
		return fmt.Errorf("%w: system roles have no tenant; tenant roles require one", domain.ErrInvalidArgument)
	}
	if !domain.ValidRoleCode(r.Code) || strings.TrimSpace(r.Name) == "" || len(r.Name) > 100 || len(r.Description) > 500 {
		return fmt.Errorf("%w: role code, name, or description is invalid", domain.ErrInvalidArgument)
	}
	r.Enabled = true
	if r.ID == "" {
		r.ID = s.ids.New()
	}
	return s.repo.CreateRole(ctx, r)
}
func (s *Service) Roles(ctx context.Context, tenant *string) ([]domain.Role, error) {
	return s.repo.Roles(ctx, tenant)
}
func (s *Service) RoleByID(ctx context.Context, id string, tenant *string) (*domain.Role, error) {
	return s.repo.RoleByID(ctx, id, tenant)
}
func (s *Service) UpdateRole(ctx context.Context, r domain.Role, tenant *string) error {
	if strings.TrimSpace(r.Name) == "" || len(r.Name) > 100 || len(r.Description) > 500 {
		return fmt.Errorf("%w: role name or description is invalid", domain.ErrInvalidArgument)
	}
	return s.repo.UpdateRole(ctx, r, tenant)
}
func (s *Service) DeleteRole(ctx context.Context, id string, tenant *string) error {
	return s.repo.DeleteRole(ctx, id, tenant)
}
func (s *Service) CreatePermission(ctx context.Context, p domain.Permission) error {
	if !domain.ValidPermissionCode(p.Code) || len(p.Description) > 500 {
		return fmt.Errorf("%w: permission code or description is invalid", domain.ErrInvalidArgument)
	}
	if p.ID == "" {
		p.ID = s.ids.New()
	}
	return s.repo.CreatePermission(ctx, p)
}
func (s *Service) Permissions(ctx context.Context) ([]domain.Permission, error) {
	return s.repo.Permissions(ctx)
}
func (s *Service) PermissionByID(ctx context.Context, id string) (*domain.Permission, error) {
	return s.repo.PermissionByID(ctx, id)
}
func (s *Service) UpdatePermission(ctx context.Context, p domain.Permission) error {
	if !domain.ValidPermissionCode(p.Code) || len(p.Description) > 500 {
		return fmt.Errorf("%w: permission code or description is invalid", domain.ErrInvalidArgument)
	}
	return s.repo.UpdatePermission(ctx, p)
}
func (s *Service) DeletePermission(ctx context.Context, id string) error {
	return s.repo.DeletePermission(ctx, id)
}
func (s *Service) GrantPermission(ctx context.Context, roleID, permissionID string, tenant *string) error {
	return s.repo.GrantPermission(ctx, roleID, permissionID, tenant)
}
func (s *Service) RevokePermission(ctx context.Context, roleID, permissionID string, tenant *string) error {
	return s.repo.RevokePermission(ctx, roleID, permissionID, tenant)
}
func (s *Service) AssignRole(ctx context.Context, userID, roleID string, tenant *string, actor string) error {
	return s.repo.AssignRole(ctx, userID, roleID, tenant, actor)
}
func (s *Service) RevokeRole(ctx context.Context, userID, roleID string, tenant *string) error {
	return s.repo.RevokeRole(ctx, userID, roleID, tenant)
}
func (s *Service) SetUserStatus(ctx context.Context, userID, status string) error {
	if status != "ACTIVE" && status != "DISABLED" && status != "LOCKED" {
		return fmt.Errorf("%w: invalid user status", domain.ErrInvalidArgument)
	}
	return s.repo.SetUserStatus(ctx, userID, status)
}

func newRefresh() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	hash = hashRefresh(raw)
	return
}
func hashRefresh(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
