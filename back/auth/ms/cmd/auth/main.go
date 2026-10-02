package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"superapp/auth/internal/application"
	"superapp/auth/internal/infrastructure/config"
	"superapp/auth/internal/infrastructure/ids"
	"superapp/auth/internal/infrastructure/postgres"
	"superapp/auth/internal/infrastructure/security"
	"superapp/auth/internal/presentation/httpapi"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	tokens, err := security.LoadTokens(cfg.JWTPrivateKeyFile, cfg.JWTIssuer, cfg.JWTAuthAudience, cfg.AccessTTL)
	if err != nil {
		log.Fatal(err)
	}
	service := application.New(postgres.New(db), security.Passwords{}, tokens, ids.UUID{}, application.Config{AccessTTL: cfg.AccessTTL, RefreshTTL: cfg.RefreshTTL})
	server := &http.Server{Addr: cfg.Addr, Handler: httpapi.New(service, cfg.GatewayTenantSecret), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}

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

	log.Printf("Auth MS listening on %s", cfg.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(fmt.Errorf("serve Auth MS: %w", err))
	}
}
