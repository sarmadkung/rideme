# 54 — Driver Earnings & Commission

## Objective
Calculate what the driver earns and what the platform retains.

## Earnings Breakdown
```text
Customer Fare
- Taxes where applicable
- Platform Commission
- Payment Fees where applicable
- Adjustments
= Driver Earnings
```

Actual commercial policy is configurable.

## Earnings Record
```text
job_id
driver_id
gross_amount
commission
adjustments
net_earnings
currency
pricing_version
earnings_version
status
```

## Earnings States
```text
PENDING
AVAILABLE
PAID
REVERSED
DISPUTED
```

## Completion
Driver earnings should be generated exactly once after valid job completion.

Use a unique job reference.

## Adjustments
Support:
- waiting
- toll
- cancellation compensation
- bonuses
- penalties
- manual support adjustments

Every adjustment needs:
- reason
- actor
- timestamp
- audit reference

## Driver App
Show:
- today's earnings
- completed jobs
- pending earnings
- available balance
- payout history

## Definition of Done
Driver earnings are deterministic, auditable and cannot be duplicated by repeated completion events.
