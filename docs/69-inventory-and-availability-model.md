# 69 — Inventory & Availability Model

## MVP
Start with:
```text
AVAILABLE / OUT_OF_STOCK
```

Where quantity tracking is needed:
```text
store_id
product_id
quantity
reserved_quantity
updated_at
```

## Reservation
```text
available → reserved → fulfilled
```

Cancelled/failed orders release applicable reservations.

## Concurrency
Atomic reservation is required so two customers cannot safely reserve the same final unit simultaneously.

## Grocery Substitution
Per item support:
`ALLOW_SUBSTITUTION`, `NO_SUBSTITUTION`, `CONTACT_CUSTOMER`

## Definition of Done
Availability is reliable and inventory reservation cannot oversell under concurrent orders.
