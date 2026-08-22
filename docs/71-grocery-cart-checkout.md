# 71 — Grocery Cart & Checkout

## Flow
```text
Browse → Product → Cart → Address → Delivery Option → Quote → Payment → Place Order
```

Cart contains store, items, quantities, variants and substitution preferences.

## Checkout Validation
Verify:
- store is open
- products still exist
- prices are current
- quantities are available
- delivery area is supported
- minimum order rules

## Price Snapshot
Store:
```text
product price
quantity
discount
delivery fee
service fee
tax
total
```

If prices or availability changed materially, show the customer the updated checkout before final confirmation.

## Definition of Done
Stale catalog or inventory state cannot silently produce an invalid order.
