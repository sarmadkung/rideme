DROP TABLE IF EXISTS payment_methods;
ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_payment_method_valid;
ALTER TABLE orders DROP COLUMN IF EXISTS payment_method;
ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_payment_method_valid;
ALTER TABLE jobs DROP COLUMN IF EXISTS payment_method;
