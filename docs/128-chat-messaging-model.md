# 128 — Chat Messaging Model

## Message
```text
id
conversation_id
sender_id
type
body
metadata
created_at
edited_at
deleted_at
```

## Message Types
- text
- image
- file
- system
- location reference
- order/job reference

## Delivery States
```text
SENT
DELIVERED
READ
```

## Idempotency
Client message submission should support an idempotency/client-message ID.

## Editing/Deletion
Apply policy-specific rules and retain audit metadata where required.

## Attachments
Store secure media references rather than embedding large binaries in message records.

## Definition of Done
Messaging supports reliable delivery, idempotency, read state and controlled attachments.
