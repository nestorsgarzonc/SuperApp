-- ═══════════════════════════════════════════════════════════════════════════
-- DATOS DE DEMOSTRACIÓN — SOLO DESARROLLO / PRUEBAS. NUNCA EN PRODUCCIÓN.
--
-- Espejo en Auth de las personas de control-de-accesos/db/seeds/001_demo.sql:
-- mismos UUID de usuario, y cada conjunto es un tenant (tenant_id = complex_id).
-- Por eso NO está en db/seeds (cmd/setup aplica esa carpeta a cualquier base).
--
--   psql "$AUTH_DATABASE_URL" -f db/dev-seeds/001_demo.sql
--
-- Contraseña de todos: Demo-Pass-2026!   (bcrypt cost 12)
-- Idempotente: se puede ejecutar varias veces.
-- ═══════════════════════════════════════════════════════════════════════════

INSERT INTO auth_users (id, email, email_normalized, full_name, password_hash) VALUES
    ('10000000-0000-0000-0000-000000000001', 'ana.garcia@demo.test',   'ana.garcia@demo.test',   'Ana García',   '$2a$12$7afRjHVKCoUz4Bj57Q54TOR5O7YdI32rFArSuD2RyPBXoe1FY7MVK'),
    ('10000000-0000-0000-0000-000000000002', 'pedro.gomez@demo.test',  'pedro.gomez@demo.test',  'Pedro Gómez',  '$2a$12$7afRjHVKCoUz4Bj57Q54TOR5O7YdI32rFArSuD2RyPBXoe1FY7MVK'),
    ('10000000-0000-0000-0000-000000000003', 'luis.mora@demo.test',    'luis.mora@demo.test',    'Luis Mora',    '$2a$12$7afRjHVKCoUz4Bj57Q54TOR5O7YdI32rFArSuD2RyPBXoe1FY7MVK'),
    ('10000000-0000-0000-0000-000000000004', 'marta.ruiz@demo.test',   'marta.ruiz@demo.test',   'Marta Ruiz',   '$2a$12$7afRjHVKCoUz4Bj57Q54TOR5O7YdI32rFArSuD2RyPBXoe1FY7MVK'),
    ('10000000-0000-0000-0000-000000000005', 'carlos.rojas@demo.test', 'carlos.rojas@demo.test', 'Carlos Rojas', '$2a$12$7afRjHVKCoUz4Bj57Q54TOR5O7YdI32rFArSuD2RyPBXoe1FY7MVK'),
    ('10000000-0000-0000-0000-000000000006', 'sofia.diaz@demo.test',   'sofia.diaz@demo.test',   'Sofía Díaz',   '$2a$12$7afRjHVKCoUz4Bj57Q54TOR5O7YdI32rFArSuD2RyPBXoe1FY7MVK'),
    ('10000000-0000-0000-0000-000000000007', 'andres.vega@demo.test',  'andres.vega@demo.test',  'Andrés Vega',  '$2a$12$7afRjHVKCoUz4Bj57Q54TOR5O7YdI32rFArSuD2RyPBXoe1FY7MVK')
ON CONFLICT (id) DO NOTHING;

-- Roles por conjunto (tenant). Conjunto 1 = Los Pinos (…0001), conjunto 2 = Alto del Bosque (…0002).
INSERT INTO auth_roles (id, code, name, description, scope, tenant_id) VALUES
    ('30000000-0000-0000-0000-000000000101', 'RESIDENT', 'Residente',      'Residente del conjunto', 'TENANT', '00000000-0000-0000-0000-000000000001'),
    ('30000000-0000-0000-0000-000000000102', 'GUARD',    'Guardia',        'Guardia del conjunto',   'TENANT', '00000000-0000-0000-0000-000000000001'),
    ('30000000-0000-0000-0000-000000000103', 'ADMIN',    'Administrador',  'Administrador del conjunto', 'TENANT', '00000000-0000-0000-0000-000000000001'),
    ('30000000-0000-0000-0000-000000000201', 'RESIDENT', 'Residente',      'Residente del conjunto', 'TENANT', '00000000-0000-0000-0000-000000000002'),
    ('30000000-0000-0000-0000-000000000202', 'GUARD',    'Guardia',        'Guardia del conjunto',   'TENANT', '00000000-0000-0000-0000-000000000002'),
    ('30000000-0000-0000-0000-000000000203', 'ADMIN',    'Administrador',  'Administrador del conjunto', 'TENANT', '00000000-0000-0000-0000-000000000002')
ON CONFLICT DO NOTHING;

-- Asignaciones (mismas que unit_members / complex_staff de control-de-accesos):
--   Ana García   → residente c1          Pedro Gómez  → residente c2
--   Luis Mora    → guardia c1            Marta Ruiz   → guardia c2
--   Carlos Rojas → administrador c1
--   Sofía Díaz   → residente c1 y c2
--   Andrés Vega  → residente c1 y guardia c2
INSERT INTO auth_user_roles (id, user_id, role_id, tenant_id) VALUES
    ('40000000-0000-0000-0000-000000000001', '10000000-0000-0000-0000-000000000001', '30000000-0000-0000-0000-000000000101', '00000000-0000-0000-0000-000000000001'),
    ('40000000-0000-0000-0000-000000000002', '10000000-0000-0000-0000-000000000002', '30000000-0000-0000-0000-000000000201', '00000000-0000-0000-0000-000000000002'),
    ('40000000-0000-0000-0000-000000000003', '10000000-0000-0000-0000-000000000003', '30000000-0000-0000-0000-000000000102', '00000000-0000-0000-0000-000000000001'),
    ('40000000-0000-0000-0000-000000000004', '10000000-0000-0000-0000-000000000004', '30000000-0000-0000-0000-000000000202', '00000000-0000-0000-0000-000000000002'),
    ('40000000-0000-0000-0000-000000000005', '10000000-0000-0000-0000-000000000005', '30000000-0000-0000-0000-000000000103', '00000000-0000-0000-0000-000000000001'),
    ('40000000-0000-0000-0000-000000000006', '10000000-0000-0000-0000-000000000006', '30000000-0000-0000-0000-000000000101', '00000000-0000-0000-0000-000000000001'),
    ('40000000-0000-0000-0000-000000000007', '10000000-0000-0000-0000-000000000006', '30000000-0000-0000-0000-000000000201', '00000000-0000-0000-0000-000000000002'),
    ('40000000-0000-0000-0000-000000000008', '10000000-0000-0000-0000-000000000007', '30000000-0000-0000-0000-000000000101', '00000000-0000-0000-0000-000000000001'),
    ('40000000-0000-0000-0000-000000000009', '10000000-0000-0000-0000-000000000007', '30000000-0000-0000-0000-000000000202', '00000000-0000-0000-0000-000000000002')
ON CONFLICT DO NOTHING;
