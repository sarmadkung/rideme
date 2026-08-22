# 132 — Support Refunds & Operational Actions

## Objective
Allow authorized support agents to perform controlled actions.

## Possible Actions
- refund
- partial refund
- resend notification
- cancel eligible job/order
- reschedule
- reassign
- add credit/adjustment where supported

## Principle
Support should invoke domain APIs rather than directly editing database records.

## Permissions
High-impact actions require:
- role permission
- reason
- optional approval
- audit event

## Financial Actions
Refunds must use the payment/ledger system and preserve financial history.

## Definition of Done
Support can resolve common issues safely without bypassing core business rules.
