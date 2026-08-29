# Autonomous Implementation Agent Instructions

## 1. Mission

You are the primary software implementation agent for this logistics platform.

The project documentation contains the product requirements, architecture, domain models, workflows, technical specifications, testing requirements, security requirements, and implementation procedures.

Your job is to transform that documentation into a working, production-quality platform.

The platform includes:

- Customer mobile application
- Driver/provider mobile application
- ReactJS administration dashboard
- Backend APIs
- PostgreSQL/PostGIS
- Redis
- Background workers
- Queues/events
- Realtime communication
- Authentication and authorization
- Ride sharing / ride booking
- Grocery delivery
- Parcel delivery
- Cargo/loader/truck services
- Payments
- Provider earnings
- Merchant operations
- Dispatch
- Notifications
- Support
- Analytics
- Infrastructure
- Monitoring
- Testing
- Security

---

# 1a. Execution Policy

**`docs/IMPLEMENTATION_EXECUTION_POLICY.md` defines this project's
implementation-efficiency and verification policy. It is the source of truth for
*how* implementation work is performed.**

It governs: the primary objective (implement quickly while keeping quality
sufficient), progressive documentation loading, the implementation loop, CI
priority, proportional testing, E2E restraint, build scope, architecture and
business-decision handling, code quality, token efficiency, autonomous
execution, progress tracking, commits, and stop conditions.

That policy is not restated here. Read it; do not infer it from this document.

Where these instructions and the execution policy differ on *how* work is
performed, **the execution policy wins** — it was written against the built
repository, this document against an empty one. Where they differ on *what* is
built, the Tier A documentation (`00`–`190`) governs both.

The hierarchy:

```text
AGENTS.md                             mission and protocol
    ↓
IMPLEMENTATION_EXECUTION_POLICY.md    how work is performed
    ↓
implementation skills                 the operational loop
    ↓
authoritative documentation           what to build (Tier A)
    ↓
implementation  →  targeted verification
```

## Repository Skills

Reusable agent skills are stored in `.skills/`.

Both supported coding agents consume the same canonical skills:

- Claude Code: `.claude/skills -> ../.skills`
- Codex: `.agents/skills -> ../.skills`

Do not create duplicate Claude-specific or Codex-specific copies of a skill unless the skill
genuinely requires agent-specific behavior.

When modifying a skill, update the canonical version under `.skills/`.

# 2. Documentation Is the Source of Truth

The `/docs` directory contains the project's implementation documentation.

Do NOT assume that document numbers represent a simple linear implementation order.

The implementation order must be determined by:

1. dependencies
2. architecture
3. domain ownership
4. infrastructure requirements
5. current repository state
6. implementation status
7. testing requirements

Important control documents include:

- Document 365 — Master Document Index
- Document 366 — Implementation Dependency Graph
- Document 367 — Implementation Phases
- Document 368 — Agent Work Queue
- Document 443 — Agent Progress State
- Document 444 — Final Agent Execution Protocol
- Document 564 — Agent Workflow Execution Specification

Read these before beginning implementation.

---

# 3. First Rule: Audit Before Coding

DO NOT immediately start writing application code.

First inspect:

- repository structure
- existing source code
- package manager
- workspace configuration
- applications
- packages
- infrastructure
- CI/CD
- environment configuration
- database configuration
- existing migrations
- tests
- documentation
- dependency versions
- existing APIs
- existing mobile applications
- existing dashboard

Determine what already exists.

Never recreate functionality that already exists.

---

# 4. Documentation Audit

Before major implementation begins, audit all documentation.

Search the entire `/docs` directory.

Identify:

- duplicate specifications
- conflicting requirements
- contradictory architecture decisions
- duplicate entities
- conflicting state machines
- conflicting API conventions
- conflicting database assumptions
- obsolete documents
- missing dependencies
- documents that depend on undocumented behavior
- documents that are conceptual only
- documents that are implementation-ready

Create:

```text
docs/DOCUMENT_AUDIT.md
```

The audit should contain:

```text
Total documents:
Authoritative:
Needs review:
Duplicate:
Conflicting:
Obsolete:
Missing dependency:
Implementation-ready:
```

Do not delete documents automatically.

If two documents conflict, determine which document is authoritative based on:

1. latest explicit architecture decision
2. master architecture
3. canonical domain model
4. dependency graph
5. README
6. existing validated implementation

If ambiguity remains, stop and record the issue instead of silently choosing.

---

# 5. Create the Implementation Control Files

Create and maintain:

```text
docs/
├── DOCUMENT_AUDIT.md
├── DOCUMENT_CONFLICTS.md
├── IMPLEMENTATION_PLAN.md
├── IMPLEMENTATION_STATUS.md
├── ARCHITECTURE_DECISIONS.md
└── BLOCKED_TASKS.md
```

These files become the operational control layer for implementation.

---

# 6. Implementation Status

Maintain:

```text
docs/IMPLEMENTATION_STATUS.md
```

Every implementation item must have one of:

* NOT_STARTED
* READY
* IN_PROGRESS
* BLOCKED
* IMPLEMENTED
* VERIFIED
* DEFERRED
* OBSOLETE

Never mark something `VERIFIED` merely because code was written.

---

# 7. Definition of Done

A task is NOT complete when:

* code compiles
* a component exists
* an endpoint exists
* a database table exists

A task is complete only when appropriate:

```text
Implementation
+
Tests
+
Integration
+
Security
+
Error handling
+
Observability
+
Migration
+
Documentation
+
Verification
```

have been completed.

---

# 8. Never Implement the Entire Platform at Once

Work incrementally.

Implement one coherent vertical slice at a time.

Preferred pattern:

```text
Infrastructure
    ↓
Database
    ↓
Backend domain
    ↓
API
    ↓
Realtime
    ↓
Mobile/Web client
    ↓
Tests
    ↓
Observability
    ↓
Verification
```

Do not build hundreds of disconnected screens first.

---

# 9. Recommended Implementation Order

Use this as the high-level execution strategy.

## Phase 0 — Documentation Audit

* Audit all documentation
* Resolve conflicts
* Build dependency graph
* Create implementation status
* Identify first executable tasks

Do not build product functionality during this phase unless required to understand the repository.

---

# Phase 1 — Repository Foundation

Implement:

* monorepo/workspace
* package boundaries
* shared configuration
* TypeScript configuration
* linting
* formatting
* testing infrastructure
* environment management
* local development tooling
* CI
* development scripts

Verify:

```bash
install
typecheck
lint
test
build
```

---

# Phase 2 — Infrastructure Foundation

Implement:

* PostgreSQL
* PostGIS
* Redis
* queue infrastructure
* object storage abstraction
* local development services
* environment configuration
* secrets handling

Everything must work locally.

---

# Phase 3 — Backend Foundation

Implement:

* application bootstrap
* configuration
* logging
* error handling
* validation
* API framework
* database layer
* migrations
* transactions
* idempotency
* authorization
* background jobs
* health checks
* observability

---

# Phase 4 — Authentication

Implement:

* registration
* login
* verification
* sessions
* refresh tokens
* logout
* account recovery
* device management
* authorization
* roles
* permissions

Test security aggressively.

---

# Phase 5 — Canonical Domain

Implement foundational entities:

```text
User
Customer
Provider
Merchant
Vehicle
VehicleType
Organization
Address
Service
Booking
Order
Job
Payment
Ledger
Settlement
```

Do not create duplicate versions of these entities in different modules.

---

# Phase 6 — ReactJS Dashboard Foundation

Build:

* authentication
* layout
* navigation
* permissions
* API client
* TanStack Query
* tables
* forms
* error handling
* notifications
* realtime infrastructure
* maps infrastructure

The dashboard should be operationally useful rather than merely visually complete.

---

# Phase 7 — React Native Foundation

Build the shared mobile architecture.

Customer app:

```text
Authentication
Navigation
API client
Query cache
Realtime
Offline handling
Push notifications
Deep links
Location
Maps
```

Driver app:

```text
Authentication
Navigation
API client
Realtime
Location
Background execution
Push notifications
Maps
Job state
Availability
```

Do not duplicate business logic between iOS and Android.

Use React Native for shared application logic.

Use native modules/platform-specific code only where required.

---

# Phase 8 — First Vertical Slice: Ride

Implement the complete ride lifecycle.

```text
Customer
   ↓
Request ride
   ↓
Quote
   ↓
Confirm
   ↓
Dispatch
   ↓
Provider offer
   ↓
Provider accepts
   ↓
Navigation
   ↓
Arrival
   ↓
Pickup
   ↓
Trip
   ↓
Completion
   ↓
Payment
   ↓
Receipt
   ↓
Rating
```

Everything must work end-to-end.

Do not proceed to major new services until this vertical slice is stable.

---

# Phase 9 — Delivery

Implement:

* parcel booking
* delivery quote
* dispatch
* pickup
* delivery
* proof
* payment
* tracking
* exceptions

---

# Phase 10 — Grocery

Implement:

```text
Merchant
   ↓
Catalog
   ↓
Inventory
   ↓
Customer
   ↓
Cart
   ↓
Checkout
   ↓
Order
   ↓
Merchant acceptance
   ↓
Picking
   ↓
Packing
   ↓
Provider assignment
   ↓
Pickup
   ↓
Delivery
```

---

# Phase 11 — Cargo

Implement:

* cargo request
* dimensions
* weight
* vehicle requirements
* quote
* provider matching
* truck/loader assignment
* loading
* transport
* delivery
* proof
* settlement

---

# Phase 12 — Financial System

Implement and verify:

* payment intents
* payment methods
* capture
* refunds
* ledger
* commissions
* provider earnings
* merchant settlement
* reconciliation
* invoices
* receipts
* disputes

Financial state must be authoritative and auditable.

Never calculate financial truth independently in multiple clients.

---

# Phase 13 — Operations

Implement:

* dispatch dashboard
* live map
* provider monitoring
* booking monitoring
* order monitoring
* incident management
* support
* refunds
* financial operations
* provider management
* vehicle management
* merchant management

---

# Phase 14 — Testing

Implement:

* unit tests
* integration tests
* API contract tests
* realtime tests
* database tests
* worker tests
* ReactJS tests
* React Native tests
* E2E tests
* performance tests
* failure tests
* security tests

Critical journeys must have automated E2E tests.

---

# Phase 15 — Production Readiness

Verify:

* security
* backups
* restore
* monitoring
* alerting
* logging
* tracing
* rate limiting
* abuse prevention
* disaster recovery
* deployment
* rollback
* mobile releases
* database migrations
* secrets
* cost controls
* capacity

---

# 10. Vertical Slice Rule

Whenever implementing a feature, prefer:

```text
Database
+
Backend
+
API
+
Realtime
+
Mobile/Web
+
Tests
```

over implementing:

```text
100 database tables
then
100 APIs
then
100 screens
```

The first approach produces working software.

---

# 11. Database Rules

PostgreSQL is authoritative for transactional state.

Use constraints wherever possible.

Important invariants should be enforced using:

* foreign keys
* unique constraints
* check constraints
* transactions
* indexes
* locking
* application-level validation

Never rely solely on frontend validation.

---

# 12. API Rules

Every API must define:

* authentication
* authorization
* request schema
* response schema
* validation
* errors
* pagination where applicable
* idempotency where applicable
* rate limiting where applicable
* observability

Avoid arbitrary endpoint creation.

Search existing APIs first.

---

# 13. Realtime Rules

Realtime events are not the database source of truth.

The authoritative state is stored server-side.

Clients must be able to recover from:

* disconnect
* duplicate event
* missing event
* out-of-order event
* stale state

Implement snapshot/resynchronization mechanisms.

---

# 14. Mobile Rules

React Native should be the default implementation for shared application behavior.

Do NOT create separate iOS and Android screens unnecessarily.

Use:

```text
Shared React Native
        ↓
Platform-specific abstraction
        ↓
Native implementation
```

Only create native modules when the platform capability/performance requires it.

Examples:

* background location
* advanced location services
* high-performance native processing
* push notification integration
* secure storage
* device APIs

---

# 15. Performance Rules

Do not prematurely optimize.

Measure first.

For performance-sensitive areas:

* profile
* benchmark
* identify bottleneck
* optimize
* benchmark again

Pay particular attention to:

* location updates
* maps
* realtime events
* dispatch
* database queries
* large lists
* mobile startup
* background execution

---

# 16. Security Rules

Assume every client is untrusted.

Never trust:

* mobile input
* dashboard input
* provider input
* browser state
* client-side prices
* client-side permissions
* client-side status transitions

The backend must verify everything important.

Never expose:

* secrets
* private keys
* internal credentials
* unnecessary personal data
* unnecessary location history
* internal financial data

---

# 17. Financial Safety

Financial operations require special care.

Before implementing payment functionality:

1. understand the ledger model
2. understand payment-provider behavior
3. implement idempotency
4. implement reconciliation
5. test duplicate webhooks
6. test retries
7. test partial failure
8. test refunds
9. test chargebacks/disputes

Never assume a payment provider request succeeded merely because the HTTP request succeeded.

---

# 18. Location Safety

Location is sensitive operational data.

Implement:

* minimum necessary collection
* controlled retention
* access control
* secure transmission
* secure storage
* sampling strategy
* stale-location detection
* provider/customer visibility rules

Do not store high-frequency location data indefinitely without a reason.

---

# 19. External Services

Every external provider must be accessed through an adapter.

Example:

```text
Application
    ↓
Provider Interface
    ↓
Adapter
    ↓
External Service
```

Do not scatter provider-specific SDK calls throughout the application.

This applies to:

* maps
* routing
* payments
* SMS
* email
* push notifications
* identity verification
* storage

---

# 20. Handling Existing Code

If functionality already exists:

DO NOT rewrite it automatically.

First determine:

* whether it meets the documentation
* whether it has tests
* whether it is production-safe
* whether it conflicts with the architecture

Prefer incremental refactoring.

---

# 21. Before Every Commit

Run relevant:

```bash
git diff
git status
```

Then:

```bash
typecheck
lint
test
build
```

Run E2E tests when the affected workflow requires them.

Never commit knowingly broken code unless the repository explicitly uses staged/incomplete branches and the status is documented.

---

# 22. Commit Strategy

Prefer small, coherent commits.

Examples:

```text
feat(auth): implement provider registration
feat(dispatch): implement provider offers
feat(ride): implement booking lifecycle
feat(grocery): implement cart validation
fix(payment): handle duplicate webhook
test(dispatch): add assignment concurrency tests
refactor(api): centralize error mapping
```

Avoid giant commits containing unrelated work.

---

# 23. Migration Rules

Every database migration must be:

* reversible where practical
* backward-compatible during rolling deployments
* tested
* documented
* safe for existing data

For large tables:

```text
expand
    ↓
migrate
    ↓
backfill
    ↓
verify
    ↓
switch
    ↓
contract
```

Do not perform dangerous destructive migrations blindly.

---

# 24. Autonomous Execution

You are allowed to continue from one completed task to the next.

However, you MUST stop when:

* requirements conflict
* architecture is ambiguous
* destructive migration is required without approval
* financial behavior is unclear
* security behavior is unclear
* data loss is possible
* an external credential is required
* a required external service is unavailable
* the documentation is contradictory
* a task requires a product decision not specified in the documentation

When blocked, create:

```text
docs/BLOCKED_TASKS.md
```

Record:

```text
Task
Reason
Relevant documents
What decision is required
Possible options
Recommended option
```

Do not invent business requirements.

---

# 25. Progress Tracking

After every meaningful implementation unit update:

```text
docs/IMPLEMENTATION_STATUS.md
```

Example:

```md
| Task | Status | Tests | Verified |
|------|--------|-------|----------|
| Auth registration | IMPLEMENTED | PASS | YES |
| Auth recovery | IN_PROGRESS | - | NO |
| Ride quote | READY | - | NO |
| Dispatch | BLOCKED | - | NO |
```

The status must represent reality.

---

# 26. Never Claim Success Without Evidence

Do not say:

> "Implemented."

unless you can demonstrate:

* source code exists
* tests exist
* tests pass
* integration works
* relevant checks pass

For significant workflows, provide evidence such as:

```text
Tests: 42 passed
Typecheck: passed
Lint: passed
Build: passed
E2E: 8 passed
Migration: verified
```

---

# 27. Product Development Philosophy

Build the smallest correct implementation first.

Do not add speculative complexity.

Do not implement features merely because the documentation mentions future extensibility.

Prioritize:

1. correctness
2. security
3. reliability
4. maintainability
5. observability
6. performance
7. extensibility

---

# 28. Final Rule

The objective is NOT to finish 564 documents.

The objective is to build a:

* working
* secure
* maintainable
* scalable
* testable
* observable

logistics platform.

Documents are instructions.

Code is the implementation.

Tests are evidence.

Production verification is the final authority.

---

# 29. Starting Instruction

When this AGENTS.md is first loaded, perform these actions in order:

1. Inspect repository.
2. Inspect `/docs`.
3. Read `docs/README.md` (there is no root-level README).
4. Read documents 365, 366, 367, 368, 443, 444, and 564.
5. Audit all documentation.
6. Create/update:

   * DOCUMENT_AUDIT.md
   * DOCUMENT_CONFLICTS.md
   * IMPLEMENTATION_PLAN.md
   * IMPLEMENTATION_STATUS.md
   * ARCHITECTURE_DECISIONS.md
   * BLOCKED_TASKS.md
7. Determine the first unblocked implementation task.
8. Explain the proposed first implementation phase.
9. Begin implementation.
10. Test it.
11. Update implementation status.
12. Continue to the next unblocked task.

Do not start by implementing random documents.

Follow dependencies.

Build vertically.

Test continuously.

Keep the system production-quality.
