import express from "express";
import cors from "cors";
import { applySchema, connectWithRetry, pool } from "./db.js";
import { errors, sendError } from "./errors.js";
import * as passes from "./passes.js";
import * as vehicles from "./vehicles.js";

const app = express();
app.use(cors());
app.use(express.json());

const port = Number(process.env.PORT || 4000);

async function actorFrom(req) {
  const complexId = req.header("x-complex-id");
  const userId = req.header("x-user-id");
  if (!complexId || !userId) throw errors.unauthorized();

  const user = await pool.query(
    `SELECT id, full_name FROM ref_users WHERE id = $1 AND status = 'ACTIVE'`,
    [userId],
  );
  if (!user.rows[0]) throw errors.unauthorized();

  const staff = await pool.query(
    `SELECT role FROM ref_complex_staff WHERE complex_id = $1 AND user_id = $2`,
    [complexId, userId],
  );
  const member = await pool.query(
    `SELECT unit_id, role FROM ref_unit_members WHERE complex_id = $1 AND user_id = $2`,
    [complexId, userId],
  );

  const isGuard = staff.rows.some((row) => row.role === "GUARD");
  return {
    complexId,
    userId,
    fullName: user.rows[0].full_name,
    role: isGuard ? "guard" : "resident",
    unitIds: member.rows.map((row) => row.unit_id),
  };
}

function guard(handler) {
  return async (req, res) => {
    try {
      await handler(req, res);
    } catch (error) {
      sendError(res, error);
    }
  };
}

async function requireRole(req, role) {
  const actor = await actorFrom(req);
  if (actor.role !== role) throw errors.forbiddenRole();
  return actor;
}

app.get("/health", (_req, res) => {
  res.json({ ok: true, module: "control-de-accesos" });
});

app.get(
  "/integration/context",
  guard(async (_req, res) => {
    const complex = await pool.query(
      `SELECT id, name, address, timezone FROM ref_complexes WHERE is_active ORDER BY name`,
    );
    const people = await pool.query(
      `SELECT u.id, u.full_name, u.email,
              CASE WHEN s.role = 'GUARD' THEN 'guard' ELSE 'resident' END AS role,
              m.unit_id,
              un.name AS unit_name,
              sec.name AS section_name
       FROM ref_users u
       LEFT JOIN ref_complex_staff s ON s.user_id = u.id AND s.role = 'GUARD'
       LEFT JOIN ref_unit_members m ON m.user_id = u.id
       LEFT JOIN ref_units un ON un.id = m.unit_id
       LEFT JOIN ref_sections sec ON sec.id = un.section_id
       WHERE s.user_id IS NOT NULL OR m.user_id IS NOT NULL
       ORDER BY role, u.full_name`,
    );
    res.json({ complexes: complex.rows, people: people.rows });
  }),
);

app.get(
  "/integration/notifications",
  guard(async (req, res) => {
    const actor = await actorFrom(req);
    const { rows } = await pool.query(
      `SELECT id, title, body, type, created_at
       FROM integration_notifications
       WHERE complex_id = $1 AND user_id = $2
       ORDER BY created_at DESC
       LIMIT 20`,
      [actor.complexId, actor.userId],
    );
    res.json(rows);
  }),
);

app.post(
  "/visitor-passes",
  guard(async (req, res) => {
    const actor = await requireRole(req, "resident");
    res.status(201).json(await passes.createPass(actor, req.body));
  }),
);
app.get(
  "/visitor-passes/units/:unitId",
  guard(async (req, res) => {
    const actor = await requireRole(req, "resident");
    res.json(await passes.listUnitPasses(actor, req.params.unitId));
  }),
);
app.get(
  "/visitor-passes/checkpoint-activity",
  guard(async (req, res) => {
    const actor = await requireRole(req, "guard");
    res.json(await passes.checkpointActivity(actor, req.query.limit));
  }),
);
app.post(
  "/visitor-passes/validate",
  guard(async (req, res) => {
    const actor = await requireRole(req, "guard");
    res.json(await passes.validateToken(actor, req.body));
  }),
);
app.post(
  "/visitor-passes/check-out",
  guard(async (req, res) => {
    const actor = await requireRole(req, "guard");
    res.json(await passes.checkOutToken(actor, req.body));
  }),
);
app.get(
  "/visitor-passes/:id",
  guard(async (req, res) => {
    const actor = await requireRole(req, "resident");
    res.json(await passes.getPass(actor, req.params.id));
  }),
);
app.post(
  "/visitor-passes/:id/revoke",
  guard(async (req, res) => {
    const actor = await requireRole(req, "resident");
    res.json(await passes.revokePass(actor, req.params.id));
  }),
);
app.post(
  "/visitor-passes/:id/extend",
  guard(async (req, res) => {
    const actor = await requireRole(req, "resident");
    res.json(await passes.extendPass(actor, req.params.id, req.body));
  }),
);

app.get(
  "/vehicles/lookup",
  guard(async (req, res) => {
    const actor = await requireRole(req, "guard");
    res.json(await vehicles.lookupPlate(actor, req.query.plate));
  }),
);
app.post(
  "/vehicles/access-log",
  guard(async (req, res) => {
    const actor = await requireRole(req, "guard");
    res.status(201).json(await vehicles.logAccess(actor, req.body));
  }),
);
app.get(
  "/vehicles/access-log",
  guard(async (req, res) => {
    const actor = await requireRole(req, "guard");
    res.json(await vehicles.listAccessLog(actor));
  }),
);
app.post(
  "/vehicles/visitor-preauth",
  guard(async (req, res) => {
    const actor = await requireRole(req, "resident");
    res.status(201).json(await vehicles.createPreauth(actor, req.body));
  }),
);
app.get(
  "/vehicles/visitor-preauth",
  guard(async (req, res) => {
    const actor = await requireRole(req, "resident");
    res.json(await vehicles.listPreauths(actor));
  }),
);
app.delete(
  "/vehicles/visitor-preauth/:id",
  guard(async (req, res) => {
    const actor = await requireRole(req, "resident");
    await vehicles.revokePreauth(actor, req.params.id);
    res.status(204).end();
  }),
);

const server = await (async () => {
  await connectWithRetry();
  await applySchema();
  return app.listen(port, () => {
    console.log(`control-de-accesos listening on ${port}`);
  });
})();

process.on("SIGINT", async () => {
  server.close();
  await pool.end();
  process.exit(0);
});
