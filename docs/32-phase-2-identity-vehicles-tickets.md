# 32 — Phase 2 Identity & Vehicles Engineering Tickets

## Goal
Complete identity, driver onboarding, vehicle verification, capability resolution, and driver availability.

---

## IDV-001 — User & Role Schema
Implement:
- users
- user_roles
- sessions
- indexes and constraints

Acceptance:
- normalized unique phone identity works
- users may hold multiple roles

---

## IDV-002 — OTP Provider Interface
Create provider abstraction.

Acceptance:
- development provider
- production provider adapter point
- OTP hashes only
- expiry and attempt limits

---

## IDV-003 — OTP Authentication API
Implement:
```text
/request
/verify
/refresh
/logout
/me
```

Acceptance:
- new user can authenticate
- existing user resolves correctly
- refresh rotation and revocation work

---

## IDV-004 — Mobile Auth Package
Implement shared auth client:
- token handling
- secure refresh-token storage
- session restoration
- logout
- unauthorized handling

Acceptance:
Customer and Driver apps both consume the same abstraction.

---

## IDV-005 — Authorization Middleware
Implement role and resource checks.

Acceptance:
Authorization tests cover cross-user and cross-merchant access attempts.

---

## IDV-006 — Driver Profile
Implement driver entity, API and onboarding progress.

Acceptance:
Onboarding can be stopped and resumed.

---

## IDV-007 — Document Upload Infrastructure
Implement signed object-storage uploads.

Acceptance:
Files upload directly to storage and are associated with the correct driver/vehicle.

---

## IDV-008 — Driver Document Verification
Implement document states, expiry and admin review.

Acceptance:
Admin approval/rejection is audited and rejection supports resubmission.

---

## IDV-009 — Vehicle Registration
Implement vehicle CRUD for drivers.

Acceptance:
Vehicle ownership and driver relationship are validated server-side.

---

## IDV-010 — Vehicle Documents
Implement registration/required-document workflow.

Acceptance:
Required documents vary by configured vehicle type/market.

---

## IDV-011 — Vehicle Verification Admin
Admin can:
- review
- approve
- reject
- suspend

Acceptance:
Only verified vehicles become active.

---

## IDV-012 — Capability Resolver
Implement backend capability calculation.

Inputs:
- vehicle type
- vehicle capacity
- verification
- driver status/license
- market rules

Acceptance:
Client cannot grant itself a capability.

---

## IDV-013 — Active Vehicle
Driver selects one active vehicle for MVP.

Acceptance:
Only verified and eligible vehicles can be activated.

---

## IDV-014 — Driver Availability
Implement:
```text
online
offline
pause
```

Acceptance:
All server-side preconditions are enforced.

---

## IDV-015 — Mobile Location Foundation
Integrate location abstraction in Driver app.

Acceptance:
- foreground location works
- development build supports future native customization
- permissions have clear UX
- location failures are handled

---

## IDV-016 — Realtime Driver Location
Send authenticated location updates to backend.

Acceptance:
Current location and freshness are visible to operational services.

---

## IDV-017 — Redis Driver State
Implement current driver operational state.

Acceptance:
Online/offline and location updates are atomic enough for dispatch consumption.

---

## IDV-018 — Admin Driver/Vehicle Screens
React/Vite admin screens for:
- driver review
- document review
- vehicle review
- verification history

Acceptance:
Admin can complete the full verification workflow.

---

## IDV-019 — Audit Events
Audit:
- role changes
- driver verification
- vehicle verification
- suspension
- sensitive identity changes

Acceptance:
Audit records cannot be edited through ordinary admin APIs.

---

## IDV-020 — Phase 2 E2E
Critical flow:

```text
User signs in
 -> becomes driver
 -> completes profile
 -> uploads documents
 -> adds vehicle
 -> admin verifies
 -> capabilities resolve
 -> driver activates vehicle
 -> driver goes online
 -> location reaches backend
```

Acceptance:
The flow passes in CI/staging.

## Phase 2 Exit Criteria

Phase 2 is complete when a real driver can be securely onboarded, verified, assigned a valid vehicle/capability set, go online, and publish a trustworthy current location.

Next phase: **Jobs, Quotes & Booking**.
