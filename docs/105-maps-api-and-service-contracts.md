# 105 — Maps API & Service Contracts

## Internal APIs

### Geocode
```text
POST /geo/geocode
```

### Reverse Geocode
```text
POST /geo/reverse-geocode
```

### Route
```text
POST /geo/route
```

### Matrix
```text
POST /geo/matrix
```

### ETA
```text
POST /geo/eta
```

The public API should expose normalized domain data rather than provider-specific response formats.

## Security
Validate:
- coordinates
- request size
- route point count
- authenticated access where required
- rate limits

## Versioning
Provider adapters can change internally without breaking client contracts.

## Definition of Done
Consumers use stable internal geographic contracts.
