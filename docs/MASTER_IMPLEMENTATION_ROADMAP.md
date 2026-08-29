# Master Implementation Roadmap

The single ordered implementation roadmap for the RideMe logistics platform after Phase 1.

This document supersedes the phase *ordering* in `AGENTS.md` §9 and in
`IMPLEMENTATION_PLAN.md` for execution purposes. It does not supersede either document on
anything else — mission, protocol, sequencing principle, token policy and deferred-decision
tracking continue to live where they already live.

**Authority order** (unchanged from `IMPLEMENTATION_EXECUTION_POLICY.md`):

```text
AGENTS.md                             mission and protocol
    ↓
IMPLEMENTATION_EXECUTION_POLICY.md    how work is performed
    ↓
MASTER_IMPLEMENTATION_ROADMAP.md      what order work is performed in  ← this document
    ↓
implementation skills                 the operational loop
    ↓
authoritative documentation           what to build (Tier A, 000–190)
    ↓
implementation                        smallest complete solution
    ↓
targeted verification                 proportional to impact
```

**Objective:** build the product quickly while maintaining correctness, maintainability,
security and sufficient verification. Verification is proportional, not exhaustive
(`IMPLEMENTATION_EXECUTION_POLICY.md` §A, §E).

**Reference convention:** a bare number (`005`) is the shorthand used throughout the control
documents; the file it names is `docs/05-dispatch-pricing.md` — documents below 100 are not
zero-padded on disk. Full references in this document use the on-disk name.

**Nothing in this roadmap invents functionality.** Every phase scope traces to an existing
Tier A document. Where a required rule does not exist in the documentation, it is named as a
business decision and left open, never guessed.

---

## Phase Numbering Reconciliation

Three phase numberings exist in this repository. They are **not** interchangeable, and every
cross-reference in older documents uses an older numbering. This table is the translation key.

| This roadmap | `IMPLEMENTATION_PLAN.md` spine | `AGENTS.md` §9 |
|---|---|---|
| 1 — Engineering foundation | 1 (+ local half of 2) | 1 (+ 2) |
| 2 — Contracts and core platform foundation | 3 — backend foundation | 3 — backend foundation |
| 3 — Identity, authn, authz | 4 — authentication | 4 — authentication |
| 4 — Core domain model | 5 — canonical domain | 5 — canonical domain |
| 5 — Providers, vehicles, eligibility | part of 5 | part of 5 |
| 6 — Location and realtime foundation | 8 — location + realtime | part of 8 |
| 7 — Ride booking | 6 (pricing/quote) + 9 (ride slice) | 8 — first vertical slice: ride |
| 8 — Dispatch engine | 7 — dispatch | part of 8 |
| 9 — Delivery and cargo | 10 + 12 | 9 + 11 |
| 10 — Grocery and merchant platform | 11 — grocery | 10 — grocery |
| 11 — Payments and financial system | 13 — financial completeness | 12 — financial system |
| 12 — Mobile production features | grown alongside slices | 7 — React Native foundation |
| 13 — Operational dashboards | 14 — operations console | 6 — dashboard foundation |
| 14 — Production infrastructure and observability | 15 (cloud half) | 15 (part) |
| 15 — Hardening, performance, release | 15 — production readiness | 14 + 15 |

The `BUSINESS_DECISION_REGISTER.md` "Timeline" table is written against the
`IMPLEMENTATION_PLAN.md` numbering. Use the per-phase **Business decisions** blocks below
instead; they are the remapped, authoritative version.

Differences of substance — not just numbering — are recorded in
[Conflicts Discovered](#conflicts-discovered). None has been silently resolved.

---

## Dependency Graph

```text
Phase 1  ENGINEERING FOUNDATION                                   ✅ COMPLETE
    ↓
Phase 2  CONTRACTS AND CORE PLATFORM FOUNDATION
    ↓
Phase 3  IDENTITY, AUTHENTICATION AND AUTHORIZATION
    ↓
Phase 4  CORE DOMAIN MODEL
    ↓
Phase 5  PROVIDERS, VEHICLES AND SERVICE ELIGIBILITY
    ↓
Phase 6  LOCATION AND REALTIME FOUNDATION
    ↓
Phase 7  RIDE BOOKING
    ↓
Phase 8  DISPATCH ENGINE
    ├────────────────┬────────────────┐
    ↓                ↓                │
Phase 9          Phase 10             │
DELIVERY         GROCERY AND          │
AND CARGO        MERCHANT             │
    └────────────────┴────────────────┘
                     ↓
Phase 11  PAYMENTS AND FINANCIAL SYSTEM
                     ↓
Phase 12  MOBILE PRODUCTION FEATURES  ─┐
                     ↓                 │ portions may overlap
Phase 13  OPERATIONAL DASHBOARDS      ─┘
                     ↓
Phase 14  PRODUCTION INFRASTRUCTURE AND OBSERVABILITY
                     ↓
Phase 15  HARDENING, PERFORMANCE AND RELEASE
```

### Parallelism rules

Phases are **sequential by default**. Work may run in parallel only where the list below
explicitly permits it, and only after the named prerequisite is complete.

| Parallel work | Permitted after | Condition |
|---|---|---|
| Phase 9 ‖ Phase 10 | Phase 8 complete | They share only the Job core and the dispatch engine, both frozen by then. Do not let either fork a parallel booking entity. |
| Phase 12 customer app ‖ Phase 12 driver app | Phase 11 complete | Separate applications, separate screens, shared `@platform/*` only. |
| Phase 13 admin ‖ Phase 13 merchant dashboard | Phase 11 complete | Separate applications. Merchant dashboard additionally requires Phase 10. |
| Phase 12 ‖ Phase 13 | Phase 11 complete | Different surfaces consuming a frozen API. If either forces an API change, the API change is sequenced first and both re-sync. |
| Phase 14 observability groundwork | Phase 11 complete | Only where a domain already emits the signal being observed. Do not build ahead of the code. |

Do not force artificial serialization when dependency analysis proves work is independent.
Equally, do not declare independence to gain concurrency — if two streams touch the same
module, they are sequential.

---

## Cross-Cutting Capabilities

Six capabilities span multiple phases. They are **not** phases, and they are **not** optional
extras. Each has exactly **one architectural boundary**, created by its first real consumer and
extended by every later one.

**The rule:** a capability's boundary is created at the phase that first genuinely needs it —
never earlier, never speculatively. Every subsequent consumer extends it *behind the same
boundary*. **A second implementation of a capability is a defect, not an optimization.**

| ID | Capability | Boundary created | Complete by | Resolves |
|---|---|---|---|---|
| CAP-1 | Pricing and quote | Phase 7 | Phase 13 | R-2 |
| CAP-2 | Maps, routing and ETA | Phase 6 | Phase 14 | R-6 |
| CAP-3 | Safety, trust and fraud | Phase 3 (device/session trust) | Phase 15 | R-6 |
| CAP-4 | Notifications, chat and support | **Phase 3 (mandatory — OTP)** | Phase 14 | R-6 |
| CAP-5 | Analytics and event taxonomy | Phase 2 (envelope only) | Phase 15 | R-6 |
| CAP-6 | Client platform | Phase 2 (api-client) | Phase 13 | R-5 |

---

### CAP-1 — Pricing and Quote

**There is no pricing phase.** Pricing is a shared domain capability with service-specific rule
sets behind one boundary. `005` gives four fare formulas that share `base + distance`:

```text
ride    base + distance + time + demand + vehicle adjustment
parcel  base + distance + size/weight + urgency
cargo   base + distance + vehicle + capacity + loading + waiting + schedule
grocery delivery fee + service rules
```

Four formulas, one shape. Implemented four times, they drift four ways.

| Phase | Increment |
|---|---|
| 2 | Money representation (BD-07) — the contract pricing must obey. Not pricing itself. |
| **7** | **Boundary created.** Fare engine, service-parameterized from the first line. Ride rule set implemented; `demand` present but inert (BD-02). Quote lifecycle per `034`. |
| 8 | `price_fit` scoring term consumes the quote. **No pricing logic in dispatch.** |
| 9 | Parcel and cargo rule sets added **behind the boundary** (`086`, `087`). Loading/waiting recorded as events, unpriced (BD-13). |
| 10 | Grocery and delivery-fee rule sets behind the boundary. Substitution pricing stays unwired (BD-11). |
| 11 | Commission and promotion act on the ledger, not on the fare engine. |
| 13 | Admin pricing configuration surfaces (`142`). |
| 15 | Confirm or exclude every open pricing value (BD-01, BD-02, BD-13). |

**Documents:** `005` · `034` · `086` · `087`
**Prohibited:** a fare calculation inside the ride, delivery, cargo or grocery module.

---

### CAP-2 — Maps, Routing and ETA

Required by location, ride, dispatch, delivery and cargo. `095` already specifies the interface —
`route()`, `routeMatrix()`, `estimateETA()` — and `093` requires it be provider-agnostic.

| Phase | Increment |
|---|---|
| **6** | **Boundary created.** Routing provider abstraction per `095`. Coordinate and address representation (`093`, `094`). Distance calculation. PostGIS is already present from Phase 1. Geofencing/zones (`097`) only if Phase 5 service-area eligibility demands it. |
| 7 | Route and ETA for the ride quote (the `distance` and `time` fare terms) and for the trip (`096`, `100`). |
| **8** | **Route matrix — `Drivers × Pickup`** per `096`. Feeds `eta_score`, `route_compatibility` and `empty_km`, three of the nine `005` scoring terms. Combined with PostGIS/Redis geo (`042`). |
| 9 | Multi-stop matrix (`Stops × Stops`) for delivery and cargo routing (`082`, `096`). |
| 12 | Map display and navigation handoff on mobile (`100`, `101`). |
| 13 | Dashboard maps and the dispatch console map (`141`). |
| 14 | Provider cost controls and fallback (`104`), caching (`101`). |

**Documents:** `093` · `094` · `095` · `096` · `100` · `101` · `104` · `105`
**Do not** build a complete mapping platform prematurely. Phase 6 delivers an interface and one
provider, not a maps product.

---

### CAP-3 — Safety, Trust and Fraud

`107` defines this as a unified layer across identity, vehicle, trip, emergency, ratings, fraud,
abuse and incidents. Those pieces mature at very different phases — the layer is one boundary,
the capabilities arrive when their data does.

| Phase | Increment |
|---|---|
| 3 | Device and session trust (`116`) — already inside the session lifecycle. |
| 5 | Provider identity and vehicle verification (`108`) — **already Phase 5 scope**; it *is* the verification arm of the safety layer, not a separate build. |
| 7 | Safety event and audit log (`115`) started — trips are what generate the events. Trip safety and SOS surfaces (`109`, `110`) where `021` places them in the ride journey. |
| **8** | **Ratings minimum (`111`)** — `driver_reliability` is a term in the `005` scoring formula, so a reliability signal must exist or the term is dead weight. Minimum viable rating capture and aggregation only. |
| 9 · 10 | Delivery exceptions and merchant issues already carry the operational half; no new safety build. |
| **11** | **Fraud and risk engine (`112`).** `112`'s signals are payment failures, cancellation patterns, promotion abuse and device/account relationships — **none of which exist before Phase 11.** This is the earliest phase where the engine is buildable rather than imaginary. |
| 13 | Safety operations dashboard (`119`), enforcement (`113`), incident management (`114`). |
| 14 · 15 | Safety data privacy and retention (`118`), settled with BD-15/BD-16. Final security review (`183`). |

**Documents:** `107` · `108` · `109` · `110` · `111` · `112` · `113` · `114` · `115` · `116` · `118` · `119`
**Do not** build a complete fraud engine before payment and operational data exist.

---

### CAP-4 — Notifications, Chat and Support

**This capability has a hard, non-negotiable minimum at Phase 3.** `020` states the initial auth
method is phone OTP; `028` specifies `POST /auth/otp/request` and `/auth/otp/verify` and requires
that *"OTP provider must be behind an interface."* **Authentication cannot ship without a
messaging capability.** The roadmap previously had none before Phase 7 — that was the gap.

| Phase | Increment |
|---|---|
| **3** | **Boundary created — mandatory.** Provider-independent messaging interface, SMS/OTP path only (`123`, `121`). No templates, no preferences, no push. |
| 6 | Realtime notification transport **reuses the Phase 6 realtime channel** (`126`). Never a second channel. |
| 7 | Ride status notifications (`121`, `122`). |
| 8 | Dispatch offer notifications — push, and the most latency-sensitive path in the platform (`122`). |
| 9 · 10 | Delivery and grocery order status notifications. |
| 11 | Payment event notifications. |
| 12 | Push registration, preferences (`124`), templates and localization (`125`). |
| 13 | Support ticketing, routing and operational actions (`130`, `131`, `132`). |
| 14 | Communication observability and delivery tracking (`133`). |

**Chat (`127`–`129`) blocks nothing in Phases 3–11.** No acceptance criterion in any of those
phases requires it. Its earliest genuine need is customer↔provider contact during a trip; it is
placed with the **Phase 12** mobile consolidation and may move later without consequence.

---

### CAP-5 — Analytics and Event Taxonomy

`149` is a pipeline — applications → domain events → collection → stream → storage. The pipeline
is worthless before the events exist, but the **event envelope is nearly free now and expensive
to retrofit**, because retrofitting means revisiting every emission site in the platform.

| Phase | Increment |
|---|---|
| **2** | **Event envelope conventions only**, as part of the contract work: `event_id`, `event_name`, `actor_id`, `timestamp`, `source` per `150`. No collection, no storage, no pipeline. |
| 4 → 11 | Each domain emits its documented events at its documented boundary as it lands. **Emission only.** Dispatch events already exist as a NATS requirement (`049`) — analytics consumes them, it does not duplicate them. |
| 13 | Operational metrics and dashboards (`156`, `159`) — the first phase where analytics has a real consumer. |
| 14 | Collection, stream and warehouse storage (`149`, `157`). |
| 15 | Product and marketplace analytics (`151`, `152`), governance and data quality (`161`), experimentation (`160`). |

**Documents:** `149` · `150` · `156` · `157` · `158` · `159` · `161`
**Do not** build an analytics platform before meaningful events exist. Only the taxonomy and the
emission boundary are early — everything else follows its data.

---

### CAP-6 — Client Platform

The horizontal half of R-5. Shared client infrastructure **may** be built centrally; product
workflows **may not**.

**Permitted horizontally, at the phase that first needs it:**
shared React Native infrastructure · shared UI and design-system primitives ·
`@platform/api-client` functionality · navigation foundations

**Required vertically, inside the slice that owns them:**
every product workflow, screen and journey

| Phase | Increment |
|---|---|
| 2 | API client conventions (`@platform/api-client`). |
| 3 → 11 | Each slice adds its own screens and workflows **inside the slice**. Shared primitives are extracted when a second consumer appears, not in anticipation of one. |
| 12 · 13 | Consolidation and productionization of what the slices already built. |

---

### Capability Increment Matrix

What capability work is in scope at each phase. Blank means none.

| Phase | CAP-1 pricing | CAP-2 maps/ETA | CAP-3 safety | CAP-4 notify | CAP-5 analytics | CAP-6 client |
|---|---|---|---|---|---|---|
| 2 | money contract | — | — | — | **envelope** | api-client |
| 3 | — | — | device/session trust | **OTP — mandatory** | emit | slice screens |
| 4 | — | — | — | — | emit | slice screens |
| 5 | — | zones if needed | identity/vehicle verification | — | emit | slice screens |
| 6 | — | **boundary** | — | realtime transport | emit | slice screens |
| 7 | **boundary** | route + ETA | audit log · SOS | ride status | emit | slice screens |
| 8 | consumes quote | **route matrix** | **ratings minimum** | dispatch offers | emit | slice screens |
| 9 | parcel + cargo rules | multi-stop matrix | — | delivery status | emit | slice screens |
| 10 | grocery rules | — | — | order status | emit | slice screens |
| 11 | commission via ledger | — | **fraud engine** | payment events | emit | slice screens |
| 12 | — | map display · navigation | — | push · prefs · templates · **chat** | — | consolidation |
| 13 | admin pricing config | dashboard maps | ops dashboard · enforcement · incidents | support ticketing | metrics · dashboards | consolidation |
| 14 | — | cost · fallback · caching | privacy · retention | delivery observability | collection · stream · storage | — |
| 15 | confirm values | — | final security review | — | product · governance · experimentation | — |

---

## Phase 1 — Engineering Foundation

**Status:** ✅ **COMPLETE and VERIFIED (2026-08-27).** Do **not** redo.

Delivered: pnpm workspace · Turborepo · seven `@platform/*` packages · Go 1.25 backend
foundation (own module, ADR-001) · PostgreSQL + PostGIS · Redis · NATS · MinIO · Docker
development environment · React/Vite admin dashboard shell · two Expo mobile shells ·
environment management · testing foundation (Vitest + jest-expo + `go test`) ·
lint/format/typecheck gates · CI configuration · health and readiness infrastructure ·
observability foundation (structured logging, W3C trace propagation).

Evidence per task: `IMPLEMENTATION_STATUS.md`.

**Only outstanding criterion:** #12 — "CI runs both paths and is green on a pull request" —
**UNVERIFIED**. Run 33077085568 triggered correctly on 2026-08-27 and every job failed to
start: the GitHub account is locked for billing. External to the repository. Under
`IMPLEMENTATION_EXECUTION_POLICY.md` §D this does not block implementation; remote CI
verification is a milestone activity, deferred to **Phase 15** (or earlier, opportunistically,
if billing is restored — it costs nothing to re-check on a push that was happening anyway).

**Re-entry rule:** touch Phase 1 output only to fix a genuine defect discovered later. A
defect fix is not a phase reopening; record it against the phase that found it.

---

## Phase 2 — Contracts and Core Platform Foundation

**Depends on:** Phase 1
**Objective:** finalize the cross-platform contracts every domain phase will depend on, so no
domain code invents its own conventions.

### Scope

- Error contract — one authoritative definition, one HTTP mapping
- API contract conventions — request/response envelope, versioning posture
- Shared TypeScript contracts and their relationship to Go types
- The Go ↔ TypeScript boundary and its generation direction
- Validation strategy (server-authoritative; Zod at the client edge)
- Pagination conventions
- Identifier conventions
- Timestamp conventions
- Money representation
- Common primitives
- API client conventions (`@platform/api-client`)

**Do not implement business domains in this phase.**

**Cross-cutting increments:**

- **CAP-5 — event envelope conventions only** (`150`): `event_id`, `event_name`, `actor_id`,
  `timestamp`, `source`. Nearly free now and expensive to retrofit, because retrofitting means
  revisiting every emission site in the platform. **No collection, no stream, no storage.**
- **CAP-6** — API client conventions (`@platform/api-client`), already in scope above.

### Authoritative documentation

`12-technical-blueprint` · `14-api-specification` · `19-payment-wallet-settlement`
(money examples) · `23-repository-monorepo-setup` · `25-backend-go-architecture`
Existing code that already encodes contracts: `services/api/pkg/httpx/errors.go`,
`packages/types/src/errors.ts`, `packages/validation`.

### Skills

`api-contracts` · `architecture-decision` · `system-architecture` · `implementation-task`

### Blockers to resolve in this phase

- **B-2 — Go ↔ TypeScript type strategy.** Status OPEN, **due immediately**. The error
  taxonomy already exists in three hand-maintained copies (Go, TS types, Zod) with no domain
  payloads yet. Choose the source of truth and the generation direction, record an ADR, and
  migrate the existing three copies to it in the same phase. Leaving them duplicated after
  choosing is the failure mode this phase exists to prevent.

### Business decisions

| Item | Class | Handling |
|---|---|---|
| **BD-07 — rounding and currency precision** | TECHNICAL_DEFAULT | Adopt the register's recommended default **explicitly, with an ADR**: money stored as integer minor units, single currency (PKR), never floating point; round once at the final customer-facing amount, half-up; ledger entries are exact integers that sum without loss. Confirm with the owner before Phase 11 builds the ledger. Pulled forward from Phase 3 of the old numbering — earlier is safe, this is the last point before any code touches an amount. |

### Acceptance

- One authoritative error contract; the other copies are generated from it or deleted
- Documented API contract strategy, recorded as an ADR
- Money representation locked and recorded
- Shared primitives available from `@platform/*` and consumed by at least one caller
- Backend ↔ frontend contract strategy established and demonstrated end to end

### Verification

Level 3 for contract code; **Level 5** for the money representation primitive and for the
error-mapping path (`verification-lite`: money is Level 5 regardless of diff size). Go package
tests plus the TypeScript packages that import the changed contracts. No E2E. No full build.

---

## Phase 3 — Identity, Authentication and Authorization

**Depends on:** Phase 2
**Objective:** the identity foundation every later domain authenticates and authorizes against.

### Scope

Users · accounts · sessions · authentication · authorization · roles · permissions ·
provider identity · merchant identity · customer identity · admin identity · token and
session lifecycle.

Use **only** the documented identity model. Do **not** implement complete provider or merchant
workflows here — those are Phases 5 and 10. This phase establishes who someone is and what
they may do, not what they do.

**Cross-cutting increments (mandatory):**

- **CAP-4 — messaging boundary.** `020` makes phone OTP the initial authentication method and
  `028` requires the OTP provider to sit **behind an interface**. A provider-independent
  messaging capability, SMS/OTP path only, is therefore part of this phase and not deferrable.
  No templates, no preferences, no push — those are Phase 12.
- **CAP-3 — device and session trust** (`116`), inside the session lifecycle already in scope.

### Authoritative documentation

`20-auth-security` · `28-identity-auth-implementation` · `13-database-schema`
· `14-api-specification` · `123-sms-email-and-otp` (CAP-4) · `116-device-and-session-trust` (CAP-3)
Topic index only (Tier C): `205`–`208`, `525`–`528`.

### Skills

`domain-modeling` · `api-contracts` · `database-architecture` · `implementation-task`

### Business decisions

None blocking. `BUSINESS_DECISION_REGISTER.md` lists no item against authentication.

### Acceptance

- Registration, login, session issue/refresh/revoke work end to end against the real database
- **OTP request and verification work through a provider interface**, with no plaintext OTP
  stored and a short expiry, per `028`
- Role and permission model enforced server-side; no client-side authority anywhere
- Every identity type resolvable and distinguishable
- Token and session lifecycle including expiry and revocation is tested, not assumed

### Verification

**Level 5 — mandatory.** Auth is one of the four unconditional Level 5 areas. Targeted unit
tests, API/integration tests against real Postgres and Redis, and security-focused tests
(expired token, revoked session, privilege escalation attempt, missing/forged claims).
E2E only for the critical authentication journey, and only if a Level 4 path genuinely
justifies standing up the first E2E harness — otherwise record the gap and continue.

---

## Phase 4 — Core Domain Model

**Depends on:** Phase 3
**Objective:** the foundational entities and persistence every service lifecycle specializes.

### Scope

Identifiers · lifecycle states · timestamps · relationships · domain validation · persistence ·
migrations · repository and data-access layer.

`004` models **all** operational work as a `Job` with types `RIDE`, `PARCEL`, `GROCERY`,
`CARGO`, `FREIGHT`. Build the Job core **once**. Never fork a parallel booking entity per
service — this is the single most expensive mistake available in this codebase.

Follow the documented schema. **Do not invent undocumented fields.**

### Authoritative documentation

`04-domain-architecture` · `13-database-schema` · `15-job-state-machine`
· `16-driver-vehicle-state-machine` · `26-database-implementation`
· `25-backend-go-architecture` · `09-project-structure`

### Skills

`domain-modeling` · `database-architecture` · `architecture-decision` · `implementation-task`

### Conflicts to resolve in this phase

- **C-5 — backend module list.** `009` and `025` disagree: `025` adds `tracking`, omits
  `wallet`, `ratings`, `zones`. ADR-004 is **Proposed (deferred)** and says to revisit before
  the first domain module lands — that is this phase. Resolve using the authoritative domain
  documents (`004`, plus `53-wallets-and-ledger`, `97-geofencing-and-zone-model`,
  `111-ratings-reviews-and-quality`, which give the omitted three their own Tier A basis)
  **before creating the affected modules**. Promote ADR-004 from Proposed to Accepted with the
  resolution, and close C-5 in `DOCUMENT_CONFLICTS.md`.

### Business decisions

| Item | Class | Handling |
|---|---|---|
| **BD-14 — required documents per vehicle type** | PRODUCT_DECISION | Structural only here: the document model and expiry mechanism are type-agnostic and can be built. The regulatory list is not needed until Phase 5 gating and not final until launch. Build configurable; do not hardcode a list. |

### Acceptance

- Documented entities exist with documented fields and no invented ones
- Migrations apply and roll back cleanly (`cmd/migrate` only; never on startup)
- Repository/data-access layer covers every entity created
- State transitions match `015` and `016` and are enforced in one place
- Module layout matches the C-5 resolution

### Verification

Level 3 for entity and repository work; **Level 5** where a transition or invariant is
concurrency-sensitive. Go package tests plus `-tags=integration` against real Postgres.
Migration up → down → up verified by observed output. No full repository verification.

---

## Phase 5 — Providers, Vehicles and Service Eligibility

**Depends on:** Phase 4
**Objective:** the supply side — who can perform which work, in which vehicle.

### Scope

Provider profile · provider status · vehicle registration · vehicle types · vehicle
capabilities · service eligibility · documents · verification state · suspension and
activation · service-area eligibility where documented.

Vehicle categories per the documentation: motorcycle · rickshaw · car · loader · van ·
pickup · truck.

**Eligibility must have exactly one shared implementation.** Do not duplicate eligibility
rules between dispatch and acceptance — Phase 8 consumes this, it does not reimplement it.
A second copy of the eligibility rules is a defect, not an optimization.

### Authoritative documentation

`16-driver-vehicle-state-machine` · `29-driver-onboarding-verification`
· `30-vehicle-capability-implementation` · `41-vehicle-capability-matching`
· `80-cargo-and-vehicle-capacity` · `81-loader-rickshaw-and-truck-services`
· `108-driver-identity-and-vehicle-verification`

### Skills

`provider-lifecycle` · `vehicle-service-eligibility` · `domain-modeling` · `implementation-task`

### Business decisions

| Item | Class | Handling |
|---|---|---|
| **BD-14 — required documents per vehicle type; suspension criteria and appeals** | PRODUCT_DECISION | Build the configurable document-requirement model and the expiry mechanism. `016` states a driver cannot accept jobs with expired required documents — implement that gate against configuration. The per-type list is regulatory and remains **open**; it blocks go-live, not this phase. Suspension criteria and the appeal path are also open — implement suspend/activate as states, do not invent the criteria. |

### Acceptance

- Provider and vehicle lifecycles match `016` and are enforced server-side
- One eligibility implementation, importable by dispatch and by acceptance
- Document model with expiry, driving the acceptance gate through configuration
- Every documented vehicle category representable with its capabilities

### Verification

Level 3 for domain work, escalating to Level 4 for the eligibility path once dispatch and
acceptance both consume it. Domain tests, API tests, targeted integration tests. No E2E.

---

## Phase 6 — Location and Realtime Foundation

**Depends on:** Phase 5
**Objective:** the location and realtime substrate dispatch and tracking require.

### Scope

Location ingestion · provider location · location freshness · realtime infrastructure ·
subscriptions · presence · online/offline state · tracking lifecycle · background location
interfaces where required · server-side location processing.

Coordinate the three skills — they describe one system from three angles and must agree:
`location-tracking`, `mobile-location`, `realtime-architecture`.

**Do not build advanced dispatch here.** This phase delivers fresh, trustworthy provider
locations and a working realtime channel; Phase 8 decides who gets the job.

**Cross-cutting increments:**

- **CAP-2 — the maps/routing/ETA boundary is created here.** `095` already specifies the
  interface: `route()`, `routeMatrix()`, `estimateETA()`. Build that abstraction plus coordinate
  and address representation (`093`, `094`) and distance calculation, with **one** provider
  behind it. PostGIS is already available from Phase 1. Geofencing and the zone model (`097`)
  only if Phase 5 service-area eligibility actually requires them. This is an interface and one
  provider — **not a mapping platform**.
- **CAP-4** — realtime notification transport reuses this phase's realtime channel (`126`).
  Never stand up a second channel.

### Authoritative documentation

`18-realtime-location-architecture` · `31-driver-availability-location`
· `47-realtime-websocket-architecture` · `48-driver-location-pipeline`
· `98-driver-location-tracking` · `99-react-native-native-location-strategy`
· `102-location-privacy-and-security` · `103-location-events-and-realtime`
CAP-2: `93-maps-and-location-architecture` · `94-geocoding-and-address-model`
· `95-routing-provider-abstraction`

### Skills

`location-tracking` · `mobile-location` · `realtime-architecture` · `event-driven-architecture`

### Business decisions

| Item | Class | Handling |
|---|---|---|
| **BD-17 — location tracking frequency per job state** | TECHNICAL_DEFAULT | Adopt the shape now: vary by driver state — lowest offline, low when available and stationary, highest on an active trip. Set the **actual values from real-device battery measurement** in Phase 12 (`mobile-performance`), not from guesswork. Record the measured result. |
| **BD-15 — location retention periods** | PRODUCT_DECISION | **Open.** Does not block ingestion. Build the schema and a retention job whose period is configuration with no silent default. Must be set before production location data accumulates — flagged for Phase 14/15. |

### Acceptance

- Location ingestion at documented frequency with server-side freshness evaluation
- Realtime subscriptions, presence and online/offline state working over the documented channel
- Tracking lifecycle start/stop tied to job state
- Retention mechanism present, period configurable, no silent default
- **CAP-2:** routing abstraction exists with `route`/`routeMatrix`/`estimateETA` and one working
  provider; no caller depends on a provider type

### Verification

**Level 5** — location privacy is an unconditional Level 5 area, and the realtime path is
concurrency-sensitive. Verification must specifically cover: **stale location**, **reconnect**,
**duplicate events**, **event ordering**, and **basic realtime delivery**. Avoid large-scale
E2E — these are integration-level concerns and are cheaper and more reliable to test there.

---

## Phase 7 — Ride Booking

**Depends on:** Phase 6
**Objective:** the core ride lifecycle end to end, as the reference implementation every other
service lifecycle follows.

### Scope

Ride request · pickup and dropoff · ride states · fare calculation foundation · booking ·
cancellation hooks · provider assignment **interface** · trip lifecycle · completion.

**Do not implement complex dispatch optimization** — Phase 8 owns it. This phase defines the
assignment interface and calls it; the implementation behind it arrives next phase.

**Do not invent cancellation fees or surge rules.** Both remain unresolved. They must remain
explicitly configurable and deferred, with an unset state that fails loudly rather than
defaulting silently to a number nobody chose.

**Cross-cutting increments:**

- **CAP-1 — the pricing boundary is created here, and it is service-parameterized from the
  first line.** `005` gives four fare formulas that all share `base + distance`; ride is merely
  the first. Implement the fare engine with a rule set per service type behind one boundary,
  and implement **only** the ride rule set now. A ride-specific fare calculator that Phase 9
  must later generalize is the failure mode this phase exists to prevent. **Pricing must never
  be reimplemented inside the delivery, cargo or grocery modules.**
- **CAP-2** — route and ETA supply the `distance` and `time` fare terms and the trip ETA
  (`096`, `100`). Consume the Phase 6 abstraction; never call a provider directly.
- **CAP-3** — safety event and audit log started (`115`); trips generate its events. Trip safety
  and SOS surfaces (`109`, `110`) where `021` places them in the ride journey.
- **CAP-4** — ride status notifications through the Phase 3 boundary.

### Authoritative documentation

`15-job-state-machine` · `33-job-domain-and-types`
· `35-booking-and-job-lifecycle-api` · `36-job-status-state-and-cancellation-rules`
· `34-quote-pricing-engine` · `05-dispatch-pricing` (fare formula)

### Skills

`ride-lifecycle` · `domain-modeling` · `api-contracts` · `implementation-task`

### Business decisions

| Item | Class | Handling |
|---|---|---|
| **BD-01 — cancellation fee amounts and thresholds** | PRODUCT_DECISION | **Open — blocks cancellation only.** Implement the tier *structure* from `005` (before driver movement / after meaningful travel / after arrival) as configuration. Amounts are commercial and are the owner's. "Meaningful travel" is also undefined — do not define it. The rest of the ride slice proceeds. |
| **BD-02 — surge / demand multiplier** | PRODUCT_DECISION | Register recommendation: **ship without surge.** Implement the `005` fare formula with the `demand` term present but inert at `1.0`. Do not invent a multiplier or a cap. |
| **BD-06 — refund policy** | PRODUCT_DECISION | Not needed in this phase beyond leaving the hook. Mechanism is Phase 11; automated policy stays open. |
| **BD-07** | resolved Phase 2 | Every fare amount uses the locked integer-minor-unit representation. No floating point. |

### Acceptance

- Ride request → quote → booking → trip → completion works end to end
- Ride states match `015`; transitions enforced in one place
- Fare calculation implements the `005` formula with unresolved terms inert and visible
- **The fare engine accepts a service-type rule set**; adding parcel in Phase 9 requires a new
  rule set, not a new engine
- Cancellation hooks exist with the tier structure configurable and unset by default
- Provider assignment is an interface with a documented contract, ready for Phase 8

### Verification

**Level 5** — money is touched. Unit tests on the fare formula including the inert terms,
integration tests across the booking path, state-machine transition tests including invalid
transitions. E2E deferred to Phase 8, where the ride journey is actually complete.

---

## Phase 8 — Dispatch Engine

**Depends on:** Phase 7
**Objective:** assign the right provider to the right job, safely, under concurrency.

### Scope

Candidate discovery · eligibility filtering (consuming Phase 5, not reimplementing it) ·
ranking · dispatch offers · acceptance and rejection · timeout · reassignment · concurrency
control · idempotency · stale providers · race-condition protection.

**Cross-cutting increments:**

- **CAP-2 — the route matrix (`Drivers × Pickup`, `096`).** Three of the nine `005` scoring
  terms — `eta_score`, `route_compatibility`, `empty_km` — are routing outputs. Without the
  matrix they are unimplementable and dispatch scoring is a stub. Combine with PostGIS/Redis
  geo (`042`).
- **CAP-3 — ratings minimum (`111`).** `driver_reliability` is a fourth scoring term. A minimum
  viable rating capture and aggregation must exist or the term is dead weight. Minimum only —
  reviews, disputes and quality workflows are Phase 13.
- **CAP-4** — dispatch offer notifications: push, and the most latency-sensitive path in the
  platform (`122`).
- **CAP-1** — dispatch *consumes* the quote for `price_fit`. **No pricing logic in dispatch.**

### Critical requirement

**Dispatch must be safe under concurrent requests. Two providers must never successfully claim
the same job.** This is the single hardest correctness requirement in the platform and the one
place where "it looked right in review" is not acceptable evidence. `046` exists for this.

### Authoritative documentation

`38-dispatch-engine-architecture` · `39-driver-candidate-discovery`
· `40-driver-matching-scoring-algorithm` · `42-postgis-redis-geo-dispatch`
· `43-driver-offer-reservation-system` · `44-dispatch-timeout-retry-strategy`
· `45-reassignment-failure-handling` · `46-concurrent-assignment-race-conditions`
· `49-nats-dispatch-events` · `05-dispatch-pricing` (scoring formula)
CAP-2: `96-route-matrix-and-eta` · CAP-3: `111-ratings-reviews-and-quality`

### Skills

`dispatch-engine` · `vehicle-service-eligibility` · `location-tracking`
· `realtime-architecture` · `event-driven-architecture`

### Business decisions

| Item | Class | Handling |
|---|---|---|
| **BD-04 — behaviour when no dispatch candidate exists** | BLOCKING_LATER | **Resolve before the relevant implementation point.** `015` already has `EXPIRED` as a terminal state, so the *shape* is documented; the search window, retry cadence and customer-facing failure behaviour are not. Propose the window and cadence as configuration; the customer-facing behaviour is a product decision. **Raise before dispatch retry is built.** A job that can reach `SEARCHING` with no terminal path is an operational dead end and the phase is not complete with one. |
| **BD-03 — dispatch scoring weights** | TECHNICAL_DEFAULT | `005` explicitly authorizes starting untuned: *"Weights should be configurable and learned from real outcomes later."* Implement weights as runtime configuration; start with ETA dominant and remaining terms low but non-zero so the mechanism is exercised. Confirm starting values before go-live. |

### Acceptance

- Candidate discovery and ranking implement the `005` formula with configurable weights, and
  every term in it is either backed by a real signal or explicitly inert and documented as such
- Offers, acceptance, rejection, timeout and reassignment work per `043`–`045`
- Every job has a terminal path, including the no-candidate case (per BD-04)
- Stale providers excluded using Phase 6 freshness, not a second definition of it
- Idempotent under retry; no double assignment under concurrency

### Verification

**Level 5 — mandatory and non-negotiable.** Extensive unit tests · **concurrency tests that
actually run concurrent claims against the real reservation mechanism** · integration tests ·
targeted E2E for the critical dispatch journey. If a concurrency harness cannot be stood up,
`verification-lite` requires blocking rather than shipping unverified dispatch behaviour.
Do not run unrelated E2E.

---

## Phase 9 — Delivery and Cargo

**Depends on:** Phase 8 · **may run in parallel with Phase 10**
**Objective:** non-passenger logistics on the same Job core.

### Scope

Parcel delivery · cargo jobs · loader and truck workflows · pickup and dropoff · proof of
delivery · delivery lifecycle · cargo lifecycle · pricing hooks · provider eligibility ·
delivery states.

### Authoritative documentation

`79-parcel-delivery-architecture` · `80-cargo-and-vehicle-capacity`
· `81-loader-rickshaw-and-truck-services` · `82-multi-stop-delivery`
· `83-proof-of-delivery` · `84-delivery-failure-and-return-flow`
· `86-delivery-pricing-and-distance` · `87-waiting-loading-and-unloading`
· `88-cargo-job-safety-and-restrictions` · `90-delivery-tracking-and-customer-experience`
· `91-delivery-exceptions-and-operations`

**Cross-cutting increments:**

- **CAP-1** — parcel and cargo rule sets added **behind the Phase 7 pricing boundary** (`86`,
  `87`). A new rule set, not a new engine. Loading and waiting are recorded as events and left
  unpriced (BD-13).
- **CAP-2** — multi-stop route matrix (`Stops × Stops`, `96`) for delivery and cargo routing
  (`82`).
- **CAP-4** — delivery status notifications through the Phase 3 boundary.

### Skills

`delivery-lifecycle` · `cargo-lifecycle` · `vehicle-service-eligibility` · `implementation-task`

### Business decisions

| Item | Class | Handling |
|---|---|---|
| **BD-10 — failed delivery: financial consequence and return-leg pricing** | PRODUCT_DECISION | Implement the failure **states** and the return **flow** from `084`. Leave the financial leg unwired. Build states now, money later. |
| **BD-16 — proof-of-delivery photo and signature retention** | PRODUCT_DECISION | **Open.** Build proof storage with a configurable lifecycle; set the period alongside BD-15 as one retention decision before launch. |
| **BD-13 — cargo waiting/loading rates; restricted-goods list; damage liability** | PRODUCT_DECISION | **Partial block on cargo.** Record loading and waiting as timestamped events now — needed regardless of pricing. Do **not** price them. **Do not ship cargo without a restricted-goods list**; the list is legal input and eligibility filtering depends on it. Damage liability is contractual and stays open. Parcel delivery is unaffected and proceeds. |

### Acceptance

- Parcel and cargo lifecycles on the shared Job core; no forked booking entity
- Proof of delivery captured and stored per `083`
- Failure and return flow states per `084`, with no invented financial treatment
- Loading and waiting recorded as events, unpriced
- Eligibility reuses the Phase 5 implementation

### Verification

Level 4 for the delivery and cargo workflows (they cross booking → dispatch → execution);
Level 5 for anything touching the eligibility gate or a money field. Integration tests per
module on the path. E2E only for the critical delivery journey.

---

## Phase 10 — Grocery and Merchant Platform

**Depends on:** Phase 8 · **may run in parallel with Phase 9**
**Objective:** the merchant supply side and the grocery order lifecycle.

### Scope

Merchant · catalog · products · inventory interfaces · cart · grocery order · merchant
acceptance · preparation · substitutions · delivery · order lifecycle.

### Authoritative documentation

`65-merchant-platform-architecture` · `66-merchant-onboarding-verification`
· `67-merchant-store-and-operating-hours` · `68-product-catalog-and-options`
· `69-inventory-and-availability-model` · `70-grocery-order-lifecycle`
· `71-grocery-cart-checkout` · `72-merchant-order-management`
· `73-merchant-driver-pickup-flow` · `74-grocery-substitution-and-item-issues`

**Cross-cutting increments:**

- **CAP-1** — grocery and delivery-fee rule sets **behind the Phase 7 pricing boundary**.
  Substitution pricing stays unwired (BD-11).
- **CAP-4** — merchant and customer order-status notifications.

### Skills

`grocery-lifecycle` · `merchant-lifecycle` · `domain-modeling` · `implementation-task`

### Business decisions

| Item | Class | Handling |
|---|---|---|
| **BD-12 — merchant order acceptance timeout** | BLOCKING_LATER | **Resolve before implementing the affected workflow.** Build acceptance with the timeout as configuration and an **explicit unset state that fails loudly** rather than defaulting silently. The value and the resulting auto-cancellation behaviour come from the owner. Grocery does not ship without it. |
| **BD-11 — grocery substitution: who absorbs the price difference** | PRODUCT_DECISION | **Open.** Build the offer / accept / decline **flow** from `074`. **Do not invent substitution pricing behaviour** — the financial adjustment stays unwired until the rule exists. |

### Acceptance

- Merchant onboarding, store, catalog and inventory interfaces per `065`–`069`
- Cart and checkout per `071`
- Grocery order lifecycle per `070`, with acceptance timeout configurable and unset by default
- Substitution flow present, pricing consequence absent and explicitly marked so
- Grocery delivery jobs flow through the shared Job core and the Phase 8 dispatch engine

### Verification

Level 4 for the order lifecycle; Level 5 where checkout touches an amount. Integration tests
across merchant → order → dispatch → delivery. Targeted E2E for the critical grocery journey
only.

---

## Phase 11 — Payments and Financial System

**Depends on:** Phases 9 and 10
**Objective:** the financial infrastructure the whole platform settles through.

### Scope

Payment intents · payment state · payment provider abstraction · webhook processing ·
idempotency · refunds · financial ledger · provider earnings · commissions · merchant
settlement · reconciliation · auditability.

**This phase strictly follows the money representation locked in Phase 2.**
**No floating-point financial calculations. Anywhere.**

### Authoritative documentation

`19-payment-wallet-settlement` · `51-payment-architecture`
· `52-payment-intents-and-customer-checkout` · `53-wallets-and-ledger`
· `54-driver-earnings-and-commission` · `55-driver-payouts`
· `56-merchant-settlement-and-cod` · `57-refunds-chargebacks-and-disputes`
· `58-payment-webhooks-and-reconciliation` · `59-financial-security-and-idempotency`
· `61-payment-testing-strategy` · `63-financial-data-model`

**Cross-cutting increments:**

- **CAP-3 — the fraud and risk engine (`112`) becomes buildable here, and not before.** Its
  documented signals are repeated failed payments, promotion abuse, abnormal cancellation
  patterns and device/account relationships — **none of which exist until this phase**. Building
  it earlier would mean building it against no data.
- **CAP-1** — commission and promotion act on the **ledger**, not on the fare engine.
- **CAP-4** — payment event notifications.

### Skills

`payment-flow` · `financial-ledger` · `settlement-reconciliation` · `event-driven-architecture`

### Business decisions

| Item | Class | Handling |
|---|---|---|
| **BD-05 — commission rates, payout schedule, minimum payout threshold** | PRODUCT_DECISION | Build commission as **configuration from day one**. Ledger structure can be built and tested with configured rates. **No real payout may run** until the owner supplies values. |
| **BD-06 — refund policy: window, partial rules, fee absorption** | PRODUCT_DECISION | Build **manual admin-initiated refunds with explicit amounts** first. Automate once the policy exists. Do not infer who absorbs the provider fee. |
| **BD-08 — reconciliation discrepancy tolerance** | TECHNICAL_DEFAULT | Adopt **zero tolerance**: any discrepancy raises an alert and is investigated; **nothing auto-adjusts**. Escalation routing is operational and can be set later. Record the adoption. |
| **BD-09 — COD liability between driver, merchant and platform** | PRODUCT_DECISION | **Blocks COD settlement.** Card and digital payment paths proceed. Do not allocate the loss — it is a legal allocation, not an engineering one. |
| **BD-13 / BD-10 / BD-11** | PRODUCT_DECISION | Their financial legs remain unwired from Phases 9 and 10. Do not close them here by inventing values. |

### Acceptance

- Payment intents and state machine per `052`; provider abstraction with no vendor leakage
- Webhook processing idempotent and replay-safe per `058`, `059`
- Double-entry ledger whose invariants hold and are asserted by tests
- Earnings, commission (configured), merchant settlement and reconciliation implemented
- Every financial mutation auditable
- Zero floating-point arithmetic on money — verified, not assumed

### Verification

**Level 5 — mandatory, and the highest bar in the project.** Required coverage:
**duplicate webhook** · **retry** · **partial failure** · **refund** · **settlement** ·
**reconciliation** · **ledger invariants**. Payment-related changes receive higher verification
levels than their diff size suggests — a one-line change to fee arithmetic is Level 5.
If a payment area cannot be tested (no local sandbox), **block rather than ship unverified
financial behaviour**.

---

## Phase 12 — Mobile Production Features

**Depends on:** Phase 11 · **may run in parallel with Phase 13**
**Objective:** **consolidate and productionize** the mobile functionality that the vertical
slices already built.

> **This is not "build the mobile apps".** Under R-5, every product workflow is built inside the
> slice that owns it — ride screens in Phase 7, dispatch-offer screens in Phase 8, delivery in
> Phase 9, and so on. By the time this phase begins, both applications already work. This phase
> finishes them: it closes gaps, hardens, measures, and completes the shared client platform
> (CAP-6). **Nothing here is a reason to postpone client work during the slices.**

### Scope

**Customer mobile:** registration and login · booking · tracking · orders · ride lifecycle ·
delivery lifecycle · notifications · profile.

**Driver/provider mobile:** onboarding · vehicle · availability · location · dispatch offers ·
trip workflow · delivery workflow · earnings.

**Native code only where genuinely necessary** (`native-module-boundary`). Background location
is the documented case; most things are not.

**Cross-cutting increments:**

- **CAP-6** — completion of shared React Native infrastructure, design-system primitives,
  navigation foundations and `@platform/api-client`. Shared primitives are extracted **when a
  second consumer appears**, not in anticipation of one.
- **CAP-4** — push registration, notification preferences (`124`), templates and localization
  (`125`). **Chat (`127`–`129`) lands here** — it blocks nothing in Phases 3–11 and may move
  later without consequence.
- **CAP-2** — map display and navigation handoff (`100`, `101`).

**Do not create separate iOS and Android screens** unless platform behaviour actually requires
it. Platform divergence is a cost paid forever.

### Authoritative documentation

`17-native-mobile-architecture` · `21-screen-map`
· `99-react-native-native-location-strategy` · `179-react-native-and-react-testing`
· `182-mobile-performance-and-battery` · `184-offline-network-resilience`

### Skills

`react-native-platform` · `native-module-boundary` · `mobile-location`
· `mobile-offline-sync` · `mobile-performance`

### Business decisions

| Item | Class | Handling |
|---|---|---|
| **BD-17 — location tracking frequency per job state** | TECHNICAL_DEFAULT | **Settle here.** Measure on a representative low-end Android device and set the per-state frequencies from the measurement. Record the result; do not carry the placeholder forward. |
| **BD-18 — offline queue expiry; which mutations may be queued** | TECHNICAL_DEFAULT | Adopt an **allow-list, not a deny-list**: a mutation is queueable only if explicitly marked so. **Financial mutations and job acceptance are never optimistically confirmed.** Queued items carry an action-appropriate expiry; expired items surface to the user rather than replaying. Record the adoption. |
| **BD-19 — mobile performance budgets** | TECHNICAL_DEFAULT | Set budgets **empirically from the first working build** on a representative low-end Android device, then hold the line. Do not adopt numbers from elsewhere. |

### Acceptance

- Both applications complete the documented journeys against the real API
- Background location working per `099` with the measured frequencies
- Offline behaviour per the BD-18 allow-list; no optimistic financial or acceptance confirms
- Performance budgets recorded and met
- Native modules limited to cases with a documented justification

### Verification

Level 3–4 per feature; **Level 5** for anything touching location, earnings or job acceptance.
`jest-expo` tests per ADR-006. Real-device measurement for BD-17 and BD-19 — device output
observed, not inferred. Mobile E2E only for the critical journeys.

---

## Phase 13 — Operational Dashboards

**Depends on:** Phase 11 · **may run in parallel with Phase 12**
**Objective:** **consolidate and complete** the operational dashboard functionality developed
alongside the domain slices.

> As with Phase 12, this is not first contact with the dashboards. Operational surfaces are
> built in the slice that produces the data they operate on. This phase completes them and adds
> what only makes sense once every domain exists — cross-domain consoles, audit, and the
> capability surfaces below.

### Scope

**Admin dashboard:** users · providers · vehicles · rides · deliveries · grocery · cargo ·
dispatch · payments · settlements · operational monitoring · audit information.

**Merchant dashboard:** merchant profile · catalog · orders · acceptance · preparation ·
inventory-related workflows · settlements. (Additionally requires Phase 10.)

Use the existing frontend architecture. **Do not convert the dashboards to Next.js** —
ADR-002 scopes Next.js to `marketing-web` only, and `012` locks it.

### Authoritative documentation

`77-merchant-dashboard-react` · `135-admin-operations-architecture`
· `136-admin-roles-and-permissions` · `137-admin-user-driver-management`
· `138-admin-vehicle-and-fleet-management` · `139-admin-merchant-management`
· `140-admin-order-job-operations` · `141-admin-dispatch-console`
· `142-admin-pricing-configuration` · `143-admin-service-zones-and-rules`
· `144-admin-configuration-and-feature-flags` · `145-admin-approvals-and-review-queues`
· `146-admin-audit-and-change-management` · `147-admin-react-dashboard-architecture`

### Skills

`system-architecture` · `api-contracts` · `implementation-task`

### Business decisions

**Cross-cutting increments:** CAP-1 admin pricing configuration (`142`) · CAP-2 dashboard and
dispatch-console maps (`141`) · CAP-3 safety operations dashboard (`119`), enforcement (`113`),
incident management (`114`) · CAP-4 support ticketing, routing and operational actions (`130`,
`131`, `132`) · CAP-5 operational metrics and dashboards (`156`, `159`) — the first phase where
analytics has a real consumer · CAP-6 consolidation.

None new. The dashboards **surface** the unresolved values (commission rates, cancellation
tiers, timeouts) as configuration screens — they must not hardcode a value the register leaves
open, and an unset value must display as unset.

### Acceptance

- Admin dashboard covers the documented operational surfaces against the real API
- Merchant dashboard covers the documented merchant workflows
- Role and permission enforcement is server-side; the UI reflects it, it does not implement it
- Configuration screens expose unresolved business values as unset rather than defaulted
- Still React + Vite; no Next.js introduced

### Verification

Level 2–3 per surface; Level 4 where a console drives a cross-module workflow (dispatch
console, finance console). Vitest per package plus typecheck on importers. Playwright E2E
limited to the critical admin journeys.

---

## Phase 14 — Production Infrastructure and Observability

**Depends on:** Phase 13
**Objective:** make the working system deployable and observable.

**Do not build production infrastructure prematurely during domain development.** This phase
exists at position 14 deliberately: infrastructure built before the code it serves is
speculative infrastructure, which `IMPLEMENTATION_EXECUTION_POLICY.md` §J forbids.

### Scope (where documented)

Deployment · cloud infrastructure · secrets · scaling · queues · caching · monitoring ·
tracing · logging · metrics · alerts · backups · database operations · object storage ·
service health · production configuration.

### Authoritative documentation

`163-production-infrastructure-architecture` · `164-aws-networking-and-security`
· `165-docker-and-container-strategy` · `166-ecs-service-and-scaling-strategy`
· `167-postgresql-postgis-production` · `168-redis-cache-and-realtime-state`
· `169-events-queues-and-background-workers` · `170-cicd-github-actions`
· `171-secrets-config-and-environments` · `172-observability-logging-metrics-tracing`
· `173-monitoring-alerting-and-slos` · `174-backups-disaster-recovery-and-business-continuity`
· `175-cost-capacity-and-performance-infrastructure`

**Cross-cutting increments:** CAP-5 event collection, stream and warehouse storage (`149`,
`157`) — the pipeline, now that meaningful events exist · CAP-2 map provider cost controls and
fallback (`104`), caching (`101`) · CAP-3 safety data privacy and retention (`118`), settled
with BD-15/BD-16 · CAP-4 communication observability and delivery tracking (`133`).

### Skills

`system-architecture` · `event-driven-architecture` · `architecture-decision`

### Business decisions

| Item | Class | Handling |
|---|---|---|
| **BD-15 — location retention periods** | PRODUCT_DECISION | **Must be set here at the latest** — before production location data accumulates. A conservative technical shape is obvious (full resolution briefly, downsampled beyond) but the period is a privacy and legal decision. Escalate; do not adopt silently. |
| **BD-16 — proof retention** | PRODUCT_DECISION | Set alongside BD-15 as one retention policy decision; implement as object-storage lifecycle rules. |

### Acceptance

- Deployable to the documented cloud target with secrets managed, not committed
- Logging, tracing, metrics and alerting operational against real signals the code emits
- Backup and restore exercised, not merely configured
- Retention policies implemented once BD-15/BD-16 are decided
- Production configuration validated by the same `pkg/config` path as local

### Verification

**Level 5** — `verification-lite` classifies infrastructure changes as Level 5 regardless of
diff size. Restore must be tested by restoring. Alerts must be tested by firing.

---

## Phase 15 — Hardening, Performance and Release

**Depends on:** Phase 14
**Objective:** the final engineering phase. **This is the only phase where broader regression
verification is performed.**

### Scope

Security review · performance review · database optimization · caching optimization · rate
limiting · abuse prevention · reliability · concurrency review · failure recovery ·
observability review · accessibility · mobile performance · dashboard performance ·
production readiness · release verification.

### Authoritative documentation

`177-testing-strategy-and-pyramid` · `178-backend-and-api-testing`
· `179-react-native-and-react-testing` · `180-e2e-critical-user-journeys`
· `181-performance-load-and-stress-testing` · `182-mobile-performance-and-battery`
· `183-security-testing-and-hardening` · `184-offline-network-resilience`
· `185-data-consistency-and-idempotency` · `186-release-management-and-feature-flags`
· `187-production-readiness-checklist` · `188-incident-response-and-runbooks`
· `189-production-launch-plan`

**Cross-cutting increments:** CAP-3 final security review (`183`) · CAP-5 product and
marketplace analytics (`151`, `152`), governance and data quality (`161`), experimentation
(`160`) · CAP-1 confirm or explicitly exclude every open pricing value.

### Skills

`verification-lite` · `mobile-performance` · `system-architecture` · `change-impact-analysis`

### Business decisions

| Item | Class | Handling |
|---|---|---|
| **BD-14 — regulatory document list** | PRODUCT_DECISION | **Blocks go-live.** The per-vehicle-type required-document list must exist and be loaded before launch. |
| **BD-01, BD-05, BD-09, BD-11, BD-13** | PRODUCT_DECISION | Every open commercial, legal and pricing value must be supplied before launch, or the dependent capability is explicitly excluded from the launch scope. Launching with a silently defaulted value is not an option. |
| **BD-03 starting weights, BD-19 budgets** | TECHNICAL_DEFAULT | Confirm before go-live. |

### Deferred verification completed here

- **Phase 1 acceptance criterion 12 — remote CI green on a pull request.** Blocked since
  2026-08-27 by an account billing lock external to the repository. Re-attempt and close here,
  or earlier and opportunistically if billing is restored sooner.
- The full E2E suite across the documented critical journeys (`180`)
- Full build verification across every surface (`make verify`)

### Acceptance

- Unit, integration and E2E suites pass across the documented critical journeys
- Build verification passes on every surface
- Security checks per `183` complete with findings resolved or accepted in writing
- Performance checks per `181`, `182` meet the recorded budgets
- `187` production readiness checklist satisfied
- Remote CI green, or the reason it is not is external and recorded

### Verification

Full. This is the milestone `IMPLEMENTATION_EXECUTION_POLICY.md` §D, §F and §G defer to.

---

## Business Decision Handling

Governed by `docs/BUSINESS_DECISION_REGISTER.md` and
`IMPLEMENTATION_EXECUTION_POLICY.md` §I.

**Do not block early phases because of decisions required only by later phases.**

Before entering a phase:

1. Check its required business decisions (the per-phase blocks above are authoritative).
2. Already decided → proceed.
3. Technically defaulted → follow the documented default, **adopt it explicitly and record it**.
   A default adopted silently is an invented rule.
4. A product decision is genuinely required → **stop only that phase**, record the blocker in
   `BLOCKED_TASKS.md`, and continue with independent work where safe.
5. Never invent a business rule. Never guess a commercial, legal or financial value.

### Decision map by phase

| Phase | Blocking now | Adopt as default | Open, non-blocking |
|---|---|---|---|
| 2 | — | BD-07 | — |
| 3 | — | — | — |
| 4 | — | — | BD-14 (structural only) |
| 5 | — | — | BD-14 (list; blocks go-live) |
| 6 | — | BD-17 (shape) | BD-15 |
| 7 | BD-01 (cancellation only) | BD-02 (surge inert) | BD-06 |
| 8 | **BD-04** | BD-03 | — |
| 9 | BD-13 (cargo restricted-goods list) | — | BD-10, BD-16 |
| 10 | **BD-12** | — | BD-11 |
| 11 | BD-09 (COD only) | BD-08 | BD-05, BD-06 |
| 12 | — | BD-17 (values), BD-18, BD-19 | — |
| 13 | — | — | — |
| 14 | BD-15, BD-16 | — | — |
| 15 | BD-14, and every remaining open item | BD-03, BD-19 confirmation | — |

---

## Phase Completion Rule

A phase is complete **only** when:

- the documented scope is implemented
- affected tests pass
- the relevant build passes
- `docs/IMPLEMENTATION_STATUS.md` is updated
- known blockers are recorded
- no unresolved critical defect remains

A phase does **not** require:

- full E2E
- full repository verification
- full CI
- re-reading all documentation

unless the phase explicitly requires it. Phase 15 is the only phase that requires all four.

`IMPLEMENTED` ≠ `VERIFIED`. `VERIFIED` means a command was run and its output observed.

---

## Phase Execution Record

Maintain one block per phase. **Keep it concise** — this is a record, not a report.

```text
Phase N — <name>
  Status:            NOT_STARTED | IN_PROGRESS | BLOCKED | IMPLEMENTED | VERIFIED
  Started:           YYYY-MM-DD
  Completed:         YYYY-MM-DD
  Summary:           2–4 lines. What now exists that did not before.
  Documents used:    the 2–5 actually read
  Skills used:       the skills actually invoked
  Verification:      levels assigned, commands run, what was observed
  Blockers:          B-n / BD-n, or none
  ADRs:              recorded this phase, or none
  Next phase:        N+1, or the parallel branch taken
```

### Live record

> This table has drifted twice, both times because edits used non-asserting string
> replacements that silently matched nothing. Rows 4–11 were corrected on 2026-08-28, and the
> blocker column on 2026-08-29 after the business decisions were resolved. Every edit since
> asserts its match. `IMPLEMENTATION_STATUS.md` has been correct throughout and remains the
> authority where the two disagree.

| Phase | Status | Started | Completed | Blockers |
|---|---|---|---|---|
| 1 — Engineering foundation | **VERIFIED** | 2026-08-27 | 2026-08-27 | criterion 12 (remote CI) deferred to Phase 15 |
| 2 — Contracts and core platform | **VERIFIED** | 2026-08-28 | 2026-08-28 | none — B-2 closed (ADR-007), BD-07 implemented (ADR-008) |
| 3 — Identity and auth | **VERIFIED** | 2026-08-28 | 2026-08-28 | none |
| 4 — Core domain model | **VERIFIED** | 2026-08-28 | 2026-08-28 | none — C-5 resolved, ADR-004 accepted |
| 5 — Providers, vehicles, eligibility | **VERIFIED** | 2026-08-28 | 2026-08-28 | BD-14 structural only; requirements table ships empty |
| 6 — Location and realtime | **VERIFIED** | 2026-08-28 | 2026-08-28 | BD-15/BD-17 mechanisms built, values unset |
| 7 — Ride booking | **VERIFIED** | 2026-08-28 | 2026-08-29 | none — BD-01 and BD-02 resolved 2026-08-28 and configured |
| 8 — Dispatch engine | **VERIFIED** | 2026-08-28 | 2026-08-29 | none — BD-04 resolved 2026-08-28, B-5 closed; sweeper enforces the deadline |
| 9 — Delivery and cargo | **VERIFIED** | 2026-08-28 | 2026-08-28 | BD-10/BD-13/BD-16 mechanisms built, values unset |
| 10 — Grocery and merchant | **VERIFIED** | 2026-08-28 | 2026-08-29 | none — BD-11 and BD-12 resolved 2026-08-28; sweeper auto-cancels unanswered orders |
| 11 — Payments and financial | **VERIFIED** | 2026-08-28 | 2026-08-29 | BD-05 resolved (flat 20%); BD-06/BD-09 open but block nothing built |
| 12 — Mobile production | **PARTIAL** | 2026-08-28 | — | customer booking and driver trip flows built and tested; **no map, no real location source, no push, no offline queue, no earnings**; BD-17/18/19 unmeasured |
| 13 — Operational dashboards | **PARTIAL** | 2026-08-28 | — | admin job list only; **merchant dashboard is an empty directory**; support and finance consoles outstanding |
| 14 — Production infrastructure | **PARTIAL** | 2026-08-28 | — | CI contract gate built; **container does not build**; cloud needs credentials |
| 15 — Hardening and release | **PARTIAL** | 2026-08-28 | — | full local verification run; load/E2E/remote CI outstanding |

Phase 1's full evidence stays in `IMPLEMENTATION_STATUS.md`; it is not duplicated here.

---

## Conflicts Discovered

Recorded, **not silently resolved** (`IMPLEMENTATION_EXECUTION_POLICY.md` §H). None blocks
Phase 2.

### R-1 — Three incompatible phase numberings

`AGENTS.md` §9, `IMPLEMENTATION_PLAN.md`, and this roadmap number phases differently, and
existing documents cross-reference the older schemes. **Handling:** the reconciliation table
above is the translation key; this roadmap governs execution order only. `AGENTS.md` §9 and the
`IMPLEMENTATION_PLAN.md` spine are superseded for ordering and remain authoritative for
everything else. **Status:** RESOLVED by translation.

### R-2 — No dedicated pricing and quote phase

`IMPLEMENTATION_PLAN.md` gave pricing/quote its own phase (old Phase 6) before dispatch. This
roadmap has none. Pricing is a real subsystem with Tier A ownership (`005`, `034`) and its own
engines (`086` delivery, `087` waiting/loading). **Risk:** pricing gets built four times, once
per service.

**Decision (owner, 2026-08-28): no standalone pricing phase. Pricing is a shared domain
capability with service-specific rule sets behind one boundary — CAP-1.**

The evidence supports it. `005` gives four fare formulas that all share `base + distance`:

```text
ride    base + distance + time + demand + vehicle adjustment
parcel  base + distance + size/weight + urgency
cargo   base + distance + vehicle + capacity + loading + waiting + schedule
grocery delivery fee + service rules
```

Four formulas, one shape. A dedicated phase would build all four before any consumer exists;
four independent implementations would drift four ways. Neither is right.

**Resolution, as implemented in CAP-1:**

- The pricing boundary is created by its **first real consumer — Phase 7 (ride)** — and is
  service-parameterized from the first line
- Service-specific rules live **behind** that boundary: ride in Phase 7, parcel and cargo in
  Phase 9, grocery in Phase 10
- **Pricing may never be reimplemented inside the ride, delivery, cargo or grocery modules.**
  A second fare calculator is a defect
- Money and rounding come from the Phase 2 financial contract (BD-07), not from pricing
- Unresolved product pricing values (BD-01, BD-02, BD-13) stay configurable and deferred, with
  an unset state that fails loudly
- Phase 7 acceptance requires that adding parcel in Phase 9 needs **a new rule set, not a new
  engine**

**Nothing is implemented now.** **Status:** RESOLVED.

### R-3 — Dispatch sequenced after ride booking

This roadmap puts booking (7) before dispatch (8); `IMPLEMENTATION_PLAN.md` puts dispatch
before the ride slice. Workable because Phase 7 scope explicitly defines only a *provider
assignment interface*. **Consequence:** the ride vertical slice is not actually complete until
Phase 8, so the ride critical-journey E2E belongs to Phase 8, not Phase 7. Reflected above.
**Status:** RESOLVED, with the E2E moved.

### R-4 — Location and realtime moved ahead of dispatch

This roadmap sequences location/realtime (6) before dispatch (8); `IMPLEMENTATION_PLAN.md` had
it after. The new order is the correct one — dispatch candidate discovery depends on location
freshness (`042`, `039`) and would otherwise be built against a stub. **Status:** RESOLVED in
favour of this roadmap.

### R-5 — Horizontal client phases contradict the vertical slice rule

Phases 12 and 13 read as horizontal client phases. `IMPLEMENTATION_PLAN.md` states clients
should "grow alongside the backend slices rather than as a separate horizontal phase", and
`AGENTS.md` §10 is the vertical slice rule.

**Decision (owner, 2026-08-28): vertical slices remain the primary implementation strategy.
Shared client/platform infrastructure may be built horizontally when it is genuinely
reusable — CAP-6. Phases 12 and 13 are consolidation phases, never a reason to postpone
frontend work.**

**Permitted horizontally**, at the phase that first needs it: shared React Native
infrastructure · shared UI and design-system primitives · `@platform/api-client` functionality ·
navigation foundations. Shared primitives are extracted **when a second consumer appears**, not
in anticipation of one.

**Required vertically**, inside the slice that owns them: every product workflow, screen and
journey. A slice is:

```text
ride backend + ride API + ride mobile UI + ride dashboard UI + ride verification
```

and never:

```text
build the entire mobile application  →  build the entire backend  →  connect everything
```

**Resolution, as implemented:**

- Phase 12 is a **productionization and consolidation** phase for mobile functionality already
  built through the slices — it closes gaps, hardens, measures, and completes CAP-6
- Phase 13 likewise **consolidates and completes** dashboard functionality built alongside the
  domain slices, and adds only what requires every domain to exist
- Both phase objectives are rewritten above to say so explicitly, so a future agent reading
  Phase 12 in isolation cannot mistake it for "build the mobile apps"

**Status:** RESOLVED.

### R-6 — Documented domains with no phase in this roadmap

Four Tier A domains had no home in the 15 phases: maps/routing/ETA (`093`–`106`), safety/trust/
fraud (`107`–`120`), notifications/chat/support (`121`–`134`), analytics (`149`–`162`).

**Decision (owner, 2026-08-28): resolve now rather than leave them as indefinite partials, and
resolve them as cross-cutting capabilities rather than as four new phases.**

Dependency analysis placed each one. Two findings changed the roadmap materially:

1. **Notifications has a hard, previously missed minimum at Phase 3.** `020` makes phone OTP the
   initial authentication method, and `028` specifies `/auth/otp/request` and `/auth/otp/verify`
   and requires that *"OTP provider must be behind an interface."* **Authentication cannot ship
   without a messaging capability.** The roadmap had none before Phase 7. This was a genuine
   sequencing defect, not a presentational gap.
2. **Four of the nine `005` dispatch scoring terms are capability outputs.** `eta_score`,
   `route_compatibility` and `empty_km` come from routing (CAP-2); `driver_reliability` comes
   from ratings (CAP-3). Without both, Phase 8 scoring is a stub with most of its formula inert.

**Resolution — no new phases; four capability tracks inside the existing 15:**

| Domain | Track | Boundary | Complete by |
|---|---|---|---|
| Maps, routing, ETA | CAP-2 | **Phase 6** — `route`/`routeMatrix`/`estimateETA` per `095`, one provider | Phase 14 |
| Safety, trust, fraud | CAP-3 | **Phase 3** device/session trust; verification already Phase 5; **fraud engine Phase 11** | Phase 15 |
| Notifications, chat, support | CAP-4 | **Phase 3 — mandatory** (OTP); chat Phase 12; support Phase 13 | Phase 14 |
| Analytics | CAP-5 | **Phase 2** — event envelope only; pipeline Phase 14 | Phase 15 |

Each is staged in [Cross-Cutting Capabilities](#cross-cutting-capabilities) with a per-phase
increment, and summarized in the Capability Increment Matrix. The governing constraints —
no complete mapping platform in Phase 6, no fraud engine before payment data exists, no
analytics platform before meaningful events exist, chat blocking nothing in Phases 3–11 — are
recorded in the tracks themselves.

**The 15-phase structure is unchanged. No phase was renumbered.**

**Status:** RESOLVED. `BLOCKED_TASKS.md` B-4 is closed.

### R-7 — `BUSINESS_DECISION_REGISTER.md` timeline uses the old numbering

Its "Timeline" table maps items to `IMPLEMENTATION_PLAN.md` phases. **Handling:** the per-phase
blocks and the decision map in this document are the remapped authority. **Status:** RESOLVED
by remapping; the register itself is unchanged and still correct on classification, rationale
and recommendation — only its phase numbers are stale.

### R-8 — BD-07 pulled earlier than the register states

The register places BD-07 at old Phase 3 ("before any money-shaped code"); this roadmap places
it in Phase 2. Earlier is strictly safer and satisfies the register's own condition.
**Status:** RESOLVED, no conflict of substance.

### Consistency confirmed (checked, not conflicting)

- **ADR-001** (Go outside the JS workspace) — every phase treats Go and TypeScript as separate
  verification surfaces.
- **ADR-002** (React + Vite dashboards, Next.js for marketing only) — Phase 13 explicitly
  forbids a Next.js conversion.
- **ADR-003** (directory names) — all phase references use the `023` names.
- **ADR-005** (tier map) — every phase cites Tier A documents; Tier C is used as a topic index
  only.
- **ADR-006** (two test runners) — Phase 12 verification uses `jest-expo`, Phase 13 Vitest.
- **`IMPLEMENTATION_EXECUTION_POLICY.md` §D/§E/§F/§G** — no phase but 15 requires full CI, full
  E2E or a full rebuild; every phase names a proportional verification level.
- **`verification-lite`** — every unconditional Level 5 area (money, dispatch assignment, auth,
  concurrency, location privacy, infrastructure) is Level 5 in the phase that touches it.
- **B-1** (control docs `366`–`368` empty) — this roadmap is the derived spine ADR-005 and
  `BLOCKED_TASKS.md` option (c) anticipated. It can be backfilled into `366`–`368` later.

---

## Master Agent Command

> When instructed to continue implementation, identify the earliest uncompleted phase whose
> dependencies are satisfied and execute it according to `AGENTS.md` and
> `docs/IMPLEMENTATION_EXECUTION_POLICY.md`.
>
> Do not skip phases without dependency justification.
>
> Do not restart completed phases.
>
> Do not reread unrelated documentation.
>
> Do not run unnecessary verification.
>
> After completing the phase, update implementation status and continue to the next unblocked
> phase unless a genuine decision or blocker requires stopping.
