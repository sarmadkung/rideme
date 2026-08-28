-- Identity, authentication and authorization (documents 13, 20, 28).
--
-- One users table serves every actor — customer, driver, merchant, support,
-- admin. Document 28 states a user may hold more than one role, so roles are a
-- separate table rather than a column: a driver who also orders groceries is
-- one person, not two accounts.

CREATE TABLE users (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    -- Stored normalised to E.164 so lookup is exact; document 28 requires
    -- normalisation before lookup.
    phone       text        NOT NULL UNIQUE,
    email       text,
    name        text,
    avatar_url  text,
    status      text        NOT NULL DEFAULT 'ACTIVE',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_status_valid CHECK (status IN ('ACTIVE', 'SUSPENDED', 'DELETED'))
);

-- Document 20 fixes the six roles.
CREATE TABLE user_roles (
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role       text        NOT NULL,
    granted_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, role),
    CONSTRAINT user_roles_role_valid CHECK (
        role IN ('CUSTOMER', 'DRIVER', 'MERCHANT', 'SUPPORT', 'ADMIN', 'SUPER_ADMIN')
    )
);

-- Device signals for session trust (document 116). Deliberately minimal: the
-- document warns against collecting unnecessary fingerprinting data.
CREATE TABLE devices (
    id                uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    device_identifier text        NOT NULL,
    platform          text,
    os                text,
    app_version       text,
    created_at        timestamptz NOT NULL DEFAULT now(),
    last_seen_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, device_identifier)
);

-- Sessions carry exactly the fields document 28 lists.
--
-- The refresh token is stored as a hash. A stolen database dump must not be a
-- set of usable credentials, and the platform never needs the original value —
-- it only ever compares.
CREATE TABLE sessions (
    id                 uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    device_id          uuid        REFERENCES devices (id) ON DELETE SET NULL,
    refresh_token_hash bytea       NOT NULL UNIQUE,
    created_at         timestamptz NOT NULL DEFAULT now(),
    last_used_at       timestamptz NOT NULL DEFAULT now(),
    expires_at         timestamptz NOT NULL,
    revoked_at         timestamptz,
    revoked_reason     text
);

CREATE INDEX sessions_user_active_idx ON sessions (user_id) WHERE revoked_at IS NULL;

-- OTP challenges. The code is never stored in plaintext (documents 28, 123);
-- what is stored is a keyed hash, which is enough to verify and useless to
-- read.
CREATE TABLE otp_challenges (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    phone        text        NOT NULL,
    purpose      text        NOT NULL,
    code_hash    bytea       NOT NULL,
    attempts     integer     NOT NULL DEFAULT 0,
    max_attempts integer     NOT NULL,
    expires_at   timestamptz NOT NULL,
    consumed_at  timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT otp_purpose_valid CHECK (purpose IN ('LOGIN', 'PHONE_CHANGE', 'STEP_UP'))
);

-- Only one live challenge per phone and purpose: a second request supersedes
-- the first rather than leaving two valid codes in flight.
CREATE UNIQUE INDEX otp_challenges_live_idx
    ON otp_challenges (phone, purpose)
    WHERE consumed_at IS NULL;

-- Rotated refresh tokens, kept so that reuse can be *detected* rather than
-- merely refused.
--
-- Without this table a replayed old token is indistinguishable from a random
-- invalid one: rotation has already removed it from sessions, so the lookup
-- simply misses and the theft goes unnoticed. Keeping the superseded hashes
-- means a token that was valid once and is presented again identifies the
-- session it came from, which is the signal document 28 calls "refresh-token
-- reuse".
--
-- Rows are only useful until they pass the refresh token lifetime; pruning is
-- a scheduled job (Phase 14), not a startup task.
CREATE TABLE refresh_token_history (
    token_hash bytea       PRIMARY KEY,
    session_id uuid        NOT NULL REFERENCES sessions (id) ON DELETE CASCADE,
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    rotated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX refresh_token_history_rotated_idx ON refresh_token_history (rotated_at);

-- The audit trail document 28 requires. Append-only by convention here and by
-- permission in production; nothing in the application updates or deletes.
CREATE TABLE security_events (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid        REFERENCES users (id) ON DELETE SET NULL,
    session_id uuid,
    device_id  uuid,
    event      text        NOT NULL,
    ip         inet,
    metadata   jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX security_events_user_idx ON security_events (user_id, created_at DESC);
CREATE INDEX security_events_event_idx ON security_events (event, created_at DESC);
