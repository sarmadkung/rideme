# 75 — Merchant Settlement & Reporting

## Settlement View
```text
Gross Sales
- Discounts
- Refunds
- Platform Fees
- Other Adjustments
= Net Settlement
```

## States
`PENDING`, `CALCULATED`, `PROCESSING`, `PAID`, `FAILED`

## Reports
- daily sales
- orders
- cancellations
- refunds
- commissions
- settlements
- outstanding balance

Reports must use financial records rather than recomputing old orders from current pricing configuration.

## Definition of Done
Merchant can reconcile completed orders against settlement amounts.
