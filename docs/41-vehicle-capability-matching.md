# 41 — Vehicle & Capability Matching

## Objective
Ensure jobs are offered only to vehicles that can legally and operationally perform them.

## Matching Layers
```text
Job Requirements
      ↓
Vehicle Eligibility
      ↓
Driver Eligibility
      ↓
Market Rules
      ↓
Dispatch Score
```

## Required Attributes
Examples:
- passenger capacity
- cargo capacity
- dimensions
- vehicle class
- service capability
- equipment
- license category
- document status

## Hard Constraints
Hard constraints must reject a candidate.

Examples:
```text
required_capacity > vehicle_capacity
required_capability missing
vehicle not verified
driver not authorized
mandatory document expired
```

## Soft Preferences
Soft preferences influence score.

Examples:
- preferred vehicle class
- distance
- driver experience
- service familiarity

## Capacity
Cargo jobs should validate:
```text
weight <= max_weight
dimensions <= supported_dimensions
```

Do not use weight alone.

## Multi-Item Jobs
Future support can calculate:
```text
total_weight
volume
item_count
special_requirements
```

## Passenger Safety
Ride jobs must ensure:
```text
passenger_count <= passenger_capacity
```

## Definition of Done
No dispatch path can bypass hard capability constraints.
