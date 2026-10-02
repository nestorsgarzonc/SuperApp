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
	usecases "superapp/auth/internal/core/usecases"
	"superapp/auth/internal/core/usecases/auth/login"
	"superapp/auth/internal/core/usecases/auth/logout"
	"superapp/auth/internal/core/usecases/auth/refresh"
	"superapp/auth/internal/core/usecases/auth/register"
	"superapp/auth/internal/core/usecases/contracts"
	database "superapp/auth/internal/infrastructure/persistence/postgres"
	pgadapter "superapp/auth/internal/infrastructure/repository/postgres"
	"superapp/auth/internal/infrastructure/security/jwt"
	"superapp/auth/internal/infrastructure/security/password"
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
	service := usecases.New(usecases.RepositoryDependencies{
		Users: repo, Sessions: repo, Roles: repo, Permissions: repo, Authorization: repo,
	}, uuid.NewString)
	services := httpshared.Services{
		Login:    login.New(repo, repo, password.Passwords{}, tokens, uuid.NewString, contracts.Config{AccessTTL: cfg.accessTTL, RefreshTTL: cfg.refreshTTL}),
		Register: register.New(repo, password.Passwords{}, uuid.NewString),
		Refresh:  refresh.New(repo, tokens, uuid.NewString, contracts.Config{AccessTTL: cfg.accessTTL, RefreshTTL: cfg.refreshTTL}),
		Logout:   logout.New(repo), Tokens: tokens,
		Users: service, Sessions: service,
		Roles: service, Permissions: service, Authorization: service,
	}
	server := &http.Server{Addr: cfg.addr, Handler: httpapi.New(services, cfg.gatewayTenantSecret), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}

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
