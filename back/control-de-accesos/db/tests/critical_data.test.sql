-- Critical-flow component tests — DATA TIER (control-de-accesos schema).
--
-- Components under test (the schema itself, not the Go code):
--   D1  access_grants   CHECK constraints, defaults and the single-use
--                       conditional UPDATE issued by TryMarkGrantUsed
--   D2  audit_log       append-only triggers (prevent_audit_log_mutation)
--   D3  access_logs     idempotency UNIQUE (complex_id, client_id) and the
--                       visitor/resident subject CHECK
--   D4  complex_staff / unit_members   role CHECK and ON DELETE CASCADE
--
-- Isolation: runs on a fresh database with only the migrations applied, inside
-- one transaction that is rolled back at the end. Every case runs in its own
-- subtransaction (plpgsql EXCEPTION block), so a failing statement cannot
-- affect the next case. Fixed UUIDs and fixed timestamps: no randomness, and
-- now() is only compared against itself.
--
-- Each case is tagged with its ID (TC-DB-xx); the script prints PASS/FAIL per
-- case and exits non-zero if any case fails (psql -v ON_ERROR_STOP=1).

\set QUIET on
\pset footer off
SET client_min_messages = warning;
BEGIN;
\o /dev/null

CREATE TEMP TABLE results (n serial, id text, ok boolean, detail text);

-- Statement must succeed.
CREATE FUNCTION pg_temp.expect_ok(tc text, stmt text) RETURNS void AS $$
BEGIN
    EXECUTE stmt;
    INSERT INTO results (id, ok, detail) VALUES (tc, true, 'ok');
EXCEPTION WHEN others THEN
    INSERT INTO results (id, ok, detail) VALUES (tc, false, 'unexpected error ' || SQLSTATE || ': ' || SQLERRM);
END $$ LANGUAGE plpgsql;

-- Statement must fail with the given SQLSTATE (and, optionally, message fragment).
CREATE FUNCTION pg_temp.expect_error(tc text, stmt text, state text, fragment text DEFAULT NULL) RETURNS void AS $$
BEGIN
    EXECUTE stmt;
    INSERT INTO results (id, ok, detail) VALUES (tc, false, 'expected ' || state || ' but the statement succeeded');
EXCEPTION WHEN others THEN
    INSERT INTO results (id, ok, detail) VALUES (tc,
        SQLSTATE = state AND (fragment IS NULL OR position(fragment IN SQLERRM) > 0),
        SQLSTATE || ': ' || SQLERRM);
END $$ LANGUAGE plpgsql;

-- Query must return exactly the expected value (compared as text).
CREATE FUNCTION pg_temp.expect_eq(tc text, query text, expected text) RETURNS void AS $$
DECLARE got text;
BEGIN
    EXECUTE query INTO got;
    INSERT INTO results (id, ok, detail) VALUES (tc, got IS NOT DISTINCT FROM expected,
        'expected ' || coalesce(expected, 'NULL') || ', got ' || coalesce(got, 'NULL'));
EXCEPTION WHEN others THEN
    INSERT INTO results (id, ok, detail) VALUES (tc, false, 'unexpected error ' || SQLSTATE || ': ' || SQLERRM);
END $$ LANGUAGE plpgsql;

-- Builds an INSERT into access_grants; only the columns under test vary.
CREATE FUNCTION pg_temp.grant_sql(id text, method text, qr text, status text DEFAULT 'ACTIVE', vtype text DEFAULT 'VISITOR') RETURNS text AS $$
    SELECT format(
        'INSERT INTO access_grants (id, complex_id, unit_id, created_by, method, visitor_type, visitor_name, qr_code_hash, status, valid_until)
         VALUES (%L, ''00000000-0000-0000-0000-000000000001'', ''10000000-0000-0000-0000-000000000101'',
                 ''20000000-0000-0000-0000-000000000001'', %L, %L, ''Juan'', %L, %L, ''2026-11-21 18:00:00-05'')',
        id, method, vtype, qr, status)
$$ LANGUAGE sql;

-- The exact statement of TryMarkGrantUsed (internal/access/repository.go), returning rows affected.
CREATE FUNCTION pg_temp.mark_used_sql(id text) RETURNS text AS $$
    SELECT format(
        'WITH u AS (UPDATE access_grants SET status = ''USED'', used_at = NOW(), used_by = ''30000000-0000-0000-0000-000000000001''
                   WHERE id = %L AND status = ''ACTIVE'' RETURNING 1)
         SELECT count(*) FROM u', id)
$$ LANGUAGE sql;

-- ── D1 access_grants: method / QR code (decision table) ─────────────────────
--   method   | qr_code_hash | result
--   QR       | present      | accepted
--   QR       | NULL         | rejected (qr_required_if_method_qr)
--   PRE_AUTH | NULL         | accepted
--   other    | any          | rejected (access_grants_method_check)
SELECT pg_temp.expect_ok   ('TC-DB-01a QR with code is accepted',        pg_temp.grant_sql('a0000000-0000-0000-0000-000000000001', 'QR', 'CODE-1'));
SELECT pg_temp.expect_error('TC-DB-01b QR without code is rejected',     pg_temp.grant_sql('a0000000-0000-0000-0000-000000000002', 'QR', NULL), '23514', 'qr_required_if_method_qr');
SELECT pg_temp.expect_ok   ('TC-DB-01c PRE_AUTH without code accepted',  pg_temp.grant_sql('a0000000-0000-0000-0000-000000000003', 'PRE_AUTH', NULL));
SELECT pg_temp.expect_error('TC-DB-01d unknown method is rejected',      pg_temp.grant_sql('a0000000-0000-0000-0000-000000000004', 'NFC', 'CODE-4'), '23514', 'access_grants_method_check');

-- ── D1 access_grants: enumerations (equivalence partitioning) ───────────────
SELECT pg_temp.expect_error('TC-DB-02a status outside the enumeration',  pg_temp.grant_sql('a0000000-0000-0000-0000-000000000005', 'QR', 'CODE-5', 'PENDING'), '23514', 'access_grants_status_check');
SELECT pg_temp.expect_error('TC-DB-02b visitor_type outside the enum',   pg_temp.grant_sql('a0000000-0000-0000-0000-000000000006', 'QR', 'CODE-6', 'ACTIVE', 'FRIEND'), '23514', 'access_grants_visitor_type_check');
SELECT pg_temp.expect_error('TC-DB-02c lowercase status is rejected',    pg_temp.grant_sql('a0000000-0000-0000-0000-000000000007', 'QR', 'CODE-7', 'active'), '23514', 'access_grants_status_check');
SELECT pg_temp.expect_error('TC-DB-02d valid_until is mandatory',
    $q$INSERT INTO access_grants (id, complex_id, unit_id, created_by, method, visitor_type, visitor_name, qr_code_hash)
       VALUES ('a0000000-0000-0000-0000-000000000008', gen_random_uuid(), gen_random_uuid(), gen_random_uuid(), 'QR', 'VISITOR', 'Juan', 'CODE-8')$q$, '23502');

-- ── D1 access_grants: defaults ─────────────────────────────────────────────
SELECT pg_temp.expect_eq('TC-DB-03 a new grant is ACTIVE, single-use, valid from its creation',
    $q$SELECT status || ',' || single_use || ',' || (valid_from = created_at)
       FROM access_grants WHERE id = 'a0000000-0000-0000-0000-000000000001'$q$, 'ACTIVE,true,true');

-- ── D1 single-use consumption (state transition) ────────────────────────────
--   ACTIVE --mark used--> USED --mark used--> USED (0 rows: second scan loses)
--   CANCELLED --mark used--> CANCELLED (0 rows)
SELECT pg_temp.expect_eq('TC-DB-04a first validation consumes the grant', pg_temp.mark_used_sql('a0000000-0000-0000-0000-000000000001'), '1');
SELECT pg_temp.expect_eq('TC-DB-04b grant is now USED with who/when',
    $q$SELECT status || ',' || (used_at IS NOT NULL) || ',' || used_by FROM access_grants WHERE id = 'a0000000-0000-0000-0000-000000000001'$q$,
    'USED,true,30000000-0000-0000-0000-000000000001');
SELECT pg_temp.expect_eq('TC-DB-04c second validation affects no row',   pg_temp.mark_used_sql('a0000000-0000-0000-0000-000000000001'), '0');
SELECT pg_temp.expect_ok('TC-DB-04d setup: cancelled grant',             pg_temp.grant_sql('a0000000-0000-0000-0000-000000000009', 'QR', 'CODE-9', 'CANCELLED'));
SELECT pg_temp.expect_eq('TC-DB-04e a CANCELLED grant cannot be consumed', pg_temp.mark_used_sql('a0000000-0000-0000-0000-000000000009'), '0');
SELECT pg_temp.expect_eq('TC-DB-04f an unknown id affects no row',       pg_temp.mark_used_sql('a0000000-0000-0000-0000-0000000000ff'), '0');

-- ── D2 audit_log is append-only ─────────────────────────────────────────────
SELECT pg_temp.expect_ok('TC-DB-05a an audit entry can be inserted',
    $q$INSERT INTO audit_log (id, resource_type, action, result, reason)
       VALUES ('b0000000-0000-0000-0000-000000000001', 'access_grant', 'VALIDATE_ACCESS_GRANT', 'DENIED', 'cross-tenant access attempt')$q$);
SELECT pg_temp.expect_error('TC-DB-05b UPDATE of an audit entry is blocked',
    $q$UPDATE audit_log SET result = 'ALLOWED' WHERE id = 'b0000000-0000-0000-0000-000000000001'$q$, 'P0001', 'audit_log is append-only');
SELECT pg_temp.expect_error('TC-DB-05c DELETE of an audit entry is blocked',
    $q$DELETE FROM audit_log WHERE id = 'b0000000-0000-0000-0000-000000000001'$q$, 'P0001', 'audit_log is append-only');
SELECT pg_temp.expect_eq('TC-DB-05d the entry is intact after both attempts',
    $q$SELECT result FROM audit_log WHERE id = 'b0000000-0000-0000-0000-000000000001'$q$, 'DENIED');
SELECT pg_temp.expect_error('TC-DB-05e result outside ALLOWED/DENIED is rejected',
    $q$INSERT INTO audit_log (id, resource_type, action, result) VALUES ('b0000000-0000-0000-0000-000000000002', 'unit', 'X', 'MAYBE')$q$, '23514');

-- ── D3 access_logs: offline sync idempotency ────────────────────────────────
CREATE FUNCTION pg_temp.log_sql(id text, complex text, client text, subject text DEFAULT 'VISITOR', subject_user text DEFAULT NULL,
                                movement text DEFAULT 'ENTRY', result text DEFAULT 'AUTHORIZED') RETURNS text AS $$
    SELECT format(
        'INSERT INTO access_logs (id, client_id, complex_id, movement, result, visitor_name, unit_label, registered_by, occurred_at, subject_type, subject_user_id)
         VALUES (%L, %L, %L, %L, %L, ''Juan'', ''A-101'', ''20000000-0000-0000-0000-000000000001'', ''2026-11-21 10:00:00-05'', %L, %L)',
        id, client, complex, movement, result, subject, subject_user)
$$ LANGUAGE sql;
SELECT pg_temp.expect_ok   ('TC-DB-06a first sync of a gate record',     pg_temp.log_sql('c0000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000001'));
SELECT pg_temp.expect_error('TC-DB-06b retry with the same client_id is rejected',
                                                                         pg_temp.log_sql('c0000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000001'), '23505');
SELECT pg_temp.expect_ok   ('TC-DB-06c same client_id in another complex is independent',
                                                                         pg_temp.log_sql('c0000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000002', 'd0000000-0000-0000-0000-000000000001'));

-- ── D3 access_logs: subject rule (decision table) ───────────────────────────
--   subject  | subject_user | movement     | result     | outcome
--   VISITOR  | -            | ATTEMPT      | DENIED     | accepted
--   RESIDENT | present      | ENTRY / EXIT | AUTHORIZED | accepted
--   RESIDENT | NULL         | ENTRY        | AUTHORIZED | rejected
--   RESIDENT | present      | ATTEMPT      | AUTHORIZED | rejected
--   RESIDENT | present      | ENTRY        | DENIED     | rejected
SELECT pg_temp.expect_ok   ('TC-DB-07a visitor denied attempt',          pg_temp.log_sql('c0000000-0000-0000-0000-000000000011', '00000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000011', 'VISITOR', NULL, 'ATTEMPT', 'DENIED'));
SELECT pg_temp.expect_ok   ('TC-DB-07b resident authorized exit',        pg_temp.log_sql('c0000000-0000-0000-0000-000000000012', '00000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000012', 'RESIDENT', '40000000-0000-0000-0000-000000000001', 'EXIT'));
SELECT pg_temp.expect_error('TC-DB-07c resident without user id',        pg_temp.log_sql('c0000000-0000-0000-0000-000000000013', '00000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000013', 'RESIDENT', NULL, 'ENTRY'), '23514', 'access_logs_subject_check');
SELECT pg_temp.expect_error('TC-DB-07d resident ATTEMPT',                pg_temp.log_sql('c0000000-0000-0000-0000-000000000014', '00000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000014', 'RESIDENT', '40000000-0000-0000-0000-000000000001', 'ATTEMPT'), '23514', 'access_logs_subject_check');
SELECT pg_temp.expect_error('TC-DB-07e resident DENIED',                 pg_temp.log_sql('c0000000-0000-0000-0000-000000000015', '00000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000015', 'RESIDENT', '40000000-0000-0000-0000-000000000001', 'ENTRY', 'DENIED'), '23514', 'access_logs_subject_check');

-- ── D4 staff roles and unit membership ──────────────────────────────────────
SELECT pg_temp.expect_ok   ('TC-DB-08a GUARD staff of a complex',
    $q$INSERT INTO complex_staff (complex_id, user_id, role) VALUES ('00000000-0000-0000-0000-000000000001', '50000000-0000-0000-0000-000000000001', 'GUARD')$q$);
SELECT pg_temp.expect_error('TC-DB-08b RESIDENT is not a staff role',
    $q$INSERT INTO complex_staff (complex_id, user_id, role) VALUES ('00000000-0000-0000-0000-000000000001', '50000000-0000-0000-0000-000000000002', 'RESIDENT')$q$, '23514');
SELECT pg_temp.expect_error('TC-DB-08c the same staff role twice is rejected',
    $q$INSERT INTO complex_staff (complex_id, user_id, role) VALUES ('00000000-0000-0000-0000-000000000001', '50000000-0000-0000-0000-000000000001', 'GUARD')$q$, '23505');
SELECT pg_temp.expect_ok('TC-DB-09 setup: unit with one member',
    $q$INSERT INTO units (id, complex_id, name) VALUES ('60000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001', 'Z-999');
       INSERT INTO unit_members (unit_id, user_id) VALUES ('60000000-0000-0000-0000-000000000001', '40000000-0000-0000-0000-000000000001');
       DELETE FROM units WHERE id = '60000000-0000-0000-0000-000000000001'$q$);
SELECT pg_temp.expect_eq('TC-DB-09 deleting a unit removes its memberships (no orphan access)',
    $q$SELECT count(*) FROM unit_members WHERE unit_id = '60000000-0000-0000-0000-000000000001'$q$, '0');

\o
-- ── report ──────────────────────────────────────────────────────────────────
\set QUIET off
SELECT CASE WHEN ok THEN 'PASS' ELSE 'FAIL' END AS verdict, id AS test_case,
       CASE WHEN ok THEN '' ELSE detail END AS detail
FROM results ORDER BY n;
SELECT count(*) FILTER (WHERE ok) AS passed, count(*) FILTER (WHERE NOT ok) AS failed FROM results;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM results WHERE NOT ok) THEN
        RAISE EXCEPTION 'data-tier suite: % case(s) failed', (SELECT count(*) FROM results WHERE NOT ok);
    END IF;
END $$;

ROLLBACK;
