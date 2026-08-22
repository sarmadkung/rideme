# 124 — Notification Preferences

## Objective
Let users control eligible communication channels while preserving required operational messages.

## Preference Levels
```text
USER
CHANNEL
CATEGORY
```

Example:
```text
Push → Delivery = ON
Email → Marketing = OFF
SMS → Promotional = OFF
```

## Required Notifications
Some messages cannot be disabled when required for:
- security
- payment
- active service
- safety
- legal/transactional requirements

## Quiet Hours
Where appropriate, support quiet hours for non-critical notifications.

## Device Preferences
Push permissions are ultimately controlled by the OS.

The server should know whether a token is active, but should not assume permission is guaranteed.

## Definition of Done
Notification preferences are respected without blocking essential operational communication.
