# 169 — Events, Queues & Background Workers

## Objective
Move asynchronous and retryable work out of synchronous request paths.

## Examples
```text
Order Created
 → Notification
 → Analytics
 → Merchant Update
```

```text
Delivery Completed
 → POD Processing
 → Settlement
 → Customer Notification
```

## Queue Requirements
- retries
- dead-letter handling
- idempotent consumers
- visibility/lease timeout
- backpressure
- monitoring

## Event vs Command
Events describe something that happened.

Commands request a specific action.

Keep semantics explicit.

## Definition of Done
Slow/retryable workloads do not block core API requests.
