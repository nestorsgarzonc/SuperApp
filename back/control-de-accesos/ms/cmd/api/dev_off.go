//go:build !dev

package main

import (
	"database/sql"

	"controldeaccesos/internal/qrsystem"

	"github.com/go-chi/chi/v5"
)

// Normal build: no dev tooling is compiled in, whatever ENV says.

func newQRClient() qrsystem.Client { return qrsystem.Local{} }

func mountDev(chi.Router, *sql.DB, *qrsystem.System) bool { return false }
