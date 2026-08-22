# 65 — Merchant Platform Architecture

## Objective
Add merchants as first-class participants for grocery and other pickup-based services.

```text
Customer → Merchant Order → Fulfillment Job → Dispatch → Driver
```

A **Merchant Order is not the same thing as a Delivery Job**. An order may exist before a driver is assigned and can later produce one or more fulfillment jobs.

## Merchant Capabilities
- business profile
- branches/stores
- catalog
- operating hours
- order acceptance
- preparation
- fulfillment
- settlement
- analytics

## Definition of Done
Merchant functionality integrates with dispatch through stable order/job contracts without coupling merchant internals to dispatch.
