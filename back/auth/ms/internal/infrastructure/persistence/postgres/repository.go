package postgres

import (
	"context"
	"database/sql"
)

type Repository struct{ db *sql.DB }

func New(db *sql.DB) *Repository                     { return &Repository{db: db} }
func (r *Repository) Ping(ctx context.Context) error { return r.db.PingContext(ctx) }
