-- notificaciones: esquema inicial.
-- Portado desde el monolito residential-superapp (notifications,
-- notification_preferences, user_devices). Esta base de datos es exclusiva del
-- feature: ningún otro servicio la lee ni escribe.
--
-- Requiere PostgreSQL 18+ (uuidv7()).

-- Bandeja de entrada persistente. Es la fuente de verdad; el push por FCM es
-- un canal secundario best-effort cuyo resultado queda en delivery_status.
CREATE TABLE notifications (
    id              uuid        PRIMARY KEY DEFAULT uuidv7(),
    complex_id      uuid,                                   -- NULL = notificación a nivel de cuenta
    user_id         uuid        NOT NULL,
    title           text        NOT NULL,
    body            text        NOT NULL,
    data            jsonb       NOT NULL DEFAULT '{}'::jsonb,
    type            text        NOT NULL DEFAULT 'GENERAL', -- categoría (ver preferences.Category*)
    source_event_id text        NOT NULL,                   -- evento de Redis que la originó
    is_read         boolean     NOT NULL DEFAULT false,
    read_at         timestamptz,
    delivery_status text        NOT NULL DEFAULT 'INBOX_ONLY',
    delivered_at    timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT notifications_delivery_status_check CHECK (
        delivery_status IN ('INBOX_ONLY', 'PUSH_SENT', 'PUSH_PARTIAL', 'PUSH_FAILED')
    )
);

-- Bandeja del usuario: filtra por user_id y lee en orden de created_at, así
-- el LIMIT corta temprano. El conjunto (o NULL = cuenta) se filtra encima.
CREATE INDEX idx_notifications_user_created
    ON notifications (user_id, created_at DESC);

-- Idempotencia del consumidor de eventos: un event_id se procesa una sola vez
-- aunque Redis lo entregue más de una vez (at-least-once).
CREATE TABLE processed_events (
    event_id     text        PRIMARY KEY,
    source       text,
    processed_at timestamptz NOT NULL DEFAULT now()
);

-- Preferencias por usuario y categoría (modelo opt-out: sin fila = habilitada).
-- Las categorías críticas (SOS, SECURITY, EMERGENCY, CALL) nunca se guardan
-- aquí; el servicio las rechaza.
CREATE TABLE notification_preferences (
    id          uuid        PRIMARY KEY DEFAULT uuidv7(),
    user_id     uuid        NOT NULL,
    complex_id  uuid,                                       -- solo auditoría; no es parte de la llave
    category    text        NOT NULL,
    enabled     boolean     NOT NULL DEFAULT true,
    quiet_start integer,                                    -- minuto del día [0, 1440)
    quiet_end   integer,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT notification_preferences_user_category_key UNIQUE (user_id, category),
    CONSTRAINT notification_preferences_quiet_start_check CHECK (
        quiet_start IS NULL OR (quiet_start >= 0 AND quiet_start < 1440)
    ),
    CONSTRAINT notification_preferences_quiet_end_check CHECK (
        quiet_end IS NULL OR (quiet_end >= 0 AND quiet_end < 1440)
    ),
    CONSTRAINT notification_preferences_quiet_pair_check CHECK (
        (quiet_start IS NULL) = (quiet_end IS NULL)
    )
);

-- Tokens de push de los dispositivos. En el monolito vivían en identity; aquí
-- son propiedad de notificaciones porque solo este servicio envía push.
CREATE TABLE user_devices (
    id           uuid        PRIMARY KEY DEFAULT uuidv7(),
    user_id      uuid        NOT NULL,
    platform     text        NOT NULL,
    push_token   text        NOT NULL,
    device_name  text,
    app_version  text,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    created_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT user_devices_push_token_key UNIQUE (push_token),
    CONSTRAINT user_devices_platform_check CHECK (platform IN ('IOS', 'ANDROID', 'WEB'))
);

CREATE INDEX idx_user_devices_user ON user_devices (user_id);
