-- Merchant platform and grocery (documents 65-78).
--
-- Document 070 is explicit: "Order and delivery state remain separate and
-- communicate through explicit events." A grocery order is not a Job — it is
-- the merchant's fulfilment, and it *produces* a delivery Job when it is ready
-- for pickup. Collapsing the two would mean a merchant's preparation states
-- living in the job lifecycle every ride also uses.

CREATE TABLE merchants (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_user_id uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    name          text        NOT NULL,
    status        text        NOT NULL DEFAULT 'PENDING_VERIFICATION',
    phone         text,
    address       text,
    location      geography(Point, 4326),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT merchants_status_valid CHECK (status IN (
        'PENDING_VERIFICATION', 'ACTIVE', 'SUSPENDED', 'CLOSED'))
);

CREATE INDEX merchants_owner_idx ON merchants (owner_user_id);
CREATE INDEX merchants_location_idx ON merchants USING GIST (location);

-- Now that merchants exist, the column jobs has carried since 000003 can be a
-- real foreign key.
ALTER TABLE jobs
    ADD CONSTRAINT jobs_merchant_fk FOREIGN KEY (merchant_id) REFERENCES merchants (id) ON DELETE SET NULL;

CREATE TABLE stores (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id uuid        NOT NULL REFERENCES merchants (id) ON DELETE CASCADE,
    name        text        NOT NULL,
    address     text,
    location    geography(Point, 4326),
    status      text        NOT NULL DEFAULT 'OPEN',
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT stores_status_valid CHECK (status IN ('OPEN', 'CLOSED', 'PAUSED'))
);

CREATE INDEX stores_merchant_idx ON stores (merchant_id);
CREATE INDEX stores_location_idx ON stores USING GIST (location);

-- Operating hours, per store and weekday.
CREATE TABLE store_hours (
    store_id   uuid    NOT NULL REFERENCES stores (id) ON DELETE CASCADE,
    weekday    integer NOT NULL,
    opens_at   time    NOT NULL,
    closes_at  time    NOT NULL,
    PRIMARY KEY (store_id, weekday, opens_at),
    CONSTRAINT store_hours_weekday_valid CHECK (weekday BETWEEN 0 AND 6),
    CONSTRAINT store_hours_ordered CHECK (closes_at > opens_at)
);

-- --- catalog (document 68) ---------------------------------------------------

CREATE TABLE categories (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id uuid    NOT NULL REFERENCES merchants (id) ON DELETE CASCADE,
    name        text    NOT NULL,
    sort_order  integer NOT NULL DEFAULT 0
);

CREATE TABLE products (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id  uuid        NOT NULL REFERENCES merchants (id) ON DELETE CASCADE,
    category_id  uuid        REFERENCES categories (id) ON DELETE SET NULL,
    name         text        NOT NULL,
    description  text,
    sku          text,
    -- Integer minor units with a currency (ADR-008). A catalogue price is
    -- money like any other.
    price_minor  bigint      NOT NULL,
    currency     text        NOT NULL DEFAULT 'PKR',
    status       text        NOT NULL DEFAULT 'DRAFT',
    image_key    text,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT products_status_valid CHECK (status IN (
        'DRAFT', 'ACTIVE', 'OUT_OF_STOCK', 'DISABLED', 'ARCHIVED')),
    CONSTRAINT products_price_nonneg CHECK (price_minor >= 0),
    CONSTRAINT products_currency_valid CHECK (currency = 'PKR'),
    UNIQUE (merchant_id, sku)
);

CREATE INDEX products_merchant_status_idx ON products (merchant_id, status);

-- Variants: Milk → 1L / 2L / 5L (document 68). A variant carries its own price
-- delta rather than a full price, so a catalogue-wide price change does not
-- need every variant rewritten.
CREATE TABLE product_variants (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id        uuid        NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    name              text        NOT NULL,
    sku               text,
    price_delta_minor bigint      NOT NULL DEFAULT 0,
    status            text        NOT NULL DEFAULT 'ACTIVE',
    created_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT variants_status_valid CHECK (status IN ('ACTIVE', 'OUT_OF_STOCK', 'DISABLED'))
);

CREATE INDEX product_variants_product_idx ON product_variants (product_id);

-- --- inventory (document 69) -------------------------------------------------

CREATE TABLE inventory (
    store_id          uuid    NOT NULL REFERENCES stores (id) ON DELETE CASCADE,
    product_id        uuid    NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    variant_id        uuid    REFERENCES product_variants (id) ON DELETE CASCADE,
    -- MVP is availability; quantity is tracked where it is known.
    available         boolean NOT NULL DEFAULT true,
    quantity          integer,
    reserved_quantity integer NOT NULL DEFAULT 0,
    updated_at        timestamptz NOT NULL DEFAULT now(),
    id                bigserial PRIMARY KEY,
    -- The invariant that makes overselling impossible: reservations can never
    -- exceed stock. Document 69 requires atomic reservation, and a constraint
    -- holds even if the application forgets to check.
    CONSTRAINT inventory_not_oversold CHECK (
        quantity IS NULL OR (reserved_quantity >= 0 AND reserved_quantity <= quantity))
);

-- Most products have no variant, so variant_id is null for them — and a null
-- cannot sit in a primary key. NULLS NOT DISTINCT gives the uniqueness the
-- primary key was for while letting "no variant" be a real, single row.
CREATE UNIQUE INDEX inventory_unique_idx
    ON inventory (store_id, product_id, variant_id) NULLS NOT DISTINCT;

-- --- orders (documents 70, 72, 74) -------------------------------------------

CREATE TABLE orders (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id       uuid        NOT NULL REFERENCES merchants (id) ON DELETE RESTRICT,
    store_id          uuid        REFERENCES stores (id) ON DELETE SET NULL,
    customer_user_id  uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    status            text        NOT NULL DEFAULT 'CART',
    -- The delivery job this order produced, once it is ready for pickup.
    -- Separate lifecycles, one link (document 070).
    job_id            uuid        REFERENCES jobs (id) ON DELETE SET NULL,
    currency          text        NOT NULL DEFAULT 'PKR',
    items_total_minor bigint      NOT NULL DEFAULT 0,
    -- Document 072's merchant timestamps.
    accepted_at       timestamptz,
    preparation_started_at timestamptz,
    ready_at          timestamptz,
    expected_ready_at timestamptz,
    -- When the merchant must answer by. BD-12 sets the duration; this column
    -- is null until it is configured, which is what makes an unset timeout
    -- fail loudly rather than default silently.
    accept_deadline   timestamptz,
    rejection_reason  text,
    cancelled_reason  text,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT orders_status_valid CHECK (status IN (
        'CART', 'PLACED', 'PAYMENT_PENDING', 'CONFIRMED', 'PREPARING',
        'READY_FOR_PICKUP', 'PICKED_UP', 'DELIVERING', 'DELIVERED',
        'CANCELLED', 'FAILED')),
    CONSTRAINT orders_currency_valid CHECK (currency = 'PKR'),
    CONSTRAINT orders_total_nonneg CHECK (items_total_minor >= 0)
);

CREATE INDEX orders_merchant_status_idx ON orders (merchant_id, status, created_at DESC);
CREATE INDEX orders_customer_idx ON orders (customer_user_id, created_at DESC);
-- Drives the acceptance-timeout sweep.
CREATE INDEX orders_accept_deadline_idx ON orders (accept_deadline)
    WHERE status = 'PLACED' AND accept_deadline IS NOT NULL;

-- One live cart per customer per store, so "add to cart" is idempotent.
CREATE UNIQUE INDEX orders_one_cart_idx
    ON orders (customer_user_id, store_id) WHERE status = 'CART';

-- Order lines carry a price snapshot. Document 68: "Later catalog changes must
-- never alter historical orders." Referencing the product's current price
-- would rewrite every past receipt the next time a merchant changed a price.
CREATE TABLE order_items (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id          uuid        NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    product_id        uuid        REFERENCES products (id) ON DELETE SET NULL,
    variant_id        uuid        REFERENCES product_variants (id) ON DELETE SET NULL,
    name_snapshot     text        NOT NULL,
    unit_price_minor  bigint      NOT NULL,
    quantity          integer     NOT NULL,
    -- Document 74's per-item customer preference.
    substitution_preference text  NOT NULL DEFAULT 'ASK_ME',
    status            text        NOT NULL DEFAULT 'ORDERED',
    created_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT order_items_quantity_positive CHECK (quantity > 0),
    CONSTRAINT order_items_price_nonneg CHECK (unit_price_minor >= 0),
    CONSTRAINT order_items_pref_valid CHECK (
        substitution_preference IN ('ALLOW', 'DO_NOT_ALLOW', 'ASK_ME')),
    CONSTRAINT order_items_status_valid CHECK (
        status IN ('ORDERED', 'PICKED', 'SUBSTITUTED', 'REMOVED', 'UNAVAILABLE'))
);

CREATE INDEX order_items_order_idx ON order_items (order_id);

-- Item issues and substitutions. Document 74: "Never mutate the original
-- order-line history." A substitution is a new row referencing the original,
-- so what the customer ordered remains readable after what they received
-- changed.
CREATE TABLE order_item_issues (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id         uuid        NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    order_item_id    uuid        NOT NULL REFERENCES order_items (id) ON DELETE CASCADE,
    reason           text        NOT NULL,
    action           text        NOT NULL,
    -- The replacement, when one was offered.
    substitute_product_id uuid   REFERENCES products (id) ON DELETE SET NULL,
    substitute_name  text,
    substitute_unit_price_minor bigint,
    -- Document 74 requires configurable rules for price differences and
    -- customer approval. The difference is recorded; who absorbs it is BD-11
    -- and deliberately not decided here.
    price_difference_minor bigint,
    reported_by      uuid        REFERENCES users (id) ON DELETE SET NULL,
    resolution       text        NOT NULL DEFAULT 'PENDING',
    resolved_at      timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT item_issues_reason_valid CHECK (reason IN (
        'OUT_OF_STOCK', 'DAMAGED', 'QUALITY', 'PRICE_CHANGED', 'OTHER')),
    CONSTRAINT item_issues_action_valid CHECK (action IN (
        'SUBSTITUTE', 'REMOVE', 'REQUEST_CUSTOMER_DECISION')),
    CONSTRAINT item_issues_resolution_valid CHECK (resolution IN (
        'PENDING', 'CUSTOMER_ACCEPTED', 'CUSTOMER_DECLINED', 'AUTO_APPLIED', 'CANCELLED'))
);

CREATE INDEX order_item_issues_order_idx ON order_item_issues (order_id);

-- Order status history, mirroring job_status_history.
CREATE TABLE order_status_history (
    id          bigserial PRIMARY KEY,
    order_id    uuid        NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    from_status text,
    to_status   text        NOT NULL,
    actor_type  text        NOT NULL,
    actor_id    uuid,
    metadata    jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT order_history_actor_valid CHECK (actor_type IN (
        'CUSTOMER', 'MERCHANT', 'DRIVER', 'ADMIN', 'SUPPORT', 'SYSTEM'))
);

CREATE INDEX order_status_history_order_idx ON order_status_history (order_id, created_at);

-- Merchant configuration, including the acceptance timeout.
--
-- BD-12 is BLOCKING_LATER and unresolved. accept_timeout_seconds is NULL by
-- default, and the application refuses to place an order against a merchant
-- with no configured timeout — the register's instruction exactly: "Build as
-- configuration with an explicit unset state that fails loudly rather than
-- defaulting silently."
CREATE TABLE merchant_config (
    merchant_id            uuid PRIMARY KEY REFERENCES merchants (id) ON DELETE CASCADE,
    accept_timeout_seconds integer,
    default_prep_seconds   integer,
    auto_accept            boolean NOT NULL DEFAULT false,
    updated_at             timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT merchant_config_timeout_sane CHECK (
        accept_timeout_seconds IS NULL OR accept_timeout_seconds BETWEEN 30 AND 3600)
);
