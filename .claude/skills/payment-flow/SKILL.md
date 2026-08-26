---
name: payment-flow
description: Payment intents, methods, capture, refunds, webhooks, and COD — with idempotency and duplicate handling as the default assumption. Use for any payment work. Always Level 5; a payment bug moves real money.
---

# Purpose

Charge correctly, once, and know for certain whether it happened.

# When to Use

Payment intents, methods, capture, refunds, webhooks, COD collection, checkout.

# The Flow (`docs/19`)

```text
Job → Quote → Payment Intent → Completion → Final Amount → Ledger → Settlement
```

Methods: cash, Raast, local payment gateways, cards where supported.

# Rules

- **Never assume a provider request succeeded because the HTTP call succeeded.** Timeouts and network failures leave real charges in an unknown state. Reconcile against the provider; do not infer from the response alone. This single assumption causes most double-charge incidents.
- **Every payment mutation is idempotent** (`docs/377`, `docs/59`). A retried checkout must not create a second intent or a second charge.
- **Webhooks arrive more than once, out of order, and late.** Handlers must be idempotent, must verify signatures, and must tolerate a webhook for a state already reached (`docs/58`).
- **The final amount is computed server-side** from the authoritative quote and the actual job. Never from client input.
- **Never store raw card data** (`docs/19`). Provider tokens only.
- **Payments go through an adapter** (`system-architecture`). Provider SDK calls do not appear in domain code.
- Refunds have their own idempotency and their own ledger entries — a refund is never an edit to the original charge (`financial-ledger`).
- COD follows the documented path: order → driver collection → COD record → delivery confirmation → merchant balance → settlement (`docs/19`).

# Workflow

1. Create the payment intent from the authoritative quote, with an idempotency key.
2. Record the intent before calling the provider — an unrecorded in-flight charge is unrecoverable.
3. Call the provider through the adapter.
4. Treat the webhook as the confirmation, not the synchronous response.
5. Write ledger entries on confirmed state change (`financial-ledger`).
6. Reconcile anything that stays pending.

# Verification

**Always Level 5.**

Required: duplicate webhook, out-of-order webhook, webhook for an already-final state, provider timeout with unknown outcome, retried checkout with the same idempotency key, partial capture, refund, refund of a refund rejected, concurrent capture attempts, invalid signature rejected.

Never mark payment verified without duplicate-webhook and provider-timeout tests.

# Blocking Conditions

- Provider-specific behaviour is undocumented → do not guess it. Payment provider semantics vary and guessing loses money.
- Refund policy (window, partial rules, who absorbs fees) undocumented → `BLOCKED_TASKS.md`.
- No sandbox available for the provider → block rather than ship untested payment code.
- Currency or rounding rules unspecified → ask (`docs/451`).

# Relevant Documentation

`docs/19-payment-wallet-settlement.md` · `docs/51-payment-architecture.md` · `docs/52-payment-intents-and-customer-checkout.md` · `docs/57-refunds-chargebacks-and-disputes.md` · `docs/58-payment-webhooks-and-reconciliation.md` · `docs/59-financial-security-and-idempotency.md` · `docs/61-payment-testing-strategy.md` · `docs/333-payment-testing.md` · `docs/512`–`514` (payment models)
