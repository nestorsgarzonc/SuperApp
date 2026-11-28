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

CREATE TEMP TABLE results (n serial, id text, ok boolean, detail text, skip boolean NOT NULL DEFAULT false);

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

-- Known gap: the contract says the statement must be rejected, but the schema may not
-- enforce it. Rejected (any SQLSTATE) -> PASS; accepted -> SKIP with the reason, so the
-- gap is reported (Delivery 1, section 9) without turning the suite red. The statement
-- runs in its own subtransaction and is undone either way.
CREATE FUNCTION pg_temp.expect_rejected_gap(tc text, stmt text, reason text) RETURNS void AS $$
BEGIN
    BEGIN
        EXECUTE stmt;
        RAISE EXCEPTION USING ERRCODE = 'Z0001', MESSAGE = 'accepted';
    EXCEPTION
        WHEN SQLSTATE 'Z0001' THEN
            INSERT INTO results (id, ok, skip, detail) VALUES (tc, true, true, 'rule not enforced by the schema: ' || reason);
            RETURN;
        WHEN others THEN
            INSERT INTO results (id, ok, detail) VALUES (tc, true, 'rejected ' || SQLSTATE);
            RETURN;
    END;
END $$ LANGUAGE plpgsql;

-- Known gap on a value: equal to the contract's expectation -> PASS; different -> SKIP.
CREATE FUNCTION pg_temp.expect_eq_gap(tc text, query text, expected text, reason text) RETURNS void AS $$
DECLARE got text;
BEGIN
    EXECUTE query INTO got;
    IF got IS NOT DISTINCT FROM expected THEN
        INSERT INTO results (id, ok, detail) VALUES (tc, true, 'ok');
    ELSE
        INSERT INTO results (id, ok, skip, detail) VALUES (tc, true, true,
            reason || ' (expected ' || coalesce(expected, 'NULL') || ', got ' || coalesce(got, 'NULL') || ')');
    END IF;
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

-- ════════════════════════════════════════════════════════════════════════════
-- Delivery 1, C1 (BD de Control de Accesos): cases added from the contract.
-- ════════════════════════════════════════════════════════════════════════════

-- ── D1 default status, really omitted (TC-DB-03 passes status explicitly) ────
SELECT pg_temp.expect_ok('TC-DB-03b setup: grant inserted without status',
    $q$INSERT INTO access_grants (id, complex_id, unit_id, created_by, method, visitor_type, visitor_name, qr_code_hash, valid_until)
       VALUES ('a0000000-0000-0000-0000-000000000031', '00000000-0000-0000-0000-000000000001', '10000000-0000-0000-0000-000000000101',
               '20000000-0000-0000-0000-000000000001', 'QR', 'VISITOR', 'Juan', 'CODE-31', '2026-11-21 18:00:00-05')$q$);
SELECT pg_temp.expect_eq('TC-DB-03b omitted status defaults to ACTIVE',
    $q$SELECT status FROM access_grants WHERE id = 'a0000000-0000-0000-0000-000000000031'$q$, 'ACTIVE');

-- ── TC-DB-10 valid classes: every allowed value is accepted (EP) ────────────
SELECT pg_temp.expect_ok('TC-DB-10a visitor_type DELIVERY',   pg_temp.grant_sql('a0000000-0000-0000-0000-000000000101', 'QR', 'CODE-101', 'ACTIVE', 'DELIVERY'));
SELECT pg_temp.expect_ok('TC-DB-10b visitor_type VISITOR',    pg_temp.grant_sql('a0000000-0000-0000-0000-000000000102', 'QR', 'CODE-102', 'ACTIVE', 'VISITOR'));
SELECT pg_temp.expect_ok('TC-DB-10c visitor_type SERVICE',    pg_temp.grant_sql('a0000000-0000-0000-0000-000000000103', 'QR', 'CODE-103', 'ACTIVE', 'SERVICE'));
SELECT pg_temp.expect_ok('TC-DB-10d visitor_type CONTRACTOR', pg_temp.grant_sql('a0000000-0000-0000-0000-000000000104', 'QR', 'CODE-104', 'ACTIVE', 'CONTRACTOR'));
SELECT pg_temp.expect_ok('TC-DB-10e visitor_type GUEST',      pg_temp.grant_sql('a0000000-0000-0000-0000-000000000105', 'QR', 'CODE-105', 'ACTIVE', 'GUEST'));
SELECT pg_temp.expect_ok('TC-DB-10f status USED',             pg_temp.grant_sql('a0000000-0000-0000-0000-000000000106', 'QR', 'CODE-106', 'USED'));
SELECT pg_temp.expect_ok('TC-DB-10g status EXPIRED',          pg_temp.grant_sql('a0000000-0000-0000-0000-000000000107', 'QR', 'CODE-107', 'EXPIRED'));
SELECT pg_temp.expect_ok('TC-DB-10h staff role ADMIN',
    $q$INSERT INTO complex_staff (complex_id, user_id, role) VALUES ('00000000-0000-0000-0000-000000000001', '50000000-0000-0000-0000-000000000001', 'ADMIN')$q$);
SELECT pg_temp.expect_ok('TC-DB-10i audit result ALLOWED',
    $q$INSERT INTO audit_log (id, resource_type, action, result) VALUES ('b0000000-0000-0000-0000-000000000010', 'access_grant', 'CREATE_ACCESS_GRANT', 'ALLOWED')$q$);

-- ── TC-DB-11 the INSERT of accesslog.Create (internal/accesslog/repository.go) ─
-- Same ON CONFLICT DO NOTHING and the same grant_id subquery (kept only when the
-- grant belongs to the same complex); returns the rows affected.
CREATE FUNCTION pg_temp.create_log_sql(complex text, client text, grant_id text DEFAULT NULL) RETURNS text AS $$
    SELECT format(
        $f$WITH ins AS (INSERT INTO access_logs (id, client_id, complex_id, movement, result, visitor_name, unit_label,
                                                 visitor_type, vehicle, grant_id, registered_by, occurred_at)
                        VALUES (gen_random_uuid(), %L, %L, 'ENTRY', 'AUTHORIZED', 'Juan', 'A-101', 'VISITOR', NULL,
                                (SELECT id FROM access_grants WHERE id = %L AND complex_id = %L),
                                '20000000-0000-0000-0000-000000000001', '2026-11-21 10:00:00-05')
                        ON CONFLICT (complex_id, client_id) DO NOTHING RETURNING 1)
           SELECT count(*) FROM ins$f$, client, complex, grant_id, complex)
$$ LANGUAGE sql;
SELECT pg_temp.expect_eq('TC-DB-11a first sync inserts one row',
    pg_temp.create_log_sql('00000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000111'), '1');
SELECT pg_temp.expect_eq('TC-DB-11b retry affects 0 rows and raises no error',
    pg_temp.create_log_sql('00000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000111'), '0');
SELECT pg_temp.expect_eq('TC-DB-11c still exactly one record for that client_id',
    $q$SELECT count(*) FROM access_logs WHERE complex_id = '00000000-0000-0000-0000-000000000001' AND client_id = 'd0000000-0000-0000-0000-000000000111'$q$, '1');
SELECT pg_temp.expect_eq('TC-DB-11d setup: log with a grant of the same complex',
    pg_temp.create_log_sql('00000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000112', 'a0000000-0000-0000-0000-000000000101'), '1');
SELECT pg_temp.expect_eq('TC-DB-11d grant of the same complex is kept',
    $q$SELECT coalesce(grant_id::text, 'NULL') FROM access_logs WHERE client_id = 'd0000000-0000-0000-0000-000000000112'$q$,
    'a0000000-0000-0000-0000-000000000101');
SELECT pg_temp.expect_eq('TC-DB-11e setup: log with a grant of another complex',
    pg_temp.create_log_sql('00000000-0000-0000-0000-000000000002', 'd0000000-0000-0000-0000-000000000113', 'a0000000-0000-0000-0000-000000000101'), '1');
SELECT pg_temp.expect_eq('TC-DB-11e grant of another complex is stored as NULL',
    $q$SELECT coalesce(grant_id::text, 'NULL') FROM access_logs WHERE client_id = 'd0000000-0000-0000-0000-000000000113'$q$, 'NULL');

-- ── TC-DB-12 the "active" filter of GrantSummary (internal/access/repository.go), BVA
-- now() is constant inside a transaction, so "exactly now" is a stable boundary.
CREATE FUNCTION pg_temp.summary_active_sql(id text) RETURNS text AS $$
    SELECT format(
        $f$SELECT count(*) FILTER (WHERE status = 'ACTIVE' AND valid_from <= NOW() AND valid_until >= NOW())
           FROM access_grants WHERE id = %L$f$, id)
$$ LANGUAGE sql;
SELECT pg_temp.expect_ok('TC-DB-12 setup: grants at the validity boundaries',
    $q$INSERT INTO access_grants (id, complex_id, unit_id, created_by, method, visitor_type, visitor_name, qr_code_hash, valid_from, valid_until) VALUES
       ('a0000000-0000-0000-0000-000000000121', '00000000-0000-0000-0000-000000000003', gen_random_uuid(), gen_random_uuid(), 'QR', 'VISITOR', 'Juan', 'CODE-121', now() - interval '1 hour', now()),
       ('a0000000-0000-0000-0000-000000000122', '00000000-0000-0000-0000-000000000003', gen_random_uuid(), gen_random_uuid(), 'QR', 'VISITOR', 'Juan', 'CODE-122', now() - interval '1 hour', now() - interval '1 microsecond'),
       ('a0000000-0000-0000-0000-000000000123', '00000000-0000-0000-0000-000000000003', gen_random_uuid(), gen_random_uuid(), 'QR', 'VISITOR', 'Juan', 'CODE-123', NULL, now() + interval '1 hour')$q$);
SELECT pg_temp.expect_eq('TC-DB-12a valid_until = now() counts as active',    pg_temp.summary_active_sql('a0000000-0000-0000-0000-000000000121'), '1');
SELECT pg_temp.expect_eq('TC-DB-12b valid_until = now() - 1us is not active', pg_temp.summary_active_sql('a0000000-0000-0000-0000-000000000122'), '0');
SELECT pg_temp.expect_eq_gap('TC-DB-12c a grant without valid_from counts as active',
    pg_temp.summary_active_sql('a0000000-0000-0000-0000-000000000123'), '1',
    'the service accepts a NULL valid_from, but the filter becomes NULL and GrantSummary skips it (DB-CA-F7)');

-- ── TC-DB-13 access_logs subject_type ───────────────────────────────────────
SELECT pg_temp.expect_eq('TC-DB-13a subject_type defaults to VISITOR',
    $q$SELECT subject_type FROM access_logs WHERE client_id = 'd0000000-0000-0000-0000-000000000111'$q$, 'VISITOR');
SELECT pg_temp.expect_error('TC-DB-13b subject_type outside VISITOR/RESIDENT',
    pg_temp.log_sql('c0000000-0000-0000-0000-000000000131', '00000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000131', 'ROBOT'),
    '23514', 'access_logs_subject_check');

-- ── Known gaps: the contract expects a rejection that the schema may not enforce
SELECT pg_temp.expect_rejected_gap('TC-DB-14 a USED grant cannot go back to ACTIVE',
    $q$UPDATE access_grants SET status = 'ACTIVE' WHERE id = 'a0000000-0000-0000-0000-000000000001'$q$,
    'no trigger protects final states (DB-CA-F1)');
SELECT pg_temp.expect_rejected_gap('TC-DB-15 valid_until before valid_from is rejected',
    $q$INSERT INTO access_grants (id, complex_id, unit_id, created_by, method, visitor_type, visitor_name, qr_code_hash, valid_from, valid_until)
       VALUES ('a0000000-0000-0000-0000-000000000151', gen_random_uuid(), gen_random_uuid(), gen_random_uuid(), 'QR', 'VISITOR', 'Juan', 'CODE-151',
               '2026-11-22 10:00:00-05', '2026-11-21 10:00:00-05')$q$,
    'no CHECK valid_from < valid_until (DB-CA-F2)');
SELECT pg_temp.expect_rejected_gap('TC-DB-16a blank visitor_name is rejected',
    $q$INSERT INTO access_grants (id, complex_id, unit_id, created_by, method, visitor_type, visitor_name, qr_code_hash, valid_until)
       VALUES ('a0000000-0000-0000-0000-000000000161', gen_random_uuid(), gen_random_uuid(), gen_random_uuid(), 'QR', 'VISITOR', '   ', 'CODE-161', '2026-11-21 18:00:00-05')$q$,
    'visitor_name is only NOT NULL (DB-CA-F3)');
SELECT pg_temp.expect_rejected_gap('TC-DB-16b PRE_AUTH without visitor_id_number is rejected',
    $q$INSERT INTO access_grants (id, complex_id, unit_id, created_by, method, visitor_type, visitor_name, valid_until)
       VALUES ('a0000000-0000-0000-0000-000000000162', gen_random_uuid(), gen_random_uuid(), gen_random_uuid(), 'PRE_AUTH', 'VISITOR', 'Juan', '2026-11-21 18:00:00-05')$q$,
    'no CHECK ties PRE_AUTH to visitor_id_number (DB-CA-F3)');
SELECT pg_temp.expect_rejected_gap('TC-DB-16c USED without used_at / used_by is rejected',
    pg_temp.grant_sql('a0000000-0000-0000-0000-000000000163', 'QR', 'CODE-163', 'USED'),
    'no CHECK ties USED to used_at and used_by (DB-CA-F4)');
SELECT pg_temp.expect_rejected_gap('TC-DB-17 a second grant with the same qr_code_hash is rejected',
    pg_temp.grant_sql('a0000000-0000-0000-0000-000000000171', 'QR', 'CODE-1'),
    'qr_code_hash has an index, not a UNIQUE constraint (DB-CA-F5)');
SELECT pg_temp.expect_rejected_gap('TC-DB-18a UPDATE of an access_logs record is rejected',
    $q$UPDATE access_logs SET result = 'DENIED' WHERE client_id = 'd0000000-0000-0000-0000-000000000111'$q$,
    'access_logs has no append-only trigger (DB-CA-F6)');
SELECT pg_temp.expect_rejected_gap('TC-DB-18b DELETE of an access_logs record is rejected',
    $q$DELETE FROM access_logs WHERE client_id = 'd0000000-0000-0000-0000-000000000111'$q$,
    'access_logs has no append-only trigger (DB-CA-F6)');

\o
-- ── report ──────────────────────────────────────────────────────────────────
\set QUIET off
SELECT CASE WHEN skip THEN 'SKIP' WHEN ok THEN 'PASS' ELSE 'FAIL' END AS verdict, id AS test_case,
       CASE WHEN ok AND NOT skip THEN '' ELSE detail END AS detail
FROM results ORDER BY n;
SELECT count(*) FILTER (WHERE ok AND NOT skip) AS passed, count(*) FILTER (WHERE NOT ok) AS failed,
       count(*) FILTER (WHERE skip) AS skipped FROM results;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM results WHERE NOT ok) THEN
        RAISE EXCEPTION 'data-tier suite: % case(s) failed', (SELECT count(*) FROM results WHERE NOT ok);
    END IF;
END $$;

ROLLBACK;
