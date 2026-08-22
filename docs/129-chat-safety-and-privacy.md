# 129 — Chat Safety & Privacy

## Objective
Prevent chat from becoming a channel for abuse or privacy leakage.

## Controls
- block prohibited content where required
- report message/user
- rate limiting
- attachment validation
- abuse detection
- conversation expiration

## Privacy
Do not reveal private phone numbers by default.

Use masked communication or in-app messaging where appropriate.

## Conversation Access
Access should be derived from active relationships:
```text
customer ↔ assigned driver
merchant ↔ relevant order
support ↔ case participants
```

Access may end when a service ends, subject to retention/support rules.

## Evidence
Reported messages should remain available to authorized trust/support staff under controlled access.

## Definition of Done
Chat is operationally useful while minimizing privacy and abuse risk.
