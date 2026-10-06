-- Auth owns these tables. Tenant IDs are opaque references; no cross-service FKs.
CREATE TABLE auth_users (
    id UUID PRIMARY KEY,
    email TEXT NOT NULL,
    email_normalized TEXT NOT NULL UNIQUE,
    full_name TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'DISABLED', 'LOCKED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at TIMESTAMPTZ
);

CREATE TABLE auth_sessions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
    device_name TEXT,
    user_agent TEXT,
    ip_address INET,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ
);
CREATE INDEX auth_sessions_user_active_idx ON auth_sessions(user_id, expires_at) WHERE revoked_at IS NULL;

CREATE TABLE auth_refresh_tokens (
    id UUID PRIMARY KEY,
    session_id UUID NOT NULL REFERENCES auth_sessions(id) ON DELETE CASCADE,
    token_hash BYTEA NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    replaced_by UUID REFERENCES auth_refresh_tokens(id)
);
CREATE INDEX auth_refresh_tokens_session_idx ON auth_refresh_tokens(session_id);

CREATE TABLE auth_roles (
    id UUID PRIMARY KEY,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    scope TEXT NOT NULL CHECK (scope IN ('SYSTEM', 'TENANT')),
    tenant_id UUID,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((scope = 'SYSTEM' AND tenant_id IS NULL) OR (scope = 'TENANT' AND tenant_id IS NOT NULL))
);
CREATE UNIQUE INDEX auth_roles_system_code_uq ON auth_roles(code) WHERE scope = 'SYSTEM';
CREATE UNIQUE INDEX auth_roles_tenant_code_uq ON auth_roles(tenant_id, code) WHERE scope = 'TENANT';

CREATE TABLE auth_permissions (
    id UUID PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE auth_role_permissions (
    role_id UUID NOT NULL REFERENCES auth_roles(id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES auth_permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE auth_user_roles (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES auth_roles(id) ON DELETE CASCADE,
    tenant_id UUID,
    assigned_by UUID REFERENCES auth_users(id) ON DELETE SET NULL,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX auth_user_roles_active_global_uq
    ON auth_user_roles(user_id, role_id) WHERE tenant_id IS NULL AND revoked_at IS NULL;
CREATE UNIQUE INDEX auth_user_roles_active_tenant_uq
    ON auth_user_roles(user_id, role_id, tenant_id) WHERE tenant_id IS NOT NULL AND revoked_at IS NULL;
CREATE INDEX auth_user_roles_user_tenant_idx ON auth_user_roles(user_id, tenant_id) WHERE revoked_at IS NULL;
