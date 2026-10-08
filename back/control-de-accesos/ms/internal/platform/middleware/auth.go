package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	apperrors "controldeaccesos/internal/platform/errors"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Context keys for storing user information
type contextKey string

const (
	UserIDKey    contextKey = "user_id"
	ComplexIDKey contextKey = "complex_id"
	UserRoleKey  contextKey = "user_role"
	EmailKey     contextKey = "email"
)

// JWTClaims represents the JWT token payload
type JWTClaims struct {
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	Role      string `json:"role,omitempty"`
	ComplexID string `json:"complex_id,omitempty"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`

	// fromJWKS marks identity-only claims from an auth-issued RS256 token: the
	// complex and role still have to be resolved from the memberships.
	fromJWKS bool
}

// Headers the API gateway injects once it has validated the caller (see
// docs/api-gateway.md). They are only trusted together with a valid
// X-Gateway-Secret; without it they are ignored like any other client header.
const (
	HeaderGatewaySecret = "X-Gateway-Secret"
	HeaderUserID        = "X-User-ID"
	HeaderComplexID     = "X-Complex-ID"
	HeaderComplexRole   = "X-Complex-Role"
)

// gatewayClaims reads the identity injected by the gateway. viaGateway is true
// when the request carries X-Gateway-Secret at all, in which case err says
// whether it is acceptable: the secret must match GATEWAY_SECRET (constant
// time) and X-User-ID must be a UUID. A request without the header is not a
// gateway request and falls through to the regular JWT path.
func gatewayClaims(r *http.Request) (claims *JWTClaims, viaGateway bool, err error) {
	provided := r.Header.Get(HeaderGatewaySecret)
	if provided == "" {
		return nil, false, nil
	}
	secret := os.Getenv("GATEWAY_SECRET")
	if secret == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(secret)) != 1 {
		return nil, true, errors.New("request did not come through the gateway")
	}
	userID, perr := uuid.Parse(r.Header.Get(HeaderUserID))
	if perr != nil {
		return nil, true, errors.New("authentication required")
	}
	c := &JWTClaims{UserID: userID.String(), Role: strings.ToUpper(strings.TrimSpace(r.Header.Get(HeaderComplexRole)))}
	if raw := r.Header.Get(HeaderComplexID); raw != "" {
		complexID, perr := uuid.Parse(raw)
		if perr != nil {
			return nil, true, errors.New("invalid complex ID format")
		}
		c.ComplexID = complexID.String()
	}
	return c, true, nil
}

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if claims, viaGateway, err := gatewayClaims(r); viaGateway {
			if err != nil {
				respondWithError(w, http.StatusUnauthorized, err.Error())
				return
			}
			next.ServeHTTP(w, r.WithContext(withClaims(r.Context(), claims)))
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			respondWithError(w, http.StatusUnauthorized, "Missing authorization header")
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			respondWithError(w, http.StatusUnauthorized, "Invalid authorization header format")
			return
		}

		claims, err := ValidateJWTContext(r.Context(), parts[1])
		if err != nil {
			respondWithError(w, http.StatusUnauthorized, fmt.Sprintf("Invalid token: %v", err))
			return
		}
		if claims.fromJWKS {
			if status, err := attachComplex(r, claims); err != nil {
				respondWithError(w, status, err.Error())
				return
			}
		}

		ctx := withClaims(r.Context(), claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// OptionalAuthMiddleware tries to extract user info but doesn't fail if token is missing.
// Used in development so local testing works both with and without a bearer token.
func OptionalAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if claims, viaGateway, err := gatewayClaims(r); viaGateway {
			if err != nil {
				respondWithError(w, http.StatusUnauthorized, err.Error())
				return
			}
			next.ServeHTTP(w, r.WithContext(withClaims(r.Context(), claims)))
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && parts[0] == "Bearer" {
				if claims, err := ValidateJWTContext(r.Context(), parts[1]); err == nil {
					if claims.fromJWKS {
						if _, err := attachComplex(r, claims); err != nil {
							next.ServeHTTP(w, r)
							return
						}
					}
					r = r.WithContext(withClaims(r.Context(), claims))
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func withClaims(ctx context.Context, claims *JWTClaims) context.Context {
	ctx = context.WithValue(ctx, UserIDKey, claims.UserID)
	ctx = context.WithValue(ctx, EmailKey, claims.Email)
	if claims.ComplexID != "" {
		ctx = context.WithValue(ctx, ComplexIDKey, claims.ComplexID)
	}
	if claims.Role != "" {
		ctx = context.WithValue(ctx, UserRoleKey, claims.Role)
	}
	return ctx
}

// ValidateJWT validates and parses a JWT token
func ValidateJWT(tokenString string) (*JWTClaims, error) {
	return ValidateJWTContext(context.Background(), tokenString)
}

// ValidateJWTContext accepts HS256 tokens (dev/tests, signed with JWT_SECRET) and,
// when ConfigureRS256 was called, RS256 tokens issued by the auth service.
func ValidateJWTContext(ctx context.Context, tokenString string) (*JWTClaims, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid token format")
	}

	// Only HS256 and RS256 are accepted, each verified with its own key material
	// (HMAC secret vs. JWKS public key), so "none" and RS256→HS256 key confusion
	// are closed before we ever touch the signature. typ, when present, must be JWT.
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("failed to decode header: %w", err)
	}
	var header struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("failed to parse header: %w", err)
	}
	if header.Typ != "" && header.Typ != "JWT" {
		return nil, errors.New("unsupported JWT type")
	}
	switch header.Alg {
	case "HS256":
	case "RS256":
		return validateRS256(ctx, parts, header.Kid)
	default:
		return nil, errors.New("unsupported JWT algorithm")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode payload: %w", err)
	}

	var claims JWTClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return nil, errors.New("JWT_SECRET not configured")
	}

	expectedSignature := generateSignature(parts[0]+"."+parts[1], jwtSecret)
	if !hmac.Equal([]byte(parts[2]), []byte(expectedSignature)) {
		return nil, errors.New("invalid signature")
	}

	if claims.ExpiresAt < time.Now().Unix() {
		return nil, errors.New("token expired")
	}

	return &claims, nil
}

// GenerateJWT creates a new JWT token for a user with default (24h) expiry
func GenerateJWT(userID, email, role, complexID string) (string, error) {
	return GenerateJWTWithExpiry(userID, email, role, complexID, 24*time.Hour)
}

// GenerateJWTWithExpiry creates a new JWT token for a user with custom expiry
func GenerateJWTWithExpiry(userID, email, role, complexID string, expiry time.Duration) (string, error) {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return "", errors.New("JWT_SECRET not configured")
	}

	now := time.Now()
	claims := JWTClaims{
		UserID:    userID,
		Email:     email,
		Role:      role,
		ComplexID: complexID,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(expiry).Unix(),
	}

	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	headerJSON, _ := json.Marshal(header)
	headerEncoded := base64.RawURLEncoding.EncodeToString(headerJSON)

	payloadJSON, _ := json.Marshal(claims)
	payloadEncoded := base64.RawURLEncoding.EncodeToString(payloadJSON)

	message := headerEncoded + "." + payloadEncoded
	signature := generateSignature(message, jwtSecret)

	return message + "." + signature, nil
}

func generateSignature(message, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(message))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// Context helper functions

func GetUserID(ctx context.Context) string {
	if userID, ok := ctx.Value(UserIDKey).(string); ok {
		return userID
	}
	return ""
}

func GetUserIDAsUUID(ctx context.Context) (uuid.UUID, error) {
	userID := GetUserID(ctx)
	if userID == "" {
		return uuid.Nil, errors.New("user ID not found in context")
	}
	return uuid.Parse(userID)
}

func GetComplexID(ctx context.Context) string {
	if complexID, ok := ctx.Value(ComplexIDKey).(string); ok {
		return complexID
	}
	return ""
}

func GetComplexIDAsUUID(ctx context.Context) (uuid.UUID, error) {
	complexID := GetComplexID(ctx)
	if complexID == "" {
		return uuid.Nil, errors.New("complex ID not found in context")
	}
	return uuid.Parse(complexID)
}

func GetUserRole(ctx context.Context) string {
	if role, ok := ctx.Value(UserRoleKey).(string); ok {
		return role
	}
	return ""
}

// RequireRoleMiddleware allows the request only if the JWT's role claim is
// one of allowedRoles. Must run after ApplyEnvironmentAuth so the role is
// already in context.
func RequireRoleMiddleware(allowedRoles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := GetUserRole(r.Context())
			for _, allowed := range allowedRoles {
				if role == allowed {
					next.ServeHTTP(w, r)
					return
				}
			}
			respondWithError(w, http.StatusForbidden, "your role does not have access to this operation")
		})
	}
}

// RequireStaffMiddleware allows the request only when the role claim is one of
// allowedRoles AND the user is registered as that role for the complex of this
// request. Must run after RequireComplexAccessMiddleware (it reads the
// resolved complex from the context).
func RequireStaffMiddleware(checker StaffChecker, allowedRoles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := GetUserRole(r.Context())
			allowed := false
			for _, a := range allowedRoles {
				if role == a {
					allowed = true
					break
				}
			}
			if !allowed {
				respondWithError(w, http.StatusForbidden, "your role does not have access to this operation")
				return
			}
			userID, err := GetUserIDAsUUID(r.Context())
			if err != nil {
				respondWithError(w, http.StatusUnauthorized, "authentication required")
				return
			}
			complexID, err := GetComplexIDAsUUID(r.Context())
			if err != nil {
				respondWithError(w, http.StatusBadRequest, "complex ID is required")
				return
			}
			ok, err := checker.IsStaffOfComplex(r.Context(), userID, complexID, role)
			if err != nil {
				respondWithError(w, http.StatusInternalServerError, "failed to verify staff access")
				return
			}
			if !ok {
				respondWithError(w, http.StatusForbidden, "you are not "+role+" of this complex")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// respondWithError writes the same {success,error:{code,message}} envelope the
// handlers use, so clients parse a single error shape.
func respondWithError(w http.ResponseWriter, status int, message string) {
	code := apperrors.ErrCodeInternalError
	switch status {
	case http.StatusBadRequest:
		code = apperrors.ErrCodeBadRequest
	case http.StatusUnauthorized:
		code = apperrors.ErrCodeAuthFailed
	case http.StatusForbidden:
		code = apperrors.ErrCodePermissionDenied
	case http.StatusNotFound:
		code = apperrors.ErrCodeResourceNotFound
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(apperrors.ErrorResponse{
		Success: false,
		Error:   apperrors.ErrorPayload{Code: code, Message: message},
	})
}

// ApplyEnvironmentAuth standardizes feature auth: production requires JWT
// auth, development keeps auth optional (but still validates a token if one
// is sent) so local testing is friction-free.
func ApplyEnvironmentAuth(r chi.Router) {
	if os.Getenv("ENV") != "development" {
		r.Use(AuthMiddleware)
		return
	}
	r.Use(OptionalAuthMiddleware)
}

// ComplexAccessChecker checks whether a user belongs to a complex.
type ComplexAccessChecker interface {
	IsUserAssociatedWithComplex(ctx context.Context, userID, complexID uuid.UUID) (bool, error)
}

// StaffChecker checks whether a user is staff (GUARD/ADMIN) of a complex.
type StaffChecker interface {
	IsStaffOfComplex(ctx context.Context, userID, complexID uuid.UUID, role string) (bool, error)
}

// AccessChecker is what the gate-side routes need: complex membership plus
// per-complex staff membership.
type AccessChecker interface {
	ComplexAccessChecker
	StaffChecker
}

// UnitAccessChecker checks whether a user belongs to a unit.
type UnitAccessChecker interface {
	IsUserAssociatedWithUnit(ctx context.Context, userID, unitID uuid.UUID) (bool, error)
}

func extractComplexID(r *http.Request) (uuid.UUID, error) {
	candidates := []string{
		chi.URLParam(r, "complexId"),
		chi.URLParam(r, "complex_id"),
		GetComplexID(r.Context()),
		r.Header.Get("X-Complex-ID"),
		r.URL.Query().Get("complex_id"),
		r.URL.Query().Get("complexId"),
	}

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		complexID, err := uuid.Parse(candidate)
		if err != nil {
			return uuid.Nil, errors.New("invalid complex ID format")
		}
		return complexID, nil
	}

	return uuid.Nil, errors.New("complex ID is required")
}

func extractUnitID(r *http.Request) (uuid.UUID, error) {
	candidates := []string{
		chi.URLParam(r, "unitId"),
		chi.URLParam(r, "unit_id"),
		r.URL.Query().Get("unit_id"),
		r.URL.Query().Get("unitId"),
	}

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		unitID, err := uuid.Parse(candidate)
		if err != nil {
			return uuid.Nil, errors.New("invalid unit ID format")
		}
		return unitID, nil
	}

	return uuid.Nil, errors.New("unit ID is required")
}

// RequireComplexAccessMiddleware creates a middleware that verifies the user belongs to the requested complex.
func RequireComplexAccessMiddleware(checker ComplexAccessChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := GetUserID(r.Context())
			if userID == "" {
				respondWithError(w, http.StatusUnauthorized, "authentication required")
				return
			}

			userUUID, err := uuid.Parse(userID)
			if err != nil {
				respondWithError(w, http.StatusUnauthorized, "invalid user ID")
				return
			}

			complexUUID, err := extractComplexID(r)
			if err != nil {
				respondWithError(w, http.StatusBadRequest, err.Error())
				return
			}

			hasAccess, err := checker.IsUserAssociatedWithComplex(r.Context(), userUUID, complexUUID)
			if err != nil {
				respondWithError(w, http.StatusInternalServerError, "failed to verify complex access")
				return
			}

			if !hasAccess {
				respondWithError(w, http.StatusForbidden, "you do not have access to this complex")
				return
			}

			ctx := context.WithValue(r.Context(), ComplexIDKey, complexUUID.String())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireUnitAccessMiddleware creates a middleware that verifies the user belongs to the requested unit.
func RequireUnitAccessMiddleware(checker UnitAccessChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userUUID, err := GetUserIDAsUUID(r.Context())
			if err != nil {
				respondWithError(w, http.StatusUnauthorized, "authentication required")
				return
			}

			unitUUID, err := extractUnitID(r)
			if err != nil {
				respondWithError(w, http.StatusBadRequest, err.Error())
				return
			}

			hasAccess, err := checker.IsUserAssociatedWithUnit(r.Context(), userUUID, unitUUID)
			if err != nil {
				respondWithError(w, http.StatusInternalServerError, "failed to verify unit access")
				return
			}

			if !hasAccess {
				respondWithError(w, http.StatusForbidden, "you do not have access to this unit")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
