# Business Decision Register

The 19 undocumented business rules found during the skill-system audit. Every item is classified
by how it must be handled, not merely by which domain it belongs to.

**No rule in this register has been invented.** Where a defensible engineering default exists, it
is proposed as a *recommendation requiring confirmation*, never adopted silently. Where a value is
commercial, legal, or regulatory, it is marked as requiring a product decision and left open.

## Classification

| Class | Meaning | Count |
|---|---|---|
| `BLOCKING_NOW` | Blocks work in progress or Phase 1 | **0** |
| `BLOCKING_LATER` | Blocks a specific known phase; a default is proposable but needs confirmation | **2** |
| `PRODUCT_DECISION` | Commercial, legal, or regulatory. Cannot be safely inferred. Owner decides | **11** |
| `TECHNICAL_DEFAULT` | A defensible engineering default exists; adopt it explicitly, flag it, revisit with data | **6** |

**Phase 1 is not blocked by any item in this register.** The earliest blocking point is the first
vertical slice (ride), where six items become live.

---

## PRODUCT_DECISION (11)

### BD-01 — Cancellation fee amounts and thresholds
**Domain:** ride · pricing · payments
**Why it matters:** Charges real money to customers and pays compensation to drivers. Wrong values produce disputes and chargebacks.
**Docs:** `005` defines the *tiers* — before driver movement (none/low), after meaningful travel (compensation), after arrival (higher fee) — but no amounts and no definition of "meaningful travel".
**Depends on it:** ride cancellation, delivery cancellation, driver compensation, ledger entries.
**Proceed without?** No — cancellation cannot ship. The rest of the ride slice can.
**Recommendation:** None. Amounts are commercial. The *tier structure* from `005` can be implemented as configuration now, with values supplied later.

### BD-02 — Surge / demand multiplier behaviour
**Domain:** pricing
**Why it matters:** `005` includes `demand` in the ride fare formula but never defines how it is computed, its bounds, or its caps. Uncapped surge is a regulatory and reputational risk.
**Docs:** `005`, `224-surge-and-demand-pricing` (Tier C), `504-surge-and-demand-model` (Tier C).
**Depends on it:** ride quote, fare calculation, quote display.
**Proceed without?** Yes — implement the fare formula with `demand = 1.0` (no surge) and the term present but inert.
**Recommendation:** Ship without surge. Do not invent a multiplier.

### BD-05 — Commission rates, payout schedule, minimum payout threshold
**Domain:** financial · merchant · provider
**Why it matters:** Determines what every driver and merchant is paid. Wrong rates mean paying the wrong people the wrong amounts, retroactively.
**Docs:** `019` shows a worked example (`+500` earning, `-50` fee) but states no rate. `054`, `510` name commission without values.
**Depends on it:** ledger entries, earnings, payouts, settlement runs, merchant dashboards.
**Proceed without?** Partly — the ledger *structure* can be built and tested with configured rates. No real payout may run.
**Recommendation:** Build commission as configuration from day one. Values are the owner's.

### BD-06 — Refund policy: window, partial rules, fee absorption
**Domain:** payments
**Why it matters:** Determines who absorbs the payment-provider fee on a refund and whether partial refunds are allowed.
**Docs:** `057`, `463-refund-policy-engine` (Tier C). No policy stated.
**Depends on it:** refund endpoint, ledger reversal entries, dispute handling.
**Proceed without?** Yes for the mechanism; no for automated policy. Refunds can be admin-initiated with explicit amounts before policy exists.
**Recommendation:** Build manual admin refunds first. Automate once policy is set.

### BD-09 — COD liability between driver, merchant, and platform
**Domain:** financial · merchant · delivery
**Why it matters:** Cash in a driver's hand is a real liability. Who bears the loss if it is not remitted has legal and contractual consequences.
**Docs:** `019` defines the COD *flow*; `056`, `518` name COD without allocating liability.
**Depends on it:** COD records, driver balance, merchant settlement, reconciliation.
**Proceed without?** No for COD settlement. Yes for card/digital payment paths.
**Recommendation:** None — this is a legal allocation.

### BD-10 — Failed delivery: financial consequence and return-leg pricing
**Domain:** delivery
**Why it matters:** A parcel that cannot be delivered still consumed driver time and fuel. Who pays, and what a return leg costs, is unstated.
**Docs:** `084` defines failure *states*; no financial treatment.
**Depends on it:** delivery exception handling, return flow, ledger entries.
**Proceed without?** Yes — implement failure states and the return *flow*; leave the financial leg unwired.
**Recommendation:** Build states now, money later.

### BD-11 — Grocery substitution: who absorbs the price difference
**Domain:** grocery · payments
**Why it matters:** Substitution changes the order total after payment authorization. Charging the difference to the customer, absorbing it, or capping it are materially different products.
**Docs:** `074` defines the substitution *flow*; no pricing rule.
**Depends on it:** substitution flow, payment adjustment, ledger.
**Proceed without?** No for the financial adjustment. The offer/accept/decline flow can be built.
**Recommendation:** None.

### BD-13 — Cargo waiting and loading rates; restricted-goods list; damage liability
**Domain:** cargo
**Why it matters:** `005` makes loading and waiting billable components of the cargo fare, with no rates. Restricted goods is a legal list — carrying prohibited cargo has consequences beyond the platform. Damage liability is contractual.
**Docs:** `005`, `087`, `088`. Flow documented, values and lists absent.
**Depends on it:** cargo quoting, eligibility filtering, cargo dispatch.
**Proceed without?** Partly — record loading/waiting as timestamped events now (needed regardless); do not price them. Do not ship cargo without a restricted-goods list.
**Recommendation:** Build the event recording. The list and the rates are the owner's, and the list may require legal input.

### BD-14 — Required documents per vehicle type; suspension criteria and appeals
**Domain:** provider
**Why it matters:** Regulatory. Which licence, permit, or fitness certificate each vehicle type requires is set by law, not by the platform. Suspension without a defined appeal path is a fairness and legal exposure.
**Docs:** `016` states drivers cannot accept jobs with expired required documents — but never enumerates them. `108`, `211`, `384` describe the pipeline, not the list.
**Depends on it:** provider onboarding, verification pipeline, acceptance gating.
**Proceed without?** Yes structurally — the document model and expiry mechanism are type-agnostic. No for go-live.
**Recommendation:** Build a configurable document-requirement model. The list requires local regulatory input.

### BD-15 — Location retention periods (server-side and on-device)
**Domain:** location · privacy
**Why it matters:** High-frequency location history is sensitive personal data. Indefinite retention by default is a privacy decision made by omission — the worst way to make one.
**Docs:** `102`, `118`, `293`, `497` all name retention; none states a period.
**Depends on it:** `driver_locations` schema, retention jobs, privacy compliance, data export/deletion.
**Proceed without?** Yes short-term — but a period must be set before production location data accumulates.
**Recommendation:** A conservative default is technically obvious (retain full-resolution history briefly, downsample beyond that). It is still a privacy and legal decision, so it is not adopted here. Flag as required before launch.

### BD-16 — Proof-of-delivery photo and signature retention
**Domain:** delivery · privacy
**Why it matters:** Proof images capture addresses, doorways, and sometimes people. Same category of exposure as location history.
**Docs:** `083` defines proof mechanisms; `118`, `316` name privacy without periods.
**Depends on it:** proof storage, object-storage lifecycle rules, deletion requests.
**Proceed without?** Yes short-term.
**Recommendation:** Set alongside BD-15 as one retention policy decision.

---

## TECHNICAL_DEFAULT (6)

Defaults below are **recommendations to be confirmed**, adopted explicitly and recorded, not
assumed silently.

### BD-03 — Dispatch scoring weights per service type
**Domain:** dispatch
**Why it matters:** The weights decide which driver gets which job — earnings distribution and customer wait time.
**Docs:** `005` gives the full formula and states: *"Weights should be configurable and learned from real outcomes later."* The document explicitly anticipates starting without tuned values.
**Depends on it:** candidate scoring, dispatch tuning.
**Proceed without?** **Yes** — `005` authorizes it.
**Recommended default:** Implement weights as runtime configuration. Start with ETA dominant and the remaining terms low but non-zero, so the mechanism is exercised. Tune from real outcomes as `005` directs. Confirm the starting values before go-live.

### BD-07 — Rounding and currency precision
**Domain:** financial
**Why it matters:** Ad-hoc rounding creates reconciliation drift that is painful to unwind after money has moved.
**Docs:** `451-money-and-currency-standard` (Tier C) names the standard; states nothing. `019` examples are whole rupees.
**Depends on it:** every fare, fee, commission, and ledger entry.
**Proceed without?** Only with an explicit decision — the default must be made before the first money-shaped code.
**Recommended default:** Store money as **integer minor units** in a single currency (PKR), never floating point. Round once, at the final customer-facing amount, half-up. Ledger entries carry exact integers that sum without loss. This is standard practice and reversible; confirm before the ledger is built.

### BD-08 — Reconciliation discrepancy tolerance and escalation
**Domain:** financial
**Why it matters:** A tolerance band silently hides the bug that caused the gap.
**Docs:** `019` requires reconciliation against provider, bank, and cash records; sets no tolerance.
**Depends on it:** reconciliation engine, alerting.
**Proceed without?** Yes.
**Recommended default:** **Zero tolerance.** Any discrepancy raises an alert and is investigated; nothing auto-adjusts. Escalation routing is operational and can be set later.

### BD-17 — Location tracking frequency per job state
**Domain:** mobile · location
**Why it matters:** The battery-versus-accuracy tradeoff. A driver whose phone dies mid-shift stops earning — that is an outage.
**Docs:** `017`, `018` define the pipeline and native filtering; `182` names battery. No frequencies.
**Depends on it:** native location module, dispatch freshness, battery life.
**Proceed without?** Yes — this is measurable engineering, not commercial.
**Recommended default:** Vary by driver state — lowest when offline, low when available and stationary, highest on an active trip. Set actual values from real-device battery measurement (`mobile-performance`), not from guesswork, and record the result.

### BD-18 — Offline queue expiry; which mutations may be queued
**Domain:** mobile
**Why it matters:** Replaying an hour-old job acceptance dispatches a driver to a job that no longer exists.
**Docs:** `184`, `344`, `345` describe offline architecture; no queuing policy.
**Depends on it:** mutation queue, reconnect flush.
**Proceed without?** Yes.
**Recommended default:** Allow-list, not deny-list — a mutation is queueable only if explicitly marked so. Financial mutations and job acceptance are never optimistically confirmed. Queued items carry an expiry appropriate to the action; expired items surface to the user rather than replaying.

### BD-19 — Mobile performance budgets
**Domain:** mobile
**Why it matters:** Without a budget there is no definition of done for performance work.
**Docs:** `319-performance-budget` (Tier C) names budgets; states none. `182`, `336` name the surfaces.
**Depends on it:** performance verification, release gates.
**Proceed without?** Yes — budgets are needed when there is an app to measure.
**Recommended default:** Set budgets empirically from the first working build on a representative low-end Android device, then hold the line. Do not adopt numbers from elsewhere.

---

## BLOCKING_LATER (2)

### BD-04 — Behaviour when no dispatch candidate exists
**Domain:** dispatch · ride · delivery
**Why it matters:** A job stuck in `SEARCHING` with no terminal path is an operational dead end — the customer waits indefinitely and no one is notified.
**Docs:** `015` includes `EXPIRED` as a terminal state; `044`, `228` name timeout and retry without policy. How long to search, how often to re-attempt, and what the customer sees are unstated.
**Depends on it:** dispatch retry loop, job expiry, customer notification.
**Proceed without?** No — the dispatch slice cannot be considered complete without a defined terminal path. `EXPIRED` exists in the state machine, so the *shape* is documented; the durations are not.
**Blocks at:** first vertical slice (ride dispatch).
**Recommendation:** A search window and retry cadence can be proposed as configuration once dispatch is built — but the customer-facing behaviour on failure is a product decision. Raise before the dispatch slice starts.

### BD-12 — Merchant order acceptance timeout
**Domain:** merchant · grocery
**Why it matters:** An order a merchant never accepts leaves the customer waiting and the driver unassigned. Auto-cancelling too aggressively damages merchant relations; too slowly damages customer trust.
**Docs:** `072`, `552` describe merchant order management; no timeout.
**Depends on it:** grocery order state machine, auto-cancellation, customer notification.
**Proceed without?** Partly — the acceptance flow can be built with the timeout as configuration. The value and the resulting behaviour must be set before grocery ships.
**Blocks at:** grocery slice.
**Recommendation:** Build as configuration with an explicit unset state that fails loudly rather than defaulting silently. Value from the owner.

---

## Timeline

| Phase | Items that become blocking |
|---|---|
| Phase 1 — foundation | **none** |
| Phase 2 — infrastructure | none |
| Phase 3 — backend foundation | BD-07 (before any money-shaped code) |
| Phase 4 — auth | none |
| Phase 5 — canonical domain | BD-14 structurally (configurable model only) |
| First vertical slice — ride | BD-01, BD-02, BD-03, BD-04, BD-05, BD-06 |
| Delivery slice | BD-10, BD-16 |
| Grocery slice | BD-11, BD-12 |
| Cargo slice | BD-13 |
| Financial completeness | BD-08, BD-09 |
| Before production launch | BD-15, BD-16, BD-14 (regulatory list), BD-19 |
