import { pool } from "./db.js";
import { errors } from "./errors.js";

function normalizePlate(raw) {
  return String(raw || "")
    .toUpperCase()
    .trim()
    .replace(/[\s\-_.\t]/g, "");
}

async function unitInComplex(complexId, unitId) {
  const { rows } = await pool.query(
    `SELECT EXISTS (
       SELECT 1 FROM ref_units WHERE id = $1 AND complex_id = $2
     ) AS ok`,
    [unitId, complexId],
  );
  return rows[0].ok;
}

async function assertUnitAccess(userId, unitId) {
  const { rows } = await pool.query(
    `SELECT EXISTS (
       SELECT 1 FROM ref_unit_members WHERE user_id = $1 AND unit_id = $2
     ) AS ok`,
    [userId, unitId],
  );
  if (!rows[0].ok) throw errors.unitAccessDenied();
}

async function lookupActive(complexId, plate, now) {
  const { rows } = await pool.query(
    `SELECT p.id, p.visitor_name, COALESCE(u.full_name, '') AS host_name,
            COALESCE(un.name, '') AS unit_number, p.valid_until
     FROM visitor_vehicle_preauths p
     JOIN ref_users u ON u.id = p.host_user_id
     LEFT JOIN ref_units un ON un.id = p.unit_id
     WHERE p.complex_id = $1 AND p.plate = $2 AND p.status = 'active'
       AND p.valid_from <= $3 AND p.valid_until >= $3
     ORDER BY p.valid_until DESC
     LIMIT 1`,
    [complexId, plate, now],
  );
  return rows[0] || null;
}

export async function createPreauth(actor, body) {
  const plate = normalizePlate(body.plate);
  if (!plate) throw errors.invalidPlate();
  const name = String(body.visitor_name || "").trim();
  if (!name) throw errors.visitorNameRequired();
  if (!body.unit_id) throw errors.unitNotInComplex();

  const now = new Date();
  const validFrom = body.valid_from ? new Date(body.valid_from) : now;
  const validUntil = body.valid_until ? new Date(body.valid_until) : null;
  if (!validUntil || !(validUntil > validFrom) || !(validUntil > now)) {
    throw errors.invalidWindow();
  }
  if (!(await unitInComplex(actor.complexId, body.unit_id))) {
    throw errors.unitNotInComplex();
  }
  await assertUnitAccess(actor.userId, body.unit_id);

  const { rows } = await pool.query(
    `INSERT INTO visitor_vehicle_preauths
       (complex_id, unit_id, host_user_id, plate, visitor_name, valid_from, valid_until)
     VALUES ($1, $2, $3, $4, $5, $6, $7)
     RETURNING id, complex_id, unit_id, host_user_id, plate, visitor_name,
               valid_from, valid_until, status, created_at`,
    [actor.complexId, body.unit_id, actor.userId, plate, name, validFrom, validUntil],
  );
  return rows[0];
}

export async function listPreauths(actor) {
  const { rows } = await pool.query(
    `SELECT id, complex_id, unit_id, host_user_id, plate, visitor_name,
            valid_from, valid_until, status, created_at
     FROM visitor_vehicle_preauths
     WHERE complex_id = $1 AND host_user_id = $2 AND status = 'active'
     ORDER BY valid_until DESC
     LIMIT 200`,
    [actor.complexId, actor.userId],
  );
  return rows;
}

export async function revokePreauth(actor, id) {
  const result = await pool.query(
    `UPDATE visitor_vehicle_preauths SET status = 'revoked'
     WHERE id = $1 AND complex_id = $2 AND host_user_id = $3 AND status = 'active'`,
    [id, actor.complexId, actor.userId],
  );
  if (result.rowCount === 0) throw errors.preauthNotFound();
}

export async function lookupPlate(actor, rawPlate) {
  const plate = normalizePlate(rawPlate);
  if (!plate) throw errors.invalidPlate();
  const match = await lookupActive(actor.complexId, plate, new Date());
  return {
    plate,
    found: false,
    visitor_preauth: match,
  };
}

export async function logAccess(actor, body) {
  const plate = normalizePlate(body.plate);
  if (!plate) throw errors.invalidPlate();
  const direction = String(body.direction || "").trim().toLowerCase();
  if (direction !== "in" && direction !== "out") throw errors.invalidDirection();

  const match = await lookupActive(actor.complexId, plate, new Date());
  const note = String(body.note || "").trim() || null;
  const { rows } = await pool.query(
    `INSERT INTO visitor_vehicle_access_log
       (complex_id, plate, preauth_id, direction, guard_id, note)
     VALUES ($1, $2, $3, $4, $5, $6)
     RETURNING id, complex_id, plate, preauth_id, direction, guard_id, note, created_at`,
    [actor.complexId, plate, match?.id || null, direction, actor.userId, note],
  );
  return { ...rows[0], visitor_preauth: match };
}

export async function listAccessLog(actor) {
  const { rows } = await pool.query(
    `SELECT l.id, l.complex_id, l.plate, l.preauth_id, l.direction, l.guard_id, l.note, l.created_at,
            p.visitor_name, COALESCE(u.full_name, '') AS host_name, COALESCE(un.name, '') AS unit_number
     FROM visitor_vehicle_access_log l
     LEFT JOIN visitor_vehicle_preauths p ON p.id = l.preauth_id
     LEFT JOIN ref_users u ON u.id = p.host_user_id
     LEFT JOIN ref_units un ON un.id = p.unit_id
     WHERE l.complex_id = $1
     ORDER BY l.created_at DESC
     LIMIT 50`,
    [actor.complexId],
  );
  return rows.map((row) => ({
    id: row.id,
    complex_id: row.complex_id,
    plate: row.plate,
    preauth_id: row.preauth_id,
    direction: row.direction,
    guard_id: row.guard_id,
    note: row.note,
    created_at: row.created_at,
    visitor_preauth: row.preauth_id
      ? {
          id: row.preauth_id,
          visitor_name: row.visitor_name,
          host_name: row.host_name,
          unit_number: row.unit_number,
        }
      : null,
  }));
}
