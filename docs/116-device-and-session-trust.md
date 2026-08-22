# 116 — Device & Session Trust

## Objective
Add device/session signals to account security and fraud detection.

## Device Signals
Depending on platform/privacy policy:
- device identifier
- app version
- OS
- IP risk signals
- session history
- authentication method

Avoid collecting unnecessary fingerprinting data.

## Sessions
Support:
- session expiry
- revocation
- suspicious session detection
- logout-all-devices

## Step-Up Authentication
For high-risk actions:
- identity changes
- payout changes
- sensitive account changes
- unusual login

Require additional verification where appropriate.

## Definition of Done
Account security can revoke suspicious sessions and apply step-up verification without breaking normal usage.
