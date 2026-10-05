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

-- Preferencias por grupo (modelo opt-out: sin fila = habilitado). Solo se
-- guardan los grupos configurables (RESERVATIONS, COMMUNITY, ASSEMBLIES, VOTES);
-- ACCESS_CONTROL, INTERCOM y SYSTEM siempre se entregan.
CREATE TABLE notification_preferences (
    user_id    uuid        NOT NULL,
    group_name text        NOT NULL,
    enabled    boolean     NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, group_name)
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
