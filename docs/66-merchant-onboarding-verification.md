# 66 — Merchant Onboarding & Verification

## Flow
```text
Register → Business Profile → Owner Verification → Store → Documents → Payout Info → Review → APPROVED
```

## States
`DRAFT`, `PENDING_REVIEW`, `ACTION_REQUIRED`, `APPROVED`, `SUSPENDED`, `REJECTED`

## Business Data
- legal/display name
- contact details
- category
- tax information where required
- operating locations

## Store Data
- address and coordinates
- opening hours
- preparation time
- service radius
- pickup instructions

Only approved merchants can publish products or receive production orders.

## Definition of Done
Admin can review, approve, reject or request corrections with a complete audit trail.
