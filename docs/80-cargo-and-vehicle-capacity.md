# 80 — Cargo & Vehicle Capacity

## Objective
Match cargo requirements against real vehicle capabilities.

## Cargo Attributes
```text
weight
length
width
height
volume
item_count
fragile
temperature_sensitive
special_handling
```

## Vehicle Attributes
```text
max_weight
max_volume
cargo_length
cargo_width
cargo_height
vehicle_type
equipment
```

## Hard Constraints
Reject when:
```text
cargo_weight > vehicle.max_weight
OR
cargo_dimensions > supported_dimensions
OR
required_equipment missing
```

## Multiple Items
Calculate:
```text
total_weight
total_volume
maximum_dimensions
special_requirements
```

Future packing optimization can be added later.

## Definition of Done
Dispatch cannot assign cargo to a vehicle that is physically or operationally incapable of carrying it.
