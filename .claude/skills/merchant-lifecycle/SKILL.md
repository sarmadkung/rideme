---
name: merchant-lifecycle
description: Merchant onboarding, store and hours management, catalog and inventory ownership, order handling, and payouts. Use for merchant-side work and whenever deciding what a merchant owns versus what the platform owns.
---

# Purpose

Give merchants control of their own data while the platform keeps operational and financial authority.

# When to Use

Merchant registration and verification, store setup, operating hours, catalog and inventory ownership, merchant order handling, payouts.

# The Ownership Line

| Merchant owns | Platform owns |
|---|---|
| Catalog, products, variants, prices | The delivery Job and its dispatch |
| Inventory and availability | Job status and assignment |
| Store profile and operating hours | The financial ledger |
| Order acceptance and picking | Commission, payouts, settlement |

This boundary is stated in `docs/262` and is the thing most easily blurred. A merchant setting their own payout amount, or the platform silently editing a catalog, are both violations.

# Rules

- **Merchant-owned data is merchant-editable; platform financial records are not.** Payout amounts derive from the ledger, never from merchant input.
- **Operating hours gate ordering** (`docs/67`). A closed store cannot receive orders — enforced server-side.
- **Merchant accounts are authenticated and authorized separately** from customers and providers. A merchant sees only their own orders. Cross-merchant data exposure is a serious defect.
- Merchant order acceptance has a timeout path — an order left unaccepted needs a defined outcome.
- Merchant payouts and COD reconciliation run through the ledger (`settlement-reconciliation`, `docs/56`, `docs/75`).
- Merchant webhooks and API access are security surfaces (`docs/76`) — signed, rate-limited, and scoped.

# Workflow

1. Registration and verification create the merchant in an unverified state.
2. Store setup captures profile, location, operating hours.
3. Catalog and inventory are built under merchant ownership.
4. Order handling: acceptance → picking → packing → ready for pickup.
5. Settlement runs on the documented cycle against ledger entries.

# Verification

Level 4 for merchant operations; Level 5 for payouts, COD, and any cross-merchant authorization boundary.

Required: merchant cannot read another merchant's orders, closed-store order rejected, unaccepted-order timeout, payout matches ledger sum exactly, duplicate webhook delivery.

# Blocking Conditions

- Commission rates and payout schedule undocumented → `BLOCKED_TASKS.md`; commercial decision.
- Merchant order acceptance timeout undefined → ask.
- COD liability between merchant, driver, and platform unspecified → financial decision; do not invent.

# Relevant Documentation

`docs/65-merchant-platform-architecture.md` · `docs/66-merchant-onboarding-verification.md` · `docs/67-merchant-store-and-operating-hours.md` · `docs/75-merchant-settlement-reporting.md` · `docs/76-merchant-api-security-and-webhooks.md` · `docs/259-merchant-domain.md` · `docs/267-merchant-payouts.md` · `docs/549`–`552` (merchant flows)
