package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	httpapi "superapp/auth/internal"
	"superapp/auth/internal/core/usecases/auth/login"
	"superapp/auth/internal/core/usecases/auth/logout"
	"superapp/auth/internal/core/usecases/auth/refresh"
	"superapp/auth/internal/core/usecases/auth/register"
	"superapp/auth/internal/core/usecases/authorization"
	"superapp/auth/internal/core/usecases/contracts"
	"superapp/auth/internal/core/usecases/permissions"
	"superapp/auth/internal/core/usecases/roles"
	"superapp/auth/internal/core/usecases/sessions"
	"superapp/auth/internal/core/usecases/users"
	database "superapp/auth/internal/infrastructure/persistence/postgres"
	pgadapter "superapp/auth/internal/infrastructure/repository/postgres"
	"superapp/auth/internal/infrastructure/security/jwt"
	"superapp/auth/internal/infrastructure/security/password"
	"superapp/auth/internal/presentation/cors"
	httpshared "superapp/auth/internal/presentation/shared"

	"github.com/google/uuid"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}
	db, err := database.Open(cfg.databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	tokens, err := jwt.LoadTokens(cfg.jwtPrivateKeyFile, cfg.jwtIssuer, cfg.jwtAuthAudience, cfg.accessTTL)
	if err != nil {
		log.Fatal(err)
	}
	repo := pgadapter.New(db)
	services := httpshared.Services{
		Login:    login.New(repo, repo, password.Passwords{}, tokens, uuid.NewString, contracts.Config{AccessTTL: cfg.accessTTL, RefreshTTL: cfg.refreshTTL}),
		Register: register.New(repo, password.Passwords{}, uuid.NewString),
		Refresh:  refresh.New(repo, tokens, uuid.NewString, contracts.Config{AccessTTL: cfg.accessTTL, RefreshTTL: cfg.refreshTTL}),
		Logout:   logout.New(repo), TokenVerifier: tokens, JWKS: tokens,
		Users: users.New(repo), Sessions: sessions.New(repo),
		Roles: roles.New(repo, uuid.NewString), Permissions: permissions.New(repo, uuid.NewString),
		Authorization: authorization.New(repo),
	}
	server := &http.Server{Addr: cfg.addr, Handler: cors.Middleware(httpapi.New(services, cfg.gatewayTenantSecret)), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP shutdown: %v", err)
		}
	}()

	log.Printf("Auth MS listening on %s", cfg.addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(fmt.Errorf("serve Auth MS: %w", err))
	}
}
