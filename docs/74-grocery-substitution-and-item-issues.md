# 74 — Grocery Substitution & Item Issues

## Customer Preferences
Per item:
```text
ALLOW
DO_NOT_ALLOW
ASK_ME
```

If unavailable, merchant may:
```text
SUBSTITUTE
REMOVE
REQUEST_CUSTOMER_DECISION
```

Define configurable rules for price differences and when customer approval is required.

Record item issues with:
- item
- reason
- reporter
- timestamp
- resolution

Removed/unavailable items may create a partial refund. Never mutate the original order-line history.

## Definition of Done
Every item-level change is auditable and correctly reflected in final pricing/refunds.
