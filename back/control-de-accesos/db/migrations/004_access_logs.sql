-- Bitácora de ingresos/salidas/intentos en portería. Append-only (sin
-- UPDATE/DELETE desde el servicio) e idempotente por (complex_id, client_id):
-- el front registra offline y reintenta la sincronización sin duplicar.
CREATE TABLE IF NOT EXISTS access_logs (
    id uuid PRIMARY KEY,
    client_id uuid NOT NULL,
    complex_id uuid NOT NULL,
    movement text NOT NULL CHECK (movement IN ('ENTRY', 'EXIT', 'ATTEMPT')),
    result text NOT NULL CHECK (result IN ('AUTHORIZED', 'DENIED')),
    visitor_name text NOT NULL,
    unit_label text NOT NULL,
    visitor_type text,
    vehicle text,
    grant_id uuid,
    registered_by uuid NOT NULL,
    occurred_at timestamptz NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (complex_id, client_id)
);

CREATE INDEX IF NOT EXISTS idx_access_logs_complex_occurred ON access_logs (complex_id, occurred_at DESC);
