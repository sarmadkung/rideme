-- Where a grocery order is going.
--
-- Document 071's checkout flow is Browse → Product → Cart → **Address** →
-- Delivery Option → Quote → Payment → Place Order, and the Address step had
-- nowhere to land: the only address columns in 000009 belong to merchants and
-- stores. An order therefore had a shop it came from and no destination, which
-- is why READY_FOR_PICKUP could never produce the delivery Job document 070
-- says it produces — a job needs a dropoff stop, and there was nothing to put
-- in one.
--
-- The address lives on the order rather than being resolved later from the
-- customer's profile. A customer sends groceries to their mother's house; the
-- destination belongs to the order that was placed, not to the account that
-- placed it, and a later profile edit must not move a delivery that already
-- happened.
ALTER TABLE orders
    ADD COLUMN delivery_address  text,
    -- Geography, like every other point on this platform (000005, ADR: PostGIS
    -- is already the coordinate representation for jobs and drivers).
    ADD COLUMN delivery_location geography(Point, 4326),
    -- "Second gate, ring the bell" — the difference between a delivery and a
    -- failed delivery, and free text because no taxonomy of gates exists.
    ADD COLUMN delivery_notes    text;

-- Either both or neither. A destination with a name and no coordinates cannot
-- be routed to and a pair of coordinates with no name cannot be read out to a
-- driver, so a half-set destination is not a state this table will hold.
ALTER TABLE orders
    ADD CONSTRAINT orders_delivery_complete CHECK (
        (delivery_address IS NULL AND delivery_location IS NULL)
        OR (delivery_address IS NOT NULL AND delivery_location IS NOT NULL));

-- Dispatch will ask "which orders are ready near here". Cheap now, and a GIST
-- index added later on a large table is a lock nobody wants at the time.
CREATE INDEX orders_delivery_location_idx ON orders USING GIST (delivery_location);
