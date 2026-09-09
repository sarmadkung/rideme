-- Reverses 000012. Orders lose their destination and return to the state where
-- a grocery order cannot be delivered.

DROP INDEX IF EXISTS orders_delivery_location_idx;

ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_delivery_complete;

ALTER TABLE orders
    DROP COLUMN IF EXISTS delivery_notes,
    DROP COLUMN IF EXISTS delivery_location,
    DROP COLUMN IF EXISTS delivery_address;
