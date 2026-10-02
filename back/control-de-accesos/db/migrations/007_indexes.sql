-- Las comprobaciones de pertenencia (en CADA petición) buscan por user_id, pero la
-- clave primaria de unit_members es (unit_id, user_id): sin este índice cada petición
-- recorre la tabla entera.
CREATE INDEX IF NOT EXISTS idx_unit_members_user ON unit_members (user_id);
