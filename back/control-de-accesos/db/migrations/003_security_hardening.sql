-- Audit trail: append-only (DB-enforced via triggers, not just app discipline).
CREATE TABLE IF NOT EXISTS audit_log (
    id uuid PRIMARY KEY,
    actor_id uuid,
    complex_id uuid,
    resource_type text NOT NULL,
    resource_id uuid,
    action text NOT NULL,
    result text NOT NULL CHECK (result IN ('ALLOWED', 'DENIED')),
    reason text,
    ip_address text,
    correlation_id text,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_audit_log_complex_id ON audit_log (complex_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_created_at ON audit_log (created_at);

CREATE OR REPLACE FUNCTION prevent_audit_log_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'audit_log is append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS audit_log_no_update ON audit_log;
CREATE TRIGGER audit_log_no_update BEFORE UPDATE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION prevent_audit_log_mutation();

DROP TRIGGER IF EXISTS audit_log_no_delete ON audit_log;
CREATE TRIGGER audit_log_no_delete BEFORE DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION prevent_audit_log_mutation();

-- Single-use consumption: every grant is single-use, marked USED on the
-- first successful validation (see internal/access TryMarkGrantUsed).
ALTER TABLE access_grants ADD COLUMN IF NOT EXISTS single_use boolean NOT NULL DEFAULT true;
