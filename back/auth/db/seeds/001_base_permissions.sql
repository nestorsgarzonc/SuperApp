-- Idempotent permission catalog. No users, passwords, tokens, or default admin.
INSERT INTO auth_permissions (id, code, description) VALUES
('00000000-0000-4000-8000-000000000001', 'auth.users.read', 'Consultar usuarios'),
('00000000-0000-4000-8000-000000000002', 'auth.users.manage', 'Administrar usuarios y estados'),
('00000000-0000-4000-8000-000000000003', 'auth.roles.read', 'Consultar roles y permisos'),
('00000000-0000-4000-8000-000000000004', 'auth.roles.manage', 'Administrar roles y permisos')
ON CONFLICT (code) DO NOTHING;

INSERT INTO auth_roles(id, code, name, description, scope, enabled) VALUES
('00000000-0000-4000-8000-000000000101', 'AUTH_PLATFORM_ADMIN', 'Auth Platform Administrator', 'Bootstrap role for managing identity, roles, permissions, and sessions', 'SYSTEM', TRUE)
ON CONFLICT (code) WHERE scope = 'SYSTEM' DO NOTHING;

INSERT INTO auth_role_permissions(role_id, permission_id)
SELECT r.id, p.id FROM auth_roles r CROSS JOIN auth_permissions p
WHERE r.scope = 'SYSTEM' AND r.code = 'AUTH_PLATFORM_ADMIN'
ON CONFLICT DO NOTHING;
