package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"controldeaccesos/internal/access"
	"controldeaccesos/internal/config"
	"controldeaccesos/internal/directory"
	"controldeaccesos/internal/notifier"
	"controldeaccesos/internal/platform/database"
	authmw "controldeaccesos/internal/platform/middleware"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

// Demo seed identities, used only by the development /dev/token endpoint.
// They must match SuperApp/back/control-de-accesos/db/migrations/002_seed.sql.
const (
	demoComplexID = "00000000-0000-0000-0000-000000000001"
	demoUnitID    = "10000000-0000-0000-0000-000000000101"
	demoUserID    = "10000000-0000-0000-0000-000000000001"
	demoUserEmail = "resident1@demo.com"
	demoUserRole  = "RESIDENT"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := config.Load()

	if cfg.JWTSecret == "" {
		logger.Error("JWT_SECRET is required")
		os.Exit(1)
	}
	// ValidateJWT/GenerateJWT read the secret straight from the environment.
	os.Setenv("JWT_SECRET", cfg.JWTSecret)
	os.Setenv("ENV", cfg.Env)

	dbPort, err := strconv.Atoi(cfg.DBPort)
	if err != nil {
		dbPort = 5432
	}

	db, err := database.NewPostgresClient(database.PostgresConfig{
		Host:            cfg.DBHost,
		Port:            dbPort,
		User:            cfg.DBUser,
		Password:        cfg.DBPassword,
		Database:        cfg.DBName,
		SSLMode:         cfg.DBSSLMode,
		SSLRootCert:     cfg.DBSSLRootCert,
		MaxOpenConns:    cfg.DBMaxOpenConns,
		MaxIdleConns:    cfg.DBMaxIdleConns,
		ConnMaxLifetime: time.Duration(cfg.DBConnMaxLifetimeMins) * time.Minute,
	})
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	dir := directory.NewRepository(db.DB)
	accessRepo := access.NewPostgresRepository(db.DB)
	// notifierClient is a no-op until NOTIFICATIONS_URL points at a running
	// notificaciones microservice — see internal/notifier.
	notifierClient := notifier.New(cfg.NotificationsURL, logger)
	accessService := access.NewService(accessRepo, dir, dir, notifierClient, logger)
	accessHandler := access.NewHandler(accessService)

	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := db.DB.PingContext(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("db unreachable"))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	r.Route("/api/v1", func(r chi.Router) {
		access.RegisterRoutes(r, accessHandler, dir)

		r.Route("/units", func(r chi.Router) {
			access.RegisterUnitRoutes(r, accessHandler, dir)
		})

		if cfg.Env == "development" {
			r.Get("/dev/token", devTokenHandler)
		}
	})

	logger.Info("control-de-accesos listening", "port", cfg.Port, "env", cfg.Env)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

// devTokenHandler issues a bearer token for the seeded demo resident, so the
// service can be exercised end-to-end locally without a separate auth
// microservice. Only mounted when ENV=development.
func devTokenHandler(w http.ResponseWriter, r *http.Request) {
	token, err := authmw.GenerateJWT(demoUserID, demoUserEmail, demoUserRole, demoComplexID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"token":      token,
		"complex_id": demoComplexID,
		"unit_id":    demoUnitID,
		"user_id":    demoUserID,
	})
}
