-- Zona horaria por usuario para evaluar horas de silencio en hora local.
-- Sin fila = DEFAULT_TIMEZONE del servicio (America/Bogota).
CREATE TABLE notification_settings (
    user_id    uuid        PRIMARY KEY,
    timezone   text        NOT NULL,                -- nombre IANA, p. ej. America/Bogota
    updated_at timestamptz NOT NULL DEFAULT now()
);
