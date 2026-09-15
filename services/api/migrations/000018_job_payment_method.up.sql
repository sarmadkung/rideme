-- How a job will be paid for (documents 052, 518).
--
-- The owner decided on 2026-09-15 that the platform keeps cash and adds
-- digital, rather than replacing one with the other. Recording the choice on
-- the job is the first half of that: settlement already knows how to take
-- cash, and it must know which jobs it is *not* to take cash for before any
-- digital path exists to take instead.
--
-- Defaulted to CASH so every job already in flight keeps the behaviour it was
-- created under. A nullable column would push the same decision into every
-- reader.
ALTER TABLE jobs ADD COLUMN payment_method text NOT NULL DEFAULT 'CASH';

ALTER TABLE jobs ADD CONSTRAINT jobs_payment_method_valid
    CHECK (payment_method IN ('CASH', 'CARD', 'WALLET', 'BANK'));

-- The same for a grocery order, which is paid for as a basket rather than as a
-- fare and can therefore differ from the delivery job it produces.
ALTER TABLE orders ADD COLUMN payment_method text NOT NULL DEFAULT 'CASH';

ALTER TABLE orders ADD CONSTRAINT orders_payment_method_valid
    CHECK (payment_method IN ('CASH', 'CARD', 'WALLET', 'BANK'));

-- Which methods this deployment can actually take.
--
-- A method is offerable only when something can process it. The table ships
-- with CASH enabled and nothing else, because that is the literal truth: no
-- payment provider is configured, and a customer offered a card button that
-- cannot charge a card has been lied to by the product.
--
-- Rows rather than a constant so switching a method on is an operator action
-- once credentials exist, not a deployment.
CREATE TABLE payment_methods (
    method      text PRIMARY KEY,
    enabled     boolean     NOT NULL DEFAULT false,
    -- The provider that handles it. Empty while nothing does.
    provider    text        NOT NULL DEFAULT '',
    -- What the customer sees on the button.
    label       text        NOT NULL,
    sort_order  integer     NOT NULL DEFAULT 0,
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT payment_methods_valid CHECK (method IN ('CASH', 'CARD', 'WALLET', 'BANK')),
    -- An enabled method with no provider is the mistake this constraint
    -- exists to make impossible: it would offer a customer a way to pay that
    -- nothing can complete.
    CONSTRAINT payment_methods_enabled_has_provider
        CHECK (NOT enabled OR method = 'CASH' OR provider <> '')
);

INSERT INTO payment_methods (method, enabled, provider, label, sort_order) VALUES
    ('CASH',   true,  'cash', 'Cash',          1),
    ('CARD',   false, '',     'Card',          2),
    ('WALLET', false, '',     'Mobile wallet', 3),
    ('BANK',   false, '',     'Bank transfer', 4);
