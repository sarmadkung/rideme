# 108 — Driver Identity & Vehicle Verification

## Objective
Ensure drivers and registered vehicles meet platform requirements before activation.

## Driver Verification
Possible states:
```text
UNVERIFIED
PENDING
VERIFIED
REJECTED
SUSPENDED
EXPIRED
```

## Vehicle Verification
Verify:
- registration
- ownership/authorization
- vehicle type
- required documents
- service eligibility
- document expiry

## Document Model
```text
document_type
document_reference
issued_at
expires_at
status
reviewed_at
reviewed_by
```

Store secure references rather than unnecessary raw document copies.

## Expiration
Scheduled checks should identify documents approaching expiry.

Expired critical documents can automatically restrict relevant services.

## Definition of Done
Only eligible drivers and vehicles can become active for services requiring verification.
