-- Reverses 000013. Orders stop recording what stock they hold, and the
-- inventory reservations made while it existed stay where they are: releasing
-- them here would hand back stock for orders that are still being picked.

ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_stock_state_valid;
ALTER TABLE orders DROP COLUMN IF EXISTS stock_state;
