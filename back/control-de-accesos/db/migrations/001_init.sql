-- control-de-accesos: owns only its own data. No foreign keys into other
-- services' databases (units/users below are this service's local directory
-- read-model, not the tenancy/identity microservices' tables).

CREATE TABLE IF NOT EXISTS units (
    id uuid PRIMARY KEY,
    complex_id uuid NOT NULL,
    name text NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_units_complex_id ON units (complex_id);

CREATE TABLE IF NOT EXISTS users (
    id uuid PRIMARY KEY,
    name text NOT NULL,
    phone text
);

CREATE TABLE IF NOT EXISTS unit_members (
    unit_id uuid NOT NULL REFERENCES units(id) ON DELETE CASCADE,
    user_id uuid NOT NULL,
    PRIMARY KEY (unit_id, user_id)
);

CREATE TABLE IF NOT EXISTS access_grants (
    id uuid PRIMARY KEY,
    complex_id uuid NOT NULL,
    unit_id uuid NOT NULL,
    created_by uuid NOT NULL,
    method text NOT NULL,
    visitor_type text NOT NULL,
    visitor_name text NOT NULL,
    visitor_id_number text,
    qr_code_hash text,
    details text,
    status text NOT NULL DEFAULT 'ACTIVE',
    valid_from timestamptz DEFAULT now(),
    valid_until timestamptz NOT NULL,
    used_at timestamptz,
    used_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT access_grants_method_check CHECK (method IN ('QR', 'PRE_AUTH')),
    CONSTRAINT access_grants_status_check CHECK (status IN ('ACTIVE', 'USED', 'EXPIRED', 'CANCELLED')),
    CONSTRAINT access_grants_visitor_type_check CHECK (visitor_type IN ('DELIVERY', 'VISITOR', 'SERVICE', 'CONTRACTOR', 'GUEST')),
    CONSTRAINT qr_required_if_method_qr CHECK ((method = 'QR' AND qr_code_hash IS NOT NULL) OR method <> 'QR')
);

CREATE INDEX IF NOT EXISTS idx_access_grants_complex_id ON access_grants (complex_id);
CREATE INDEX IF NOT EXISTS idx_access_grants_unit_id ON access_grants (unit_id);
CREATE INDEX IF NOT EXISTS idx_access_grants_qr_code_hash ON access_grants (qr_code_hash);
CREATE INDEX IF NOT EXISTS idx_access_grants_visitor_id_number ON access_grants (visitor_id_number);
