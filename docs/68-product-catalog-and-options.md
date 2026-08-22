# 68 — Product Catalog & Options

## Structure
```text
Merchant → Catalog → Category → Product
```

Product data:
- name
- description
- SKU
- price
- currency
- status
- image reference
- category

## States
`DRAFT`, `ACTIVE`, `OUT_OF_STOCK`, `DISABLED`, `ARCHIVED`

Support variants, sizes, weights and add-ons.

Example:
```text
Milk → 1L / 2L / 5L
```

## Price Snapshot
Orders must store the price used at checkout. Later catalog changes must never alter historical orders.

## Definition of Done
Merchants can manage products without changing historical order data.
