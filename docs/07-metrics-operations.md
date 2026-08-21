# Metrics, Operations & Risk

## North-Star Metric

Completed profitable jobs per active vehicle per day.

## Driver Metrics

- Net earnings/hour
- Gross earnings/hour
- Jobs/hour
- Acceptance rate
- Completion rate
- Cancellation rate
- Online hours
- Empty kilometers
- Revenue/km
- Driver retention

## Customer Metrics

- Search/request conversion
- Fulfillment rate
- Pickup ETA
- Cancellation rate
- Repeat rate
- Customer rating
- Support contact rate

## Merchant Metrics

- Orders/month
- Delivery success
- COD value
- Average delivery cost
- Repeat usage
- API usage
- Merchant retention

## Network Metrics

- Active vehicles
- Jobs/day
- Jobs/vehicle/day
- Empty-km ratio
- Average pickup distance
- Supply/demand ratio by zone
- Peak-hour availability

## Financial Metrics

- Gross booking value
- Revenue
- Driver payout
- Incentives
- Payment costs
- Support cost
- Contribution margin/job
- CAC
- LTV
- Payback period

## Safety

Track:
- Incidents
- SOS events
- Route deviations
- Vehicle mismatch
- Driver verification failures
- Customer safety reports

## Fraud

Monitor:
- GPS spoofing
- Multiple accounts
- Collusive trips
- Fake completion
- Repeated COD refusal
- Abnormal cancellation patterns
- Payment abuse

## Operational Support

Every issue should create a case:

```text
Case ID
Job ID
User
Category
Severity
Evidence
Assigned agent
Status
SLA
Resolution
```

Categories:
- Driver did not arrive
- Customer unavailable
- Wrong vehicle
- Extra fare request
- Payment
- Safety
- Lost item
- Damaged item
- Missing item
- COD
- Cancellation

## Cargo Chain of Custody

At pickup:
- Photo
- Item count
- Condition
- Loading confirmation

At delivery:
- Photo
- Receiver confirmation
- Signature/OTP where appropriate

## Risk Priorities

1. Driver acquisition
2. Unit economics
3. Driver churn
4. Customer acquisition
5. Supply/demand liquidity
6. Safety
7. Fraud
8. Regulatory
9. Grocery operations
10. Technology
