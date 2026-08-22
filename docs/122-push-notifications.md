# 122 — Push Notifications

## Objective
Deliver reliable mobile notifications through React Native applications.

## Device Token
Maintain:
```text
user_id
device_id
platform
push_token
app_version
last_seen_at
status
```

## Multiple Devices
A user may have multiple active devices.

Do not assume one user equals one push token.

## Notification Categories
```text
RIDE
DELIVERY
ORDER
PAYMENT
SAFETY
SUPPORT
MARKETING
```

## Deep Links
Notifications should open the relevant application screen using stable deep-link routes.

## Reliability
Track:
```text
queued
sent
provider_accepted
delivered where available
failed
```

## Definition of Done
Push delivery supports multiple devices, preferences, deep links and delivery observability.
