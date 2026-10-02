-- ═══════════════════════════════════════════════════════════════════════════
-- DATOS DE DEMOSTRACIÓN — SOLO DESARROLLO / PRUEBAS. NUNCA EN PRODUCCIÓN.
--
-- Personas, conjuntos y pases con UUID fijos y predecibles (incluye filas de
-- complex_staff que otorgan rol de guardia/administrador). Por eso NO están en
-- db/migrations (que sí se aplican a cualquier base, RDS incluida).
--
--   · docker compose los carga solo en el primer arranque (ver docker-compose.yml)
--   · db/migrate.sh solo los aplica con WITH_SEEDS=1
--   · test/reset_data.sh los vuelve a cargar
-- Idempotente: se puede ejecutar varias veces.
-- ═══════════════════════════════════════════════════════════════════════════

-- Demo data for local testing. IDs match cmd/api/main.go's /dev/token endpoint.

INSERT INTO units (id, complex_id, name) VALUES
    ('10000000-0000-0000-0000-000000000101', '00000000-0000-0000-0000-000000000001', 'A-101')
ON CONFLICT (id) DO NOTHING;

INSERT INTO users (id, name, phone) VALUES
    ('10000000-0000-0000-0000-000000000001', 'Resident Uno', '+57 300 0000001')
ON CONFLICT (id) DO NOTHING;

INSERT INTO unit_members (unit_id, user_id) VALUES
    ('10000000-0000-0000-0000-000000000101', '10000000-0000-0000-0000-000000000001')
ON CONFLICT DO NOTHING;

-- One active QR grant (qr_code_hash is fixed/readable so it can be validated
-- directly via curl without first generating a real one) and one active
-- pre-authorization, mirroring the seed data the resident app expects.
INSERT INTO access_grants (
    id, complex_id, unit_id, created_by, method, visitor_type, visitor_name,
    visitor_id_number, qr_code_hash, status, valid_from, valid_until
) VALUES
    (
        '20000000-0000-0000-0000-000000000001',
        '00000000-0000-0000-0000-000000000001',
        '10000000-0000-0000-0000-000000000101',
        '10000000-0000-0000-0000-000000000001',
        'QR', 'VISITOR', 'Juan Perez', NULL, 'demo-qr-hash-0001', 'ACTIVE',
        now(), now() + interval '1 day'
    ),
    (
        '20000000-0000-0000-0000-000000000002',
        '00000000-0000-0000-0000-000000000001',
        '10000000-0000-0000-0000-000000000101',
        '10000000-0000-0000-0000-000000000001',
        'PRE_AUTH', 'DELIVERY', 'Rappi Delivery', 'COL123456', NULL, 'ACTIVE',
        now(), now() + interval '7 day'
    )
ON CONFLICT (id) DO NOTHING;

-- ── Multi-conjunto: personas, unidades, personal y pases del conjunto 2 ─────

-- ── Datos demo ────────────────────────────────────────────────────────────
-- Conjunto 1 (…0001) ya existe desde 002_seed.sql (unidad A-101, usuario …0001).

INSERT INTO complexes (id, name) VALUES
    ('00000000-0000-0000-0000-000000000001', 'Conjunto Residencial Los Pinos'),
    ('00000000-0000-0000-0000-000000000002', 'Conjunto Alto del Bosque')
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name;

INSERT INTO units (id, complex_id, name) VALUES
    ('10000000-0000-0000-0000-000000000101', '00000000-0000-0000-0000-000000000001', 'A-101'),
    ('10000000-0000-0000-0000-000000000102', '00000000-0000-0000-0000-000000000001', 'A-102'),
    ('10000000-0000-0000-0000-000000000201', '00000000-0000-0000-0000-000000000002', 'B-201'),
    ('10000000-0000-0000-0000-000000000202', '00000000-0000-0000-0000-000000000002', 'B-202')
ON CONFLICT (id) DO NOTHING;

INSERT INTO users (id, name, phone) VALUES
    ('10000000-0000-0000-0000-000000000001', 'Ana García',   '+57 300 0000001'),
    ('10000000-0000-0000-0000-000000000002', 'Pedro Gómez',  '+57 300 0000002'),
    ('10000000-0000-0000-0000-000000000003', 'Luis Mora',    '+57 300 0000003'),
    ('10000000-0000-0000-0000-000000000004', 'Marta Ruiz',   '+57 300 0000004'),
    ('10000000-0000-0000-0000-000000000005', 'Carlos Rojas', '+57 300 0000005'),
    ('10000000-0000-0000-0000-000000000006', 'Sofía Díaz',   '+57 300 0000006'),
    ('10000000-0000-0000-0000-000000000007', 'Andrés Vega',  '+57 300 0000007')
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name;

-- Residentes
--   Ana García   → A-101 (conjunto 1)
--   Pedro Gómez  → B-201 (conjunto 2)
--   Sofía Díaz   → A-102 (conjunto 1) Y B-202 (conjunto 2)
--   Andrés Vega  → A-102 (conjunto 1) y además es GUARDIA del conjunto 2
INSERT INTO unit_members (unit_id, user_id) VALUES
    ('10000000-0000-0000-0000-000000000101', '10000000-0000-0000-0000-000000000001'),
    ('10000000-0000-0000-0000-000000000201', '10000000-0000-0000-0000-000000000002'),
    ('10000000-0000-0000-0000-000000000102', '10000000-0000-0000-0000-000000000006'),
    ('10000000-0000-0000-0000-000000000202', '10000000-0000-0000-0000-000000000006'),
    ('10000000-0000-0000-0000-000000000102', '10000000-0000-0000-0000-000000000007')
ON CONFLICT DO NOTHING;

-- Personal
--   Luis Mora    → guardia del conjunto 1
--   Marta Ruiz   → guardia del conjunto 2
--   Carlos Rojas → administrador del conjunto 1
--   Andrés Vega  → guardia del conjunto 2 (y residente del 1, ver arriba)
INSERT INTO complex_staff (complex_id, user_id, role) VALUES
    ('00000000-0000-0000-0000-000000000001', '10000000-0000-0000-0000-000000000003', 'GUARD'),
    ('00000000-0000-0000-0000-000000000002', '10000000-0000-0000-0000-000000000004', 'GUARD'),
    ('00000000-0000-0000-0000-000000000001', '10000000-0000-0000-0000-000000000005', 'ADMIN'),
    ('00000000-0000-0000-0000-000000000002', '10000000-0000-0000-0000-000000000007', 'GUARD')
ON CONFLICT DO NOTHING;

-- Grants demo del conjunto 2 (unidad B-201, creados por Pedro Gómez).
-- La cédula COL555555 existe en AMBOS conjuntos a propósito: validar en uno
-- no debe consumir la del otro.
INSERT INTO access_grants (
    id, complex_id, unit_id, created_by, method, visitor_type, visitor_name,
    visitor_id_number, qr_code_hash, status, valid_from, valid_until
) VALUES
    ('20000000-0000-0000-0000-000000000011', '00000000-0000-0000-0000-000000000002',
     '10000000-0000-0000-0000-000000000201', '10000000-0000-0000-0000-000000000002',
     'QR', 'VISITOR', 'Visita Bosque', NULL, 'demo-qr-hash-c2-0001', 'ACTIVE', now(), now() + interval '1 day'),
    ('20000000-0000-0000-0000-000000000012', '00000000-0000-0000-0000-000000000002',
     '10000000-0000-0000-0000-000000000201', '10000000-0000-0000-0000-000000000002',
     'PRE_AUTH', 'DELIVERY', 'Domiciliario Bosque', 'COL777777', NULL, 'ACTIVE', now(), now() + interval '7 day'),
    ('20000000-0000-0000-0000-000000000013', '00000000-0000-0000-0000-000000000001',
     '10000000-0000-0000-0000-000000000101', '10000000-0000-0000-0000-000000000001',
     'PRE_AUTH', 'VISITOR', 'Misma Cédula Pinos', 'COL555555', NULL, 'ACTIVE', now(), now() + interval '7 day'),
    ('20000000-0000-0000-0000-000000000014', '00000000-0000-0000-0000-000000000002',
     '10000000-0000-0000-0000-000000000201', '10000000-0000-0000-0000-000000000002',
     'PRE_AUTH', 'VISITOR', 'Misma Cédula Bosque', 'COL555555', NULL, 'ACTIVE', now(), now() + interval '7 day')
ON CONFLICT (id) DO NOTHING;
