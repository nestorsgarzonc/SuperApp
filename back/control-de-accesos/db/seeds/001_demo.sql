-- Registros de ejemplo para control de accesos.
--
-- Los UUID de complejo, sección, unidad y usuario son los mismos de
-- docker/postgres/seed.sql, para que este módulo se pueda enganchar
-- al demo "Demo Residential Complex" sin inventar otro catálogo.
--
-- El pase de María y la preautorización de placa ABC123 retoman los
-- ejemplos de access_grants del seed (visitante QR y entrega PRE_AUTH).

INSERT INTO ref_complexes (id, name, address, timezone, is_active)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    'Demo Residential Complex',
    'Calle 123 #45-67, Bogotá, Colombia',
    'America/Bogota',
    TRUE
)
ON CONFLICT (id) DO NOTHING;

INSERT INTO ref_users (id, full_name, email, status)
VALUES
    ('10000000-0000-0000-0000-000000000001', 'Admin Demo', 'admin@demo.com', 'ACTIVE'),
    ('20000000-0000-0000-0000-000000000001', 'Guard Demo', 'guard@demo.com', 'ACTIVE'),
    ('30000000-0000-0000-0000-000000000001', 'John Resident', 'resident1@demo.com', 'ACTIVE'),
    ('30000000-0000-0000-0000-000000000002', 'Jane Resident', 'resident2@demo.com', 'ACTIVE')
ON CONFLICT (id) DO NOTHING;

INSERT INTO ref_complex_staff (complex_id, user_id, role)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    '20000000-0000-0000-0000-000000000001',
    'GUARD'
)
ON CONFLICT (complex_id, user_id) DO NOTHING;

INSERT INTO ref_sections (id, complex_id, name)
VALUES (
    '40000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    'Tower A'
)
ON CONFLICT (id) DO NOTHING;

INSERT INTO ref_units (id, complex_id, section_id, name, sort_order)
VALUES
    ('50000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001', '40000000-0000-0000-0000-000000000001', 'A-101', 101),
    ('50000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000001', '40000000-0000-0000-0000-000000000001', 'A-102', 102)
ON CONFLICT (id) DO NOTHING;

INSERT INTO ref_unit_members (complex_id, unit_id, user_id, role, is_primary)
VALUES
    (
        '00000000-0000-0000-0000-000000000001',
        '50000000-0000-0000-0000-000000000001',
        '30000000-0000-0000-0000-000000000001',
        'OWNER',
        TRUE
    ),
    (
        '00000000-0000-0000-0000-000000000001',
        '50000000-0000-0000-0000-000000000002',
        '30000000-0000-0000-0000-000000000002',
        'HEAD_TENANT',
        TRUE
    )
ON CONFLICT (unit_id, user_id) DO NOTHING;

-- Pase activo de María, análogo al access_grant QR del seed (válido 24 h).
-- Token fijo para poder probar la portería sin crear el pase a mano.
INSERT INTO visitor_passes (
    id, complex_id, unit_id, host_user_id, visitor_name, document, token,
    valid_from, valid_until, status
)
VALUES (
    '60000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    '50000000-0000-0000-0000-000000000001',
    '30000000-0000-0000-0000-000000000001',
    'Maria Visitor',
    '1234567890',
    'demo-token-maria-visitor',
    NOW(),
    NOW() + INTERVAL '24 hours',
    'active'
)
ON CONFLICT (id) DO NOTHING;

-- Placa de visitante, análoga al PRE_AUTH "Rappi Driver" / ABC-123 del seed.
-- La placa se guarda normalizada (mayúsculas, sin guiones), como en el monolito.
INSERT INTO visitor_vehicle_preauths (
    id, complex_id, unit_id, host_user_id, plate, visitor_name, valid_from, valid_until, status
)
VALUES (
    '70000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    '50000000-0000-0000-0000-000000000002',
    '30000000-0000-0000-0000-000000000002',
    'ABC123',
    'Rappi Driver',
    NOW(),
    NOW() + INTERVAL '3 days',
    'active'
)
ON CONFLICT (id) DO NOTHING;
