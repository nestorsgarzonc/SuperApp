import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import pg from "pg";

const { Pool } = pg;

const databaseUrl =
  process.env.DATABASE_URL ||
  "postgres://accesos:accesos@localhost:5433/control_accesos";

export const pool = new Pool({ connectionString: databaseUrl });

const dbDir = path.join(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
  "..",
  "db",
);

export async function applySchema() {
  const existing = await pool.query(`SELECT to_regclass('public.visitor_passes') AS name`);
  if (existing.rows[0].name) return;
  const schema = fs.readFileSync(path.join(dbDir, "001_schema.sql"), "utf8");
  const seed = fs.readFileSync(path.join(dbDir, "002_seed.sql"), "utf8");
  await pool.query(schema);
  await pool.query(seed);
}

export async function connectWithRetry(attempts = 20) {
  let lastError;
  for (let i = 0; i < attempts; i += 1) {
    try {
      await pool.query("SELECT 1");
      return;
    } catch (error) {
      lastError = error;
      await new Promise((resolve) => setTimeout(resolve, 1500));
    }
  }
  throw lastError;
}
