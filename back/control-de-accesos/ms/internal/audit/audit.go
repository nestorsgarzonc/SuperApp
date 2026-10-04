// Package audit writes to the append-only audit_log table. The table itself
// rejects UPDATE/DELETE via DB triggers (db/migrations/003_security_hardening.sql);
// this package only ever INSERTs.
package audit

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
)

const (
	ResultAllowed = "ALLOWED"
	ResultDenied  = "DENIED"
)

// Entry is one audit record. ActorID/ResourceID are pointers because some
// events have no authenticated actor or no single target resource yet.
type Entry struct {
	ID            uuid.UUID
	ActorID       *uuid.UUID
	ComplexID     *uuid.UUID
	ResourceType  string
	ResourceID    *uuid.UUID
	Action        string
	Result        string
	Reason        string
	IPAddress     string
	CorrelationID string
}

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Write(ctx context.Context, e Entry) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO audit_log (
			id, actor_id, complex_id, resource_type, resource_id,
			action, result, reason, ip_address, correlation_id
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`,
		e.ID, e.ActorID, e.ComplexID, e.ResourceType, e.ResourceID,
		e.Action, e.Result, e.Reason, e.IPAddress, e.CorrelationID,
	)
	return err
}
