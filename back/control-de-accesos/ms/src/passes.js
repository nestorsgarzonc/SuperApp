import { randomBytes } from "node:crypto";
import { pool } from "./db.js";
import { errors } from "./errors.js";

const MAX_VALIDITY_MS = 30 * 24 * 60 * 60 * 1000;

const passColumns = `id, complex_id, unit_id, host_user_id,
  visitor_name, COALESCE(document, '') AS document, token,
  valid_from, valid_until, status, used_by, used_at,
  checked_out_by, checked_out_at, created_at, updated_at`;

function withoutToken(pass) {
  if (!pass) return pass;
  const copy = { ...pass };
  delete copy.token;
  return copy;
}

function effectiveStatus(pass, now) {
  if (pass.status === "active" && new Date(pass.valid_until) <= now) {
    return "expired";
  }
  return pass.status;
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

async function notify(pass, title, body) {
  await pool.query(
    `INSERT INTO integration_notifications (complex_id, user_id, title, body, type)
     VALUES ($1, $2, $3, $4, 'VISITOR_PASS')`,
    [pass.complex_id, pass.host_user_id, title, body],
  );
}

export async function createPass(actor, body) {
  const name = String(body.visitor_name || "").trim();
  if (!name) throw errors.visitorNameRequired();
  if (!body.unit_id) throw errors.unitNotInComplex();

  const now = new Date();
  const validFrom = body.valid_from ? new Date(body.valid_from) : now;
  const validUntil = body.valid_until ? new Date(body.valid_until) : null;
  if (!validUntil || !(validUntil > validFrom)) throw errors.invalidWindow();
  if (validUntil.getTime() - validFrom.getTime() > MAX_VALIDITY_MS) {
    throw errors.invalidWindow();
  }
  if (!(await unitInComplex(actor.complexId, body.unit_id))) {
    throw errors.unitNotInComplex();
  }
  await assertUnitAccess(actor.userId, body.unit_id);

  const token = randomBytes(32).toString("base64url");
  const document = String(body.document || "").trim() || null;
  const { rows } = await pool.query(
    `INSERT INTO visitor_passes
       (complex_id, unit_id, host_user_id, visitor_name, document, token, valid_from, valid_until, status)
     VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'active')
     RETURNING ${passColumns}`,
    [actor.complexId, body.unit_id, actor.userId, name, document, token, validFrom, validUntil],
  );
  return rows[0];
}

export async function getPass(actor, id) {
  const { rows } = await pool.query(
    `SELECT ${passColumns} FROM visitor_passes WHERE id = $1 AND complex_id = $2`,
    [id, actor.complexId],
  );
  if (!rows[0]) throw errors.notFound();
  await assertUnitAccess(actor.userId, rows[0].unit_id);
  const pass = rows[0];
  pass.status = effectiveStatus(pass, new Date());
  return pass;
}

export async function listUnitPasses(actor, unitId) {
  await assertUnitAccess(actor.userId, unitId);
  const now = new Date();
  await pool.query(
    `UPDATE visitor_passes SET status = 'expired'
     WHERE status = 'active' AND valid_until <= $1`,
    [now],
  );
  const { rows } = await pool.query(
    `SELECT ${passColumns}
     FROM visitor_passes
     WHERE complex_id = $1 AND unit_id = $2
     ORDER BY created_at DESC
     LIMIT 50`,
    [actor.complexId, unitId],
  );
  return rows.map((pass) => ({ ...pass, status: effectiveStatus(pass, now) }));
}

export async function revokePass(actor, id) {
  const existing = await getPass(actor, id);
  const { rows } = await pool.query(
    `UPDATE visitor_passes SET status = 'revoked'
     WHERE id = $1 AND complex_id = $2 AND status = 'active'
     RETURNING ${passColumns}`,
    [existing.id, actor.complexId],
  );
  if (!rows[0]) throw errors.passNotActive();
  return rows[0];
}

export async function extendPass(actor, id, body) {
  const existing = await getPass(actor, id);
  const now = new Date();
  const next = body.valid_until ? new Date(body.valid_until) : null;
  if (existing.status !== "active" || new Date(existing.valid_until) <= now) {
    throw errors.passNotExtendable();
  }
  if (
    !next ||
    !(next > new Date(existing.valid_until)) ||
    !(next > now) ||
    next.getTime() - new Date(existing.valid_from).getTime() > MAX_VALIDITY_MS
  ) {
    throw errors.invalidExtension();
  }
  const { rows } = await pool.query(
    `UPDATE visitor_passes SET valid_until = $1
     WHERE id = $2 AND complex_id = $3
       AND status = 'active' AND valid_until > $4 AND valid_until < $1
     RETURNING ${passColumns}`,
    [next, existing.id, actor.complexId, now],
  );
  if (!rows[0]) throw errors.passNotExtendable();
  return rows[0];
}

async function passByToken(complexId, token) {
  const { rows } = await pool.query(
    `SELECT ${passColumns} FROM visitor_passes WHERE token = $1 AND complex_id = $2`,
    [token, complexId],
  );
  if (!rows[0]) throw errors.invalidToken();
  return rows[0];
}

export async function validateToken(actor, body) {
  const token = String(body.token || "").trim();
  if (!token) throw errors.invalidToken();
  const now = new Date();
  const pass = await passByToken(actor.complexId, token);
  if (pass.status === "used" || pass.status === "revoked") throw errors.passNotActive();
  if (pass.status === "expired") throw errors.passExpired();
  if (pass.status === "checked_out") throw errors.passNotActive();
  if (now < new Date(pass.valid_from)) throw errors.passNotYetValid();
  if (now >= new Date(pass.valid_until)) throw errors.passExpired();

  const { rows } = await pool.query(
    `UPDATE visitor_passes
     SET status = 'used', used_by = $1, used_at = $2
     WHERE id = $3 AND complex_id = $4
       AND status = 'active' AND valid_from <= $2 AND valid_until > $2
     RETURNING ${passColumns}`,
    [actor.userId, now, pass.id, actor.complexId],
  );
  if (!rows[0]) throw errors.passNotActive();
  await notify(rows[0], "Your guest has arrived", `${rows[0].visitor_name} has arrived and was admitted at the entrance.`);
  return withoutToken(rows[0]);
}

export async function checkOutToken(actor, body) {
  const token = String(body.token || "").trim();
  if (!token) throw errors.invalidToken();
  const pass = await passByToken(actor.complexId, token);
  if (pass.status !== "used") throw errors.passNotCheckedIn();
  const now = new Date();
  const { rows } = await pool.query(
    `UPDATE visitor_passes
     SET status = 'checked_out', checked_out_by = $1, checked_out_at = $2
     WHERE id = $3 AND complex_id = $4 AND status = 'used'
     RETURNING ${passColumns}`,
    [actor.userId, now, pass.id, actor.complexId],
  );
  if (!rows[0]) throw errors.passNotCheckedIn();
  await notify(rows[0], "Your guest has left", `${rows[0].visitor_name} has left the premises.`);
  return withoutToken(rows[0]);
}

export async function checkpointActivity(actor, limit) {
  const size = Math.min(Math.max(Number(limit) || 10, 1), 50);
  const now = new Date();
  const { rows } = await pool.query(
    `WITH checkpoint_events AS (
       SELECT vp.id AS pass_id, 'in'::text AS direction,
              CONCAT_WS(' - ', NULLIF(s.name, ''), u.name) AS unit_label,
              vp.used_at AS occurred_at
       FROM visitor_passes vp
       JOIN ref_units u ON u.id = vp.unit_id
       LEFT JOIN ref_sections s ON s.id = u.section_id
       WHERE vp.complex_id = $1 AND vp.used_at IS NOT NULL
       UNION ALL
       SELECT vp.id, 'out'::text,
              CONCAT_WS(' - ', NULLIF(s.name, ''), u.name),
              vp.checked_out_at
       FROM visitor_passes vp
       JOIN ref_units u ON u.id = vp.unit_id
       LEFT JOIN ref_sections s ON s.id = u.section_id
       WHERE vp.complex_id = $1 AND vp.checked_out_at IS NOT NULL
     )
     SELECT pass_id, direction, unit_label, occurred_at
     FROM checkpoint_events
     ORDER BY occurred_at DESC
     LIMIT $2`,
    [actor.complexId, size],
  );
  const counts = await pool.query(
    `SELECT
       COUNT(*) FILTER (WHERE used_at >= $2)::int AS entries_last_24h,
       COUNT(*) FILTER (WHERE checked_out_at >= $2)::int AS exits_last_24h,
       COUNT(*) FILTER (WHERE status = 'used')::int AS currently_inside
     FROM visitor_passes
     WHERE complex_id = $1`,
    [actor.complexId, new Date(now.getTime() - 24 * 60 * 60 * 1000)],
  );
  return {
    activities: rows,
    ...counts.rows[0],
    generated_at: now.toISOString(),
  };
}
