-- Stock a placed order is holding.
--
-- Document 069 requires atomic reservation and the inventory table has carried
-- the constraint that makes overselling impossible since 000009 —
-- `reserved_quantity <= quantity`. Nothing ever wrote to it. `Reserve` and
-- `ReleaseReservation` were written, tested, and called by nothing, so two
-- customers could both place an order for the last bag of rice and one of them
-- was going to be disappointed by a shop rather than by a screen.
--
-- Reserving is only half a rule. Stock held by an order that is later cancelled
-- must come back, and stock that leaves the shelf must stop being merely
-- reserved and actually be gone. This column is which of those has happened, so
-- neither can happen twice: releasing a released order would invent stock, and
-- consuming a consumed one would lose it.
ALTER TABLE orders
    ADD COLUMN stock_state text NOT NULL DEFAULT 'NONE';

ALTER TABLE orders
    ADD CONSTRAINT orders_stock_state_valid CHECK (
        stock_state IN ('NONE', 'RESERVED', 'RELEASED', 'CONSUMED'));

-- Existing orders predate reservation and hold nothing. NONE is correct for
-- them and is what the default gives; this is stated rather than run, because a
-- migration that quietly rewrote live orders' stock state would be worse than
-- one that does not.
