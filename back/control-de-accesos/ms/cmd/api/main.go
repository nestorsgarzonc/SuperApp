package main

import (
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"controldeaccesos/internal/access"
	"controldeaccesos/internal/accesslog"
	"controldeaccesos/internal/audit"
	"controldeaccesos/internal/config"
	"controldeaccesos/internal/directory"
	"controldeaccesos/internal/membership"
	"controldeaccesos/internal/notifier"
	"controldeaccesos/internal/platform/database"
	authmw "controldeaccesos/internal/platform/middleware"
	"controldeaccesos/internal/qrsystem"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := config.Load()

	// A caller is authenticated either by a JWT or by the identity the
	// gateway forwards; at least one way must be configured.
	if cfg.JWTSecret == "" && cfg.GatewaySecret == "" && cfg.AuthJWKSURL == "" {
		logger.Error("JWT_SECRET, GATEWAY_SECRET or AUTH_JWKS_URL is required")
		os.Exit(1)
	}
	if err := authmw.ConfigureRS256(authmw.RS256Config{JWKSURL: cfg.AuthJWKSURL, Issuer: cfg.AuthIssuer, Audience: cfg.AuthAudience}); err != nil {
		logger.Error("invalid auth token configuration", "error", err)
		os.Exit(1)
	}
	// The auth middleware reads these straight from the environment.
	os.Setenv("JWT_SECRET", cfg.JWTSecret)
	os.Setenv("GATEWAY_SECRET", cfg.GatewaySecret)
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
	authmw.SetComplexRoleResolver(dir.GetUserComplexRole)
	accessRepo := access.NewPostgresRepository(db.DB)
	auditRepo := audit.NewRepository(db.DB)
	notifierClient, err := notifier.New(cfg.RedisURL, cfg.NotificationsStream, logger)
	if err != nil {
		logger.Error("invalid REDIS_URL", "error", err)
		os.Exit(1)
	}
	defer notifierClient.Close()
	// QR system behind a circuit breaker. In development it is simulated so its
	// failure can be provoked from the dev tools; elsewhere codes are issued locally.
	qrSys := qrsystem.New(newQRClient(), qrsystem.Config{
		Failures:      cfg.QRBreakerFailures,
		OpenFor:       time.Duration(cfg.QRBreakerOpenSeconds) * time.Second,
		CallTimeout:   time.Duration(cfg.QRCallTimeoutMs) * time.Millisecond,
		MaxConcurrent: cfg.QRMaxConcurrent,
	}, logger)
	accessService := access.NewService(accessRepo, dir, dir, notifierClient, auditRepo, qrSys, logger)
	accessHandler := access.NewHandler(accessService)
	accessLogHandler := accesslog.NewHandler(accesslog.NewPostgresRepository(db.DB), auditRepo, logger)
	membershipHandler := membership.NewHandler(dir)

	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(authmw.WithRequestMeta)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := db.DB.PingContext(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("db unreachable"))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Service-to-service endpoints (never routed by the gateway).
	membership.RegisterInternalRoutes(r, membershipHandler, cfg.InternalSecret)

	r.Route("/api/v1", func(r chi.Router) {
		membership.RegisterRoutes(r, membershipHandler)
		access.RegisterRoutes(r, accessHandler, dir)
		accesslog.RegisterRoutes(r, accessLogHandler, dir)
		accesslog.RegisterResidentRoutes(r, accessLogHandler, dir)

		r.Route("/units", func(r chi.Router) {
			access.RegisterUnitRoutes(r, accessHandler, dir)
		})

		// Dev tooling exists only in binaries built with `-tags dev` AND started
		// with ENV=development; a normal build has no /dev routes at all.
		if cfg.Env == "development" {
			if mountDev(r, db.DB, qrSys) {
				logger.Warn("development tooling enabled (/dev/*): never expose this build")
			}
		}
	})

	logger.Info("control-de-accesos listening", "port", cfg.Port, "env", cfg.Env)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
