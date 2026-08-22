# 76 — Merchant API Security & Webhooks

## Authentication
Support merchant users and approved integrations with explicit role-based permissions.

Example roles:
`OWNER`, `MANAGER`, `STAFF`, `ACCOUNTANT`

## Webhook Events
```text
order.created
order.confirmed
order.cancelled
order.ready
delivery.assigned
delivery.picked_up
delivery.completed
refund.created
```

## Security
Use:
- signing secret
- timestamp
- event ID
- replay protection
- rate limits

External order creation requires idempotency keys.

## Definition of Done
Merchant integrations cannot create duplicate orders and webhook events can be authenticated and retried safely.
