# 121 — Notification Architecture

## Objective
Create one communication layer for customers, drivers, merchants and operations.

## Channels
```text
In-App
Push
SMS
Email
Realtime
```

## Architecture
```text
Domain Event
   ↓
Notification Service
   ├── Preferences
   ├── Templates
   ├── Routing
   └── Delivery Providers
          ↓
      Channel Adapters
```

## Principle
Business services emit events. They should not directly call Twilio, Firebase, email providers or other channel vendors.

## Notification Types
- transactional
- operational
- safety
- support
- marketing

Safety and transactional messages have priority over marketing.

## Definition of Done
All major services can trigger notifications through a consistent event-driven interface.
