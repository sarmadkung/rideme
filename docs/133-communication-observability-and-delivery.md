# 133 — Communication Observability & Delivery

## Objective
Measure communication reliability across all channels.

## Metrics
```text
notification latency
provider success rate
failure rate
retry count
SMS delivery rate
push delivery rate
email bounce rate
chat delivery latency
support first response time
support resolution time
```

## Message Trace
Use:
```text
correlation_id
event_id
notification_id
provider_message_id
```

This allows debugging from domain event to provider result.

## Retry
Use exponential backoff and bounded attempts.

Do not retry permanent failures indefinitely.

## Dead Letter
Failed messages that cannot be processed should enter an operationally visible dead-letter workflow.

## Definition of Done
Communication failures can be detected, traced and recovered without guessing.
