package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"superapp/api-gateway/internal/authn"
	"superapp/api-gateway/internal/config"
	"superapp/api-gateway/internal/gateway"
	"superapp/api-gateway/internal/membership"
	"superapp/api-gateway/internal/ratelimit"
	"superapp/api-gateway/internal/revocation"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "probe /healthz of a running gateway and exit (for container HEALTHCHECK)")
	flag.Parse()
	if *healthcheck {
		os.Exit(probe())
	}

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

	verifier := authn.NewVerifier(authn.Config{
		JWKSURL: cfg.AuthJWKSURL, Issuer: cfg.AuthIssuer, Audience: cfg.AuthAudience,
		DevHS256Secret: cfg.DevHS256Secret,
	})
	go warmJWKS(ctx, verifier, logger)

	var limiter ratelimit.Limiter = ratelimit.NewMemory()
	var revoked gateway.RevocationChecker
	if cfg.RedisURL != "" {
		opts, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			return fmt.Errorf("REDIS_URL: %w", err)
		}
		rdb := redis.NewClient(opts)
		defer rdb.Close()
		limiter = ratelimit.NewRedis(rdb)
		revoked = revocation.NewRedis(rdb)
	} else {
		logger.Warn("REDIS_URL not set: rate limits are per instance and revoked tokens stay valid until they expire")
	}

	roles := membership.NewResolver(cfg.ControlDeAccesos.URL, cfg.InternalSecret, cfg.RoleCacheTTL, nil)
	gw := gateway.New(cfg, verifier, roles, revoked, limiter, logger)

	if cfg.IsDevelopment() {
		logger.Warn("ENV=development", "dev_hs256_tokens", cfg.DevHS256Secret != "", "dev_routes", cfg.ExposeDevRoutes)
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           gw,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       cfg.UpstreamTimeout + 10*time.Second,
		WriteTimeout:      cfg.UpstreamTimeout + 10*time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
	errCh := make(chan error, 1)
	go func() {
		logger.Info("api-gateway listening", "port", cfg.Port, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	select {
	case <-ctx.Done():
	case err := <-errCh:
		return err
	}
	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// warmJWKS loads auth's signing keys in the background, retrying while auth
// starts up, so the first login does not pay for the fetch.
func warmJWKS(ctx context.Context, v *authn.Verifier, logger *slog.Logger) {
	for delay := time.Second; ; delay = min(delay*2, 30*time.Second) {
		err := v.Warm(ctx)
		if err == nil {
			logger.Info("auth signing keys loaded")
			return
		}
		logger.Warn("auth signing keys not loaded yet", "error", err, "retry_in", delay.String())
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

func probe() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	c := &http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
