# 72 — Merchant Order Management

## Dashboard Queues
```text
New
Preparing
Ready
Completed
Cancelled
```

## Actions
`Accept`, `Reject`, `Start Preparing`, `Mark Ready`, `Report Issue`

Order detail includes items, quantities, preparation deadline, customer instructions, pickup details and substitutions.

Track:
```text
accepted_at
preparation_started_at
ready_at
expected_ready_at
```

Merchants can flag delays and emit an appropriate order event.

## Definition of Done
Merchant can process an order from acceptance to ready-for-pickup without admin intervention.
