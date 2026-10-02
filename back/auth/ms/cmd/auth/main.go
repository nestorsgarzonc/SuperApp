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

	"superapp/auth/internal/application"
	"superapp/auth/internal/infrastructure/ids"
	"superapp/auth/internal/infrastructure/persistence/postgres"
	"superapp/auth/internal/infrastructure/security"
	httpapi "superapp/auth/internal/presentation/http/server"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("pgx", cfg.databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	tokens, err := security.LoadTokens(cfg.jwtPrivateKeyFile, cfg.jwtIssuer, cfg.jwtAuthAudience, cfg.accessTTL)
	if err != nil {
		log.Fatal(err)
	}
	service := application.New(postgres.New(db), security.Passwords{}, tokens, ids.UUID{}, application.Config{AccessTTL: cfg.accessTTL, RefreshTTL: cfg.refreshTTL})
	server := &http.Server{Addr: cfg.addr, Handler: httpapi.New(service, cfg.gatewayTenantSecret), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}

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
