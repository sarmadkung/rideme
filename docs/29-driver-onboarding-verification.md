# 29 — Driver Onboarding & Verification

## Objective
Allow a user to become a verified driver without mixing verification state with availability state.

## Flow
```text
User
 -> Become a Driver
 -> Personal Information
 -> Identity Documents
 -> Driver License
 -> Add Vehicle
 -> Vehicle Documents
 -> Capabilities
 -> Submit
 -> Verification Review
 -> Approved
 -> Can Go Online
```

## Driver Verification States
```text
NOT_STARTED
IN_PROGRESS
SUBMITTED
UNDER_REVIEW
APPROVED
REJECTED
SUSPENDED
```

## Required Information
Configurable by market and vehicle/service type.

Potential fields:
- legal name
- phone
- profile photo
- identity document
- license
- emergency contact where legally/operationally appropriate

Do not hard-code country-specific document requirements into the domain model.

## Document Model
```text
DriverDocument {
  id
  driver_id
  type
  number
  file_key
  issued_at
  expires_at
  status
  rejection_reason
}
```

Statuses:
```text
PENDING
VERIFIED
REJECTED
EXPIRED
```

## Upload Flow
Use signed object-storage upload URLs.

```text
Mobile
 -> Request Upload URL
 -> Upload directly to object storage
 -> Confirm Upload
 -> Document record
```

Do not route large document files through the main API unless necessary.

## Verification
MVP:
- manual admin review
- document status
- rejection reason
- resubmission

Later:
- identity-verification provider
- OCR-assisted review
- automated expiry checks

## Expiry
A scheduled process identifies documents approaching expiry.

Driver receives reminders.

Expired mandatory documents can prevent the driver from going online.

## Admin Review
Admin screen should show:
- driver profile
- submitted documents
- vehicle
- prior review history
- approve/reject actions
- reason field

Every review action is audited.

## Definition of Done
- onboarding is resumable
- document uploads are secure
- verification states are enforced server-side
- rejected documents can be resubmitted
- approved driver can continue to vehicle eligibility
- expiry rules are supported
