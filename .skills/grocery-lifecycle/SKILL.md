---
name: grocery-lifecycle
description: The GROCERY journey — catalog, cart, checkout, merchant acceptance, picking, packing, provider pickup, delivery, and item substitution. Use for grocery ordering work and anywhere merchant-owned data meets platform-owned operational state.
---

# Purpose

Run merchant-fulfilled grocery orders without confusing merchant data, platform operational state, and financial records.

# When to Use

Catalog, cart, checkout, merchant order handling, picking/packing, substitution, grocery delivery jobs.

# The Journey

```text
merchant catalog → inventory → customer cart → checkout → order
→ merchant acceptance → picking → packing → provider assignment
→ pickup → delivery
```

Two linked concepts: the **customer order** (merchant fulfilment) and the **delivery Job** with `type = GROCERY` (`docs/04`, `docs/266`). The order drives fulfilment; the Job drives dispatch and delivery. Keep them linked, not merged.

# Rules

- **Separate merchant-owned data from platform-owned operational state and financial records** (`docs/262`). Merchants own catalog, inventory, and store hours. The platform owns the Job, the dispatch, and the ledger.
- **Inventory is checked at checkout, server-side.** A cart built against stale availability must fail at checkout rather than produce an unfulfillable order.
- **Substitution is an explicit, documented flow** (`docs/74`) — item unavailable, replacement offered, customer accepts or declines. It changes the order total, so it changes the payment amount. Never adjust a total without an auditable record.
- **The delivery Job is created when fulfilment reaches the right stage**, not at checkout — otherwise drivers are dispatched to orders that are not picked.
- Merchant payouts and platform commission settle through the ledger (`settlement-reconciliation`), never by direct balance mutation.
- Cash on delivery follows the documented COD path (`docs/19`).

# Workflow

1. Reuse Job, dispatch, payment. Grocery adds merchant and catalog concepts around them.
2. Build catalog and inventory with clear merchant ownership.
3. Implement cart with server-side revalidation at checkout.
4. Implement merchant acceptance → picking → packing states.
5. Create the delivery Job at the documented stage; dispatch normally.
6. Implement substitution with its financial adjustment.

# Verification

Level 4 for order flow; Level 5 for checkout totals, substitution adjustments, COD, and merchant settlement.

Required: out-of-stock at checkout, substitution accepted and declined, merchant rejects order, order cancelled after picking, COD collection recorded, duplicate checkout request.

# Blocking Conditions

- Substitution pricing policy (who absorbs a price difference) undocumented → `BLOCKED_TASKS.md`.
- Merchant commission rates unspecified → commercial decision; do not invent.
- Behaviour when a merchant never accepts an order → ask for the timeout policy.

# Relevant Documentation

`docs/65-merchant-platform-architecture.md` · `docs/68-product-catalog-and-options.md` · `docs/69-inventory-and-availability-model.md` · `docs/70-grocery-order-lifecycle.md` · `docs/71-grocery-cart-checkout.md` · `docs/72-merchant-order-management.md` · `docs/73-merchant-driver-pickup-flow.md` · `docs/74-grocery-substitution-and-item-issues.md` · `docs/537`–`540` (grocery flows)
