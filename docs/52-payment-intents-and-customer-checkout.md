# 52 — Payment Intents & Customer Checkout

## Objective
Allow customers to pay securely for rides, deliveries and cargo.

## Payment Methods
Initial architecture should support:
- cash
- card
- bank/wallet provider
- stored payment method where legally and technically appropriate

## Checkout Flow
```text
Quote
 ↓
Confirm Job
 ↓
Create Payment Intent
 ↓
Payment Action
 ↓
Payment Provider
 ↓
Webhook / Confirmation
 ↓
Payment State
```

## Payment Intent
```text
id
job_id
customer_id
amount_minor
currency
status
provider
provider_reference
idempotency_key
created_at
expires_at
```

## Authorization vs Capture
Where provider supports it:
- authorize at confirmation
- capture after completion

For immediate-payment methods, capture may occur during checkout.

## Webhooks
Provider webhooks are authoritative for provider-side asynchronous state.

Validate:
- signature
- event ID
- provider reference
- expected amount
- expected customer/job

## Duplicate Webhooks
Webhook processing must be idempotent.

Store provider event IDs.

## Failed Payment
Do not silently create an unpaid completed job.

Business rules determine whether:
- retry payment
- allow cash fallback
- hold job
- escalate

## Definition of Done
Customer can select a payment method, complete payment, and retrieve authoritative payment status.
