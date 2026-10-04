-- Entradas/salidas de RESIDENTES al edificio (conjunto). Usa la misma bitácora
-- access_logs que los visitantes, distinguiendo el sujeto:
--   VISITOR  → registrado por un guardia (como hasta ahora)
--   RESIDENT → el propio residente marca que entra/sale; nombre y unidad los
--              pone el servidor (no el cliente) y la hora es la del servidor.
-- El estado de un residente (dentro/fuera) es su último movimiento en ese conjunto.

ALTER TABLE access_logs ADD COLUMN IF NOT EXISTS subject_type text NOT NULL DEFAULT 'VISITOR';
ALTER TABLE access_logs ADD COLUMN IF NOT EXISTS subject_user_id uuid;

ALTER TABLE access_logs DROP CONSTRAINT IF EXISTS access_logs_subject_check;
ALTER TABLE access_logs ADD CONSTRAINT access_logs_subject_check CHECK (
    (subject_type = 'VISITOR')
    OR (subject_type = 'RESIDENT' AND subject_user_id IS NOT NULL
        AND movement IN ('ENTRY', 'EXIT') AND result = 'AUTHORIZED')
);

CREATE INDEX IF NOT EXISTS idx_access_logs_resident_last
    ON access_logs (complex_id, subject_user_id, occurred_at DESC)
    WHERE subject_type = 'RESIDENT';
