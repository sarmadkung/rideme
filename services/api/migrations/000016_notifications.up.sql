-- Notifications (documents 121, 122, 124).
--
-- Document 121's principle is the shape of this schema: "Business services
-- emit events. They should not directly call Twilio, Firebase, email providers
-- or other channel vendors." So a notification is a row first and a provider
-- call second, and the provider is an adapter behind a queue rather than a
-- function a booking service reaches for.

-- --- devices (document 122) --------------------------------------------------

-- "A user may have multiple active devices. Do not assume one user equals one
-- push token." The key is therefore (user, device), not the user.
CREATE TABLE device_tokens (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    device_id   text        NOT NULL,
    platform    text        NOT NULL,
    push_token  text        NOT NULL,
    app_version text,
    status      text        NOT NULL DEFAULT 'ACTIVE',
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT device_tokens_platform_valid CHECK (platform IN ('IOS', 'ANDROID', 'WEB')),
    -- RETIRED is a token the provider rejected as unregistered; REVOKED is one
    -- the user signed out of. Both stop delivery, and keeping them apart is
    -- what lets "this person turned it off" be told from "this phone is gone".
    CONSTRAINT device_tokens_status_valid CHECK (status IN ('ACTIVE', 'RETIRED', 'REVOKED'))
);

-- One row per device per user: re-registering the same device replaces its
-- token rather than accumulating dead ones.
CREATE UNIQUE INDEX device_tokens_user_device_idx ON device_tokens (user_id, device_id);
-- A push token belongs to one device. When a phone is handed on, the new
-- owner's registration must take the token rather than leaving it delivering
-- to the previous account.
CREATE UNIQUE INDEX device_tokens_token_idx ON device_tokens (push_token) WHERE status = 'ACTIVE';
CREATE INDEX device_tokens_active_idx ON device_tokens (user_id) WHERE status = 'ACTIVE';

-- --- preferences (document 124) ----------------------------------------------

-- Stored as opt-OUT rows. A user who has never touched a setting has no rows
-- and receives everything they are eligible for, which is what a new account
-- should do; the alternative would be a signup that must write a dozen rows
-- before the first notification can be sent.
CREATE TABLE notification_preferences (
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    channel    text        NOT NULL,
    category   text        NOT NULL,
    enabled    boolean     NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, channel, category),
    CONSTRAINT notification_preferences_channel_valid CHECK (channel IN ('PUSH', 'SMS', 'EMAIL', 'IN_APP')),
    CONSTRAINT notification_preferences_category_valid CHECK (category IN (
        'RIDE', 'DELIVERY', 'ORDER', 'PAYMENT', 'SAFETY', 'SUPPORT', 'MARKETING'))
);

-- --- the queue (document 122's reliability states) ---------------------------

CREATE TABLE notifications (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    device_id    uuid        REFERENCES device_tokens (id) ON DELETE SET NULL,
    channel      text        NOT NULL,
    category     text        NOT NULL,
    title        text        NOT NULL,
    body         text        NOT NULL,
    -- Document 122: "Notifications should open the relevant application screen
    -- using stable deep-link routes."
    deep_link    text,
    data         jsonb       NOT NULL DEFAULT '{}'::jsonb,
    status       text        NOT NULL DEFAULT 'QUEUED',
    attempts     integer     NOT NULL DEFAULT 0,
    provider_reference text,
    failure_reason text,
    -- What produced it, so a duplicate event cannot produce a duplicate buzz.
    idempotency_key text UNIQUE,
    created_at   timestamptz NOT NULL DEFAULT now(),
    sent_at      timestamptz,
    CONSTRAINT notifications_channel_valid CHECK (channel IN ('PUSH', 'SMS', 'EMAIL', 'IN_APP')),
    CONSTRAINT notifications_category_valid CHECK (category IN (
        'RIDE', 'DELIVERY', 'ORDER', 'PAYMENT', 'SAFETY', 'SUPPORT', 'MARKETING')),
    -- Document 122's list, plus SUPPRESSED. A notification a preference
    -- stopped is recorded rather than dropped: "why did I not get told?" is a
    -- support question, and an absent row cannot answer it.
    CONSTRAINT notifications_status_valid CHECK (status IN (
        'QUEUED', 'SENDING', 'SENT', 'DELIVERED', 'FAILED', 'SUPPRESSED'))
);

-- Drives the send pass, oldest first.
CREATE INDEX notifications_pending_idx ON notifications (created_at)
    WHERE status IN ('QUEUED', 'SENDING');
CREATE INDEX notifications_user_idx ON notifications (user_id, created_at DESC);
