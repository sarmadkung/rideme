# 67 — Merchant Stores & Operating Hours

## Model
```text
Merchant
 ├── Store A
 ├── Store B
 └── Store C
```

Store fields include name, address, coordinates, phone, timezone and status.

## Store States
`ACTIVE`, `INACTIVE`, `TEMPORARILY_CLOSED`, `SUSPENDED`

## Hours
Support weekly schedules, holidays and temporary/special hours.

A store is orderable only when:
```text
active
AND within operating hours
AND accepting orders
AND delivery area supported
```

Track default and peak preparation estimates.

## Definition of Done
Store availability is deterministic and timezone-aware.
