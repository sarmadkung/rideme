# 87 — Waiting, Loading & Unloading

## Objective
Model time spent at pickup/drop-off locations for cargo services.

## States
Track:
```text
ARRIVED
WAITING
LOADING
LOADED
UNLOADING
UNLOADED
```

## Waiting
Use configurable grace periods.

After grace:
```text
waiting_chargeable = true
```

## Loading Assistance
Some jobs may require:
- driver only
- driver + helper
- customer loading
- merchant loading

## Helper Requirement
Represent helper as an explicit job requirement.

Do not infer it only from vehicle type.

## Proof
Record relevant timestamps:
```text
arrived_at
loading_started_at
loaded_at
unloading_started_at
unloaded_at
```

## Definition of Done
Cargo-specific time and assistance requirements can affect operations and pricing without corrupting the main job state.
