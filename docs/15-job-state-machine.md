# Job State Machine

## Main Flow

```text
DRAFT -> QUOTED -> REQUESTED -> SEARCHING -> ASSIGNED -> ACCEPTED
      -> ARRIVING -> AT_PICKUP -> IN_PROGRESS -> AT_DROPOFF -> COMPLETED
```

Terminal states: `CANCELLED`, `FAILED`, `EXPIRED`, `DISPUTED`.

Clients cannot directly set arbitrary status. Backend commands perform transitions.

Every transition emits `JobStatusChanged` containing job ID, previous/new status, actor, timestamp and metadata.

Delivery evidence should be represented through proof/events such as pickup photo, pickup confirmation, delivery OTP and proof of delivery.
