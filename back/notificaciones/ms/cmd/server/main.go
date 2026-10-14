package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	_ "github.com/lib/pq"

	"superapp/notificaciones/internal/config"
	"superapp/notificaciones/internal/features/devices"
	"superapp/notificaciones/internal/features/notifications"
	"superapp/notificaciones/internal/features/preferences"
	"superapp/notificaciones/internal/platform/apperr"
	"superapp/notificaciones/internal/platform/cors"
	"superapp/notificaciones/internal/platform/gatewayauth"
	"superapp/notificaciones/internal/platform/internalauth"
	"superapp/notificaciones/internal/platform/push"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	// La base de RDS es compartida por todos los ms: pool pequeño y configurable.
	db.SetMaxOpenConns(cfg.DBMaxOpenConns)
	db.SetMaxIdleConns(cfg.DBMaxIdleConns)
	if err := db.PingContext(ctx); err != nil {
		return err
	}

	var pusher push.Sender
	if cfg.FirebaseCredentials != "" {
		fcm, err := push.NewFCM(ctx, cfg.FirebaseCredentials, cfg.FirebaseProjectID, cfg.WebAppURL)
		if err != nil {
			return err
		}
		pusher = fcm
	} else {
		logger.Warn("FIREBASE_CREDENTIALS_FILE not set: push disabled, inbox only")
	}

	prefsRepo := preferences.NewPostgresRepository(db)
	devicesRepo := devices.NewPostgresRepository(db)
	notifSvc := notifications.NewService(
		notifications.NewPostgresRepository(db), devicesRepo, pusher,
		preferences.NewChecker(prefsRepo), logger,
	)

	r := chi.NewRouter()
	r.Use(cors.Middleware, chimw.RequestID, chimw.RealIP, chimw.Recoverer, chimw.Timeout(60*time.Second))
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := db.PingContext(r.Context()); err != nil {
			apperr.RespondError(w, logger, apperr.Unavailable("database unavailable", err))
			return
		}
		apperr.RespondOK(w, map[string]string{"status": "ok"})
	})
	notifHandler := notifications.NewHandler(notifSvc, logger)
	r.Group(func(r chi.Router) {
		r.Use(internalauth.Middleware(cfg.InternalSecret))
		notifications.RegisterInternalRoutes(r, notifHandler)
	})
	r.Group(func(r chi.Router) {
		r.Use(gatewayauth.Middleware(cfg.GatewaySecret))
		notifications.RegisterRoutes(r, notifHandler)
		preferences.RegisterRoutes(r, preferences.NewHandler(preferences.NewService(prefsRepo), logger))
		devices.RegisterRoutes(r, devices.NewHandler(devices.NewService(devicesRepo), logger))
	})

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: r, ReadHeaderTimeout: 10 * time.Second}
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case <-ctx.Done():
	case err := <-serverErr:
		return err
	}

	logger.Info("shutting down")
	stop()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err = srv.Shutdown(shutdownCtx)

	notifSvc.Wait()
	return err
}
