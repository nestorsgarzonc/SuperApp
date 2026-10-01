-- Control de accesos: visitantes (personas) y vehículos de visitantes.
--
-- Base propia de este módulo. No recrea el universo del monolito
-- (complejos, unidades, usuarios, vehículos de residentes, amenidades, etc.).
--
-- Cómo se crea en el proyecto de referencia:
--   docker-compose.yml monta el esquema en /docker-entrypoint-initdb.d
--   (backend/migrations/000_initial_schema.sql y luego docker/postgres/seed.sql).
-- Este archivo es el equivalente acotado: PostgreSQL 18 con uuidv7(),
-- igual que la imagen postgres:18.6-alpine del compose principal.
--
-- Integración con otros módulos:
--   ref_* es una proyección local. El dueño de complejos, unidades y usuarios
--   hace upsert aquí (mismos UUID que docker/postgres/seed.sql en el demo).
--   Las tablas de negocio referencian esa proyección, no las tablas del monolito.

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;

-- ----------------------------------------------------------
-- Proyección de integración (otros módulos son la fuente)
-- ----------------------------------------------------------

CREATE TABLE IF NOT EXISTS ref_complexes (
    id         UUID PRIMARY KEY,
    name       TEXT        NOT NULL,
    address    TEXT,
    timezone   TEXT        NOT NULL DEFAULT 'America/Bogota',
    is_active  BOOLEAN     NOT NULL DEFAULT TRUE
);

CREATE TABLE IF NOT EXISTS ref_sections (
    id         UUID PRIMARY KEY,
    complex_id UUID        NOT NULL REFERENCES ref_complexes(id) ON DELETE CASCADE,
    name       TEXT        NOT NULL
);

CREATE TABLE IF NOT EXISTS ref_units (
    id         UUID PRIMARY KEY,
    complex_id UUID        NOT NULL REFERENCES ref_complexes(id) ON DELETE CASCADE,
    section_id UUID        REFERENCES ref_sections(id) ON DELETE SET NULL,
    name       TEXT        NOT NULL,
    sort_order INT         NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS ref_users (
    id         UUID PRIMARY KEY,
    full_name  TEXT        NOT NULL,
    email      TEXT,
    status     TEXT        NOT NULL DEFAULT 'ACTIVE'
);

-- Equivale a complex_admins (rol GUARD / MANAGER) del seed del monolito.
CREATE TABLE IF NOT EXISTS ref_complex_staff (
    complex_id UUID        NOT NULL REFERENCES ref_complexes(id) ON DELETE CASCADE,
    user_id    UUID        NOT NULL REFERENCES ref_users(id) ON DELETE CASCADE,
    role       TEXT        NOT NULL CHECK (role IN ('GUARD', 'MANAGER')),
    PRIMARY KEY (complex_id, user_id)
);

-- Equivale a unit_users. Sirve el control IDOR: solo el residente de la unidad
-- crea, lista, revoca o extiende pases y preautorizaciones de esa unidad.
CREATE TABLE IF NOT EXISTS ref_unit_members (
    complex_id UUID        NOT NULL REFERENCES ref_complexes(id) ON DELETE CASCADE,
    unit_id    UUID        NOT NULL REFERENCES ref_units(id) ON DELETE CASCADE,
    user_id    UUID        NOT NULL REFERENCES ref_users(id) ON DELETE CASCADE,
    role       TEXT        NOT NULL,
    is_primary BOOLEAN     NOT NULL DEFAULT FALSE,
    PRIMARY KEY (unit_id, user_id)
);

-- Avisos que el monolito enviaba por el seam de notificaciones
-- (visitorpass.NotificationSender). Otro módulo de comunicación puede leerlas.
CREATE TABLE IF NOT EXISTS integration_notifications (
    id         UUID        PRIMARY KEY DEFAULT uuidv7(),
    complex_id UUID        NOT NULL REFERENCES ref_complexes(id) ON DELETE CASCADE,
    user_id    UUID        NOT NULL REFERENCES ref_users(id) ON DELETE CASCADE,
    title      TEXT        NOT NULL,
    body       TEXT        NOT NULL,
    type       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ----------------------------------------------------------
-- Personas: pase QR de visitante
-- Origen: migrations 000029_visitor_passes + 000059_visitor_pass_checkout
-- ----------------------------------------------------------

CREATE TABLE IF NOT EXISTS visitor_passes (
    id             UUID        PRIMARY KEY DEFAULT uuidv7(),
    complex_id     UUID        NOT NULL REFERENCES ref_complexes(id) ON DELETE CASCADE,
    unit_id        UUID        NOT NULL REFERENCES ref_units(id) ON DELETE CASCADE,
    host_user_id   UUID        NOT NULL REFERENCES ref_users(id) ON DELETE CASCADE,
    visitor_name   TEXT        NOT NULL,
    document       TEXT,
    token          TEXT        NOT NULL,
    valid_from     TIMESTAMPTZ NOT NULL,
    valid_until    TIMESTAMPTZ NOT NULL,
    status         TEXT        NOT NULL DEFAULT 'active'
                   CHECK (status IN ('active', 'used', 'expired', 'revoked', 'checked_out')),
    used_by        UUID        REFERENCES ref_users(id) ON DELETE SET NULL,
    used_at        TIMESTAMPTZ,
    checked_out_by UUID        REFERENCES ref_users(id) ON DELETE SET NULL,
    checked_out_at TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT visitor_passes_valid_window CHECK (valid_until > valid_from)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_visitor_passes_token
    ON visitor_passes (token);

CREATE INDEX IF NOT EXISTS idx_visitor_passes_unit_created
    ON visitor_passes (unit_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_visitor_passes_complex_status
    ON visitor_passes (complex_id, status, valid_until);

CREATE INDEX IF NOT EXISTS idx_visitor_passes_active_expiry
    ON visitor_passes (status, valid_until)
    WHERE status = 'active';

DROP TRIGGER IF EXISTS trigger_visitor_passes_updated_at ON visitor_passes;
CREATE TRIGGER trigger_visitor_passes_updated_at
    BEFORE UPDATE ON visitor_passes
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- ----------------------------------------------------------
-- Vehículos de visitantes
-- Origen: 000055_visitor_vehicle_preauths + access log de 000012 / 000058.
-- No incluye resident_vehicles: ese inventario pertenece a otro módulo.
-- ----------------------------------------------------------

CREATE TABLE IF NOT EXISTS visitor_vehicle_preauths (
    id           UUID        PRIMARY KEY DEFAULT uuidv7(),
    complex_id   UUID        NOT NULL REFERENCES ref_complexes(id) ON DELETE CASCADE,
    unit_id      UUID        NOT NULL REFERENCES ref_units(id) ON DELETE CASCADE,
    host_user_id UUID        NOT NULL REFERENCES ref_users(id) ON DELETE CASCADE,
    plate        TEXT        NOT NULL,
    visitor_name TEXT        NOT NULL,
    valid_from   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    valid_until  TIMESTAMPTZ NOT NULL,
    status       TEXT        NOT NULL DEFAULT 'active'
                 CHECK (status IN ('active', 'revoked', 'expired')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_visitor_vehicle_preauths_plate
    ON visitor_vehicle_preauths (complex_id, plate)
    WHERE status = 'active';

CREATE INDEX IF NOT EXISTS idx_visitor_vehicle_preauths_host
    ON visitor_vehicle_preauths (complex_id, host_user_id);

-- Bitácora de entrada/salida de placas de visitantes.
-- En el monolito es vehicle_access_log.preauth_id; aquí solo viven
-- los eventos de este módulo (vehicle_id de residentes queda fuera).
CREATE TABLE IF NOT EXISTS visitor_vehicle_access_log (
    id         UUID        PRIMARY KEY DEFAULT uuidv7(),
    complex_id UUID        NOT NULL REFERENCES ref_complexes(id) ON DELETE CASCADE,
    plate      TEXT        NOT NULL,
    preauth_id UUID        REFERENCES visitor_vehicle_preauths(id) ON DELETE SET NULL,
    direction  TEXT        NOT NULL CHECK (direction IN ('in', 'out')),
    guard_id   UUID        NOT NULL REFERENCES ref_users(id),
    note       TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_visitor_vehicle_access_log_complex
    ON visitor_vehicle_access_log (complex_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_visitor_vehicle_access_log_plate
    ON visitor_vehicle_access_log (complex_id, plate, created_at DESC);
