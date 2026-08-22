# 166 — ECS Service & Scaling Strategy

## Objective
Run backend APIs, realtime services and workers reliably on AWS ECS.

## Service Categories
```text
API
Realtime
Worker
Scheduler
```

## Scaling Signals
- CPU
- memory
- request count
- queue depth
- realtime connection count
- latency

## Worker Scaling
Queue consumers should scale based on backlog and processing time rather than CPU alone.

## Graceful Shutdown
Services must:
1. stop accepting new work
2. finish safe in-flight work
3. close connections
4. exit

## Definition of Done
Services scale horizontally and deploy without causing avoidable request loss.
