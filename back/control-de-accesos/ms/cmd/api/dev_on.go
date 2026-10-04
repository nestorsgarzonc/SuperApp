//go:build dev

package main

import (
	"database/sql"

	"controldeaccesos/internal/devtools"
	"controldeaccesos/internal/qrsystem"

	"github.com/go-chi/chi/v5"
)

// Development build: the QR system is simulated and /dev/* is mounted.

func newQRClient() qrsystem.Client { return &qrsystem.Simulator{} }

func mountDev(r chi.Router, db *sql.DB, qrSys *qrsystem.System) bool {
	dev := devtools.NewHandler(db, qrSys)
	r.Get("/dev/users", dev.Users)
	r.Get("/dev/token", dev.Token)
	r.Get("/dev/qr-system", dev.QRSystem)
	r.Put("/dev/qr-system", dev.SetQRSystem)
	r.Post("/dev/qr-system/reset", dev.ResetQRBreaker)
	return true
}
