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
