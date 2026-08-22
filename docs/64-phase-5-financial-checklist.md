# 64 — Phase 5 Financial Production Checklist

## Before Production

### Payments
- [ ] provider sandbox tests pass
- [ ] production credentials stored securely
- [ ] webhook signatures verified
- [ ] webhook retries tested
- [ ] duplicate events tested

### Ledger
- [ ] every transaction balances
- [ ] entries are immutable
- [ ] reversal strategy exists
- [ ] reconciliation is operational

### Driver Earnings
- [ ] completion creates one earning record
- [ ] commission configuration reviewed
- [ ] payout eligibility enforced

### COD
- [ ] cash collection workflow tested
- [ ] mismatch workflow exists
- [ ] merchant settlement tested

### Refunds
- [ ] full refund
- [ ] partial refund
- [ ] failed refund
- [ ] duplicate refund attempt
- [ ] driver-earning impact

### Security
- [ ] no raw card data stored
- [ ] admin financial permissions reviewed
- [ ] sensitive actions audited
- [ ] rate limits active

### Operations
- [ ] payment failure dashboard
- [ ] payout failure dashboard
- [ ] reconciliation dashboard
- [ ] dispute workflow
- [ ] incident runbook

## Release Gate
Do not launch live payments until all financial invariants and reconciliation checks pass in staging.
