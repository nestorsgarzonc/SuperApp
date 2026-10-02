-- Multi-conjunto (SOLO ESQUEMA; los datos de demostración viven en db/seeds/001_demo.sql):
-- catálogo local de conjuntos y personal (guardias/administradores)
-- por conjunto. Son read-models de este servicio (sin FKs hacia otros servicios):
-- el dueño de la verdad es tenancy/auth, que los sincronizará.
--
-- Quién puede hacer qué en un conjunto:
--   · RESIDENT        → es miembro de una unidad de ese conjunto (unit_members)
--   · GUARD / ADMIN   → está en complex_staff para ese conjunto con ese rol
-- Una misma persona puede tener cualquier combinación en conjuntos distintos.

CREATE TABLE IF NOT EXISTS complexes (
    id uuid PRIMARY KEY,
    name text NOT NULL
);

CREATE TABLE IF NOT EXISTS complex_staff (
    complex_id uuid NOT NULL,
    user_id uuid NOT NULL,
    role text NOT NULL CHECK (role IN ('GUARD', 'ADMIN')),
    PRIMARY KEY (complex_id, user_id, role)
);

CREATE INDEX IF NOT EXISTS idx_complex_staff_user ON complex_staff (user_id);
