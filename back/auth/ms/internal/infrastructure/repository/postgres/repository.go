package postgres

import (
	"context"
	"database/sql"
)

// RevocationSink is told which sessions were revoked, after the revocation is
// committed, so validators that check access tokens locally (the api-gateway)
// can reject them before they expire. It must not fail the caller: the
// database stays the source of truth.
type RevocationSink interface {
	SessionsRevoked(ctx context.Context, sessionIDs []string)
}

type Repository struct {
	db      *sql.DB
	revoked RevocationSink
}

func New(db *sql.DB) *Repository { return &Repository{db: db} }

// WithRevocationSink returns a repository that reports revoked sessions to sink.
func (r *Repository) WithRevocationSink(sink RevocationSink) *Repository {
	return &Repository{db: r.db, revoked: sink}
}

func (r *Repository) publishRevoked(ctx context.Context, sessionIDs []string) {
	if r.revoked != nil && len(sessionIDs) > 0 {
		r.revoked.SessionsRevoked(context.WithoutCancel(ctx), sessionIDs)
	}
}

// queryIDs runs a statement that RETURNs one id column.
func queryIDs(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]string, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
