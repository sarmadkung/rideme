# Implementation Status

Reflects reality, not intent. `IMPLEMENTED` ≠ `VERIFIED` — see `progress-tracking`.
**Phases 1–11 are complete and verified.** Every backend service — ride, parcel, cargo,
grocery — runs through one job core, one dispatch engine, one pricing engine and one ledger.

**Phases 12–15 are PARTIAL.** Their foundations are built and tested; the product surfaces
they call for are not. See each phase below for exactly what exists.

All six blocking product decisions were answered by the owner on 2026-08-28 — BD-01, BD-02,
BD-04, BD-05, BD-11, BD-12 — and are implemented as configuration. See "Business Decisions
Resolved" below.

`VERIFIED` below means a command was run and its output observed, not that the
code looks right. Evidence is in the Phase 1 completion report.

**Last updated:** 2026-08-28

## Legend

`NOT_STARTED` · `READY` · `IN_PROGRESS` · `BLOCKED` · `IMPLEMENTED` · `VERIFIED` · `DEFERRED` · `OBSOLETE`

## Preparation

| Task | Status | Tests | Verified | Notes |
|------|--------|-------|----------|-------|
| AGENTS.md execution protocol | IMPLEMENTED | n/a | n/a | committed `393d793`, patched `c958f68` |
| Custom skill system (31 skills) | VERIFIED | n/a | YES | frontmatter valid; 228 doc refs resolve |
| Documentation audit | VERIFIED | n/a | YES | signature clustering; `DOCUMENT_AUDIT.md` |
| Document conflict register | IMPLEMENTED | n/a | n/a | 5 conflicts; 4 resolved, 1 deferred |
| Architecture decisions | IMPLEMENTED | n/a | n/a | ADR-001 … ADR-005 |
| Business decision register | IMPLEMENTED | n/a | n/a | 19 items classified |
| Implementation readiness | IMPLEMENTED | n/a | n/a | Phase 1 READY |
| First slice specification | IMPLEMENTED | n/a | n/a | `FIRST_IMPLEMENTATION_SLICE.md` |

## Phase 1 — Repository Foundation

Verified 2026-08-27 against the acceptance criteria in `FIRST_IMPLEMENTATION_SLICE.md`.

| Task | Status | Tests | Verified | Notes |
|------|--------|-------|----------|-------|
| pnpm workspace + Turborepo | VERIFIED | n/a | YES | pnpm 10.33, Turborepo 2.5; 10 build tasks |
| TypeScript config (strict, shared base) | VERIFIED | n/a | YES | strict + `noUncheckedIndexedAccess` + `exactOptionalPropertyTypes`; 17 typecheck tasks pass |
| ESLint + Prettier | VERIFIED | n/a | YES | flat config, re-exported per package; `format:check` clean |
| `@platform/*` package scaffolding (7) | VERIFIED | 15 | YES | no domain types; importability proved from an app by `apps/admin-dashboard/src/workspace.test.ts` |
| Go module + `cmd/api` | VERIFIED | n/a | YES | Go 1.25, own module (ADR-001); `go build ./...` clean |
| Go config loading + validation | VERIFIED | 6 | YES | loaded once at startup; reports every problem at once |
| Go structured logging + request ID | VERIFIED | 4 | YES | JSON; W3C traceparent continued, new span per request |
| Go typed error taxonomy → HTTP | VERIFIED | 6 | YES | single mapping in `pkg/httpx`; cause never serialised |
| Health checks (Postgres/Redis/NATS) | VERIFIED | 9 | YES | 200 healthy → 503 with postgres stopped → 200 on restart |
| Graceful shutdown | VERIFIED | n/a | YES | SIGTERM → in-flight drained → port released |
| Migration runner (explicit command) | VERIFIED | 1 | YES | up → down → up → version 1; never runs on startup |
| Docker Compose local infra | VERIFIED | 3 | YES | four services healthy; integration tests pass against them |
| PostGIS extension enabled | VERIFIED | 1 | YES | `postgis 3.4.3` confirmed in `pg_extension`; startup aborts without it |
| `admin-dashboard` shell (Vite) | VERIFIED | 5 | YES | builds; dev server serves on 5173 with validated env |
| `customer-mobile` shell (Expo) | VERIFIED | 5 | YES | Expo SDK 57; iOS + Android bundles exported; Metro starts |
| `driver-mobile` shell (Expo) | VERIFIED | 5 | YES | as above |
| Environment files + gitignore | VERIFIED | 3 | YES | `.env.local` ignored; public-prefix secret leak rejected at load |
| Testing harness (TS + Go) | VERIFIED | n/a | YES | Vitest, jest-expo, `go test`; integration behind a build tag |
| CI foundation (GitHub Actions) | IMPLEMENTED | n/a | **NO** | workflow parsed, registered and triggered; **execution blocked externally** — see below |
| Root README.md | VERIFIED | n/a | YES | clone-to-running path executed |
| Update `verification-lite` commands | VERIFIED | n/a | YES | real per-surface commands replace the Level-0 placeholder |
| Update `project-discovery` repo state | VERIFIED | n/a | YES | ground truth now describes the built repository |

**Not done, deliberately:** `merchant-dashboard` and `marketing-web` are reserved
directories; `services/api/internal/` is empty; no domain model, endpoint, auth,
NATS subject, WebSocket, native module, E2E harness or Terraform exists.

**Counts:** 28 Go test functions (34 cases with subtests) across 6 packages ·
36 TypeScript tests across 10 packages · 3 integration tests behind a build tag ·
0 E2E, by design.

## Phase 2 — Contracts and Core Platform Foundation

Roadmap numbering (`MASTER_IMPLEMENTATION_ROADMAP.md`). Verified 2026-08-28
against the Phase 2 acceptance criteria.

| Task | Status | Tests | Verified | Notes |
|------|--------|-------|----------|-------|
| B-2 — contract source of truth (ADR-007) | VERIFIED | 7 | YES | Go authoritative; `pkg/contract` reflects registered structs into TS + Zod |
| Contract generator `cmd/contractgen` | VERIFIED | 7 | YES | emits `packages/{types,validation}/src/generated.ts`; fails generation on an unmappable type |
| Drift gate `make contracts-check` | VERIFIED | n/a | YES | proved by injecting a Go constant without regenerating: gate failed, message names the cause; reverted |
| Three hand-kept copies removed | VERIFIED | n/a | YES | `types/src/errors.ts` and `health.ts` deleted; hand-written Zod schemas replaced by generated |
| BD-07 — money representation (ADR-008) | VERIFIED | 13 | YES | `pkg/money`; integer minor units, no float in any path |
| Money rounding — half away from zero | VERIFIED | 8 cases | YES | single rounding entry point `ApplyRate(num, den)`; rates are integers |
| Money allocation conserves every unit | VERIFIED | 2006 cases | YES | 6 table cases + 2000 randomised splits; parts always sum to the whole |
| Money JSON is deterministic and strict | VERIFIED | 7 | YES | rejects fractional, exponent, quoted, out-of-range and unknown-currency amounts |
| CAP-5 — event envelope (`150`) | VERIFIED | 9 | YES | `pkg/events`; documented fields only, UTC enforced, name shape validated |
| Pagination conventions (ADR-009) | VERIFIED | 1 | YES | cursor-based `PageInfo`; `ClampLimit` bounds 25/100 |
| API version prefix + idempotency header | VERIFIED | n/a | YES | `APIVersionPrefix`, `IdempotencyKeyHeader` from `14`/`185`; contract only, no middleware |
| Client-side money formatting | VERIFIED | 3 | YES | `formatMoney` in `@platform/types`; asserted against the Go formatter's cases |
| No client-side money arithmetic | VERIFIED | n/a | YES | deliberate — server is authoritative; recorded in ADR-008 |

**Verification evidence (all observed, 2026-08-28):**

```text
gofmt -l .                 clean
go vet ./...               ok
go test ./...              9 packages ok, 62 test functions
                           (51 run-cases across the four Phase 2 packages)
pnpm typecheck             17/17 tasks
pnpm lint                  10/10 tasks
pnpm test                  17/17 tasks, 49 tests
pnpm build                 10/10 tasks
pnpm format:check          clean
make contracts-check       in sync
```

**Verification level:** 5 for money and the contract boundary, as
`verification-lite` requires for anything touching money — mandatory regardless
of diff size. No E2E introduced. No remote CI run.

**A defect the tests caught:** the first money decoder accepted the JSON string
`"10"` as an amount, because `json.Number` is a string type underneath.
`TestUnmarshalRejectsInvalidAmounts` failed on it. Fixed by parsing the raw
literal, which rejects quoted, fractional and exponent forms in one step.

**Not done, deliberately:** no endpoint, no middleware, no domain type, no
event producer, no analytics collection. `services/api/internal/` is still
empty. Phase 2 is contracts; domains start at Phase 3.

## Phase 3 — Identity, Authentication and Authorization

Verified 2026-08-28 against the Phase 3 acceptance criteria. Documents 20, 28, 116, 123.

| Task | Status | Tests | Verified | Notes |
|------|--------|-------|----------|-------|
| Schema — users, roles, devices, sessions, OTP, audit | VERIFIED | n/a | YES | migration `000002`; up/down/up applied against local Postgres |
| One user, many roles (`28`) | VERIFIED | 2 | YES | `user_roles` table; a driver who orders groceries is one account |
| Phone normalisation to E.164 | VERIFIED | 4 | YES | 10 input forms resolve to one number; idempotent |
| OTP request → verify → resolve/create → tokens | VERIFIED | 3 | YES | full documented flow end to end against the database |
| OTP never stored in plaintext (`28`, `123`) | VERIFIED | 1 | YES | asserted against the stored bytes: 32-byte keyed HMAC, code absent |
| OTP single-use — including under concurrency | VERIFIED | 2 | YES | 8 concurrent verifications of one code → exactly 1 login |
| OTP attempt limit and expiry | VERIFIED | 2 | YES | correct code fails after the limit; expired code rejected |
| OTP purpose-bound (`123`) | VERIFIED | 1 | YES | a LOGIN code does not verify against PHONE_CHANGE |
| Account enumeration prevented (`28`) | VERIFIED | 1 | YES | known and unknown numbers return identical expiry and error code |
| Refresh rotation (`28`) | VERIFIED | 2 | YES | old token dies on rotation; 6 concurrent refreshes → exactly 1 |
| **Refresh reuse detection** | VERIFIED | 1 | YES | replaying a rotated token revokes every session; see the defect note |
| Logout and logout-all (`116`) | VERIFIED | 2 | YES | refresh stops after each |
| Session expiry | VERIFIED | 1 | YES | an expired session cannot refresh |
| Role authorization middleware | VERIFIED | 2 | YES | bearer-token forms; anonymous callers refused before the handler |
| Resource authorization (`28`) | VERIFIED | 1 | YES | another DRIVER cannot touch a driver's record — role alone would allow it |
| Roles re-read on refresh | VERIFIED | 1 | YES | a revoked role does not survive the next refresh |
| Suspended accounts cannot authenticate | VERIFIED | 1 | YES | both fresh login and existing refresh are refused |
| Rate limiting by phone and IP (`20`, `28`) | VERIFIED | 4 | YES | Redis-backed in production, in-process in tests |
| Security event audit (`28`) | VERIFIED | 2 | YES | login/logout recorded; audit stores a **masked** phone, asserted |
| CAP-4 — messaging boundary (`121`, `123`) | VERIFIED | n/a | YES | `pkg/notify`; provider replaceable, fallback on failure; API refuses to start in production without a real provider |
| CAP-3 — device and session trust (`116`) | VERIFIED | n/a | YES | device upsert, first-sighting signal, session listing |
| `rate_limited` → 429 added to the taxonomy | VERIFIED | n/a | YES | added in Go, propagated to both TypeScript artifacts by `make contracts` |

**Verification evidence (observed, 2026-08-28):**

```text
gofmt -l .                     clean
go vet ./...                   ok
go test ./...                  12 packages ok
go test -tags=integration      25 tests, ok — against real Postgres and Redis
pnpm typecheck                 17/17      pnpm lint    10/10
pnpm test                      17/17      pnpm build   10/10
pnpm format:check              clean      contracts    in sync
```

**Verification level:** 5 throughout — `verification-lite` escalates auth regardless of
diff size. The concurrency properties are tested against the real database because they are
enforced by SQL; a mocked store would assert nothing about them.

**A defect the tests caught.** Refresh-token *reuse* was not actually detected. Rotation
removed the old hash from `sessions`, so a replayed token simply missed the lookup and was
refused like any random string — the theft signal document 28 calls for was silently lost.
Fixed by recording superseded hashes in `refresh_token_history` inside the same transaction
as the rotation, so a token can never be retired without becoming detectable. A second
defect — a `u.` table alias in a column list shared with `INSERT ... RETURNING` — was caught
by the same suite.

**Not done, deliberately:** no phone-change flow (needs step-up verification, document 20),
no step-up on sensitive actions, no Google/Apple providers (`20`: "later"), no real SMS
provider, no mobile secure-storage integration (that is client work, Phase 12).

## Phase 4 — Core Domain Model

Verified 2026-08-28. Documents 04, 13, 15, 16, 25, 26.

| Task | Status | Tests | Verified | Notes |
|------|--------|-------|----------|-------|
| **C-5 resolved** — backend module list | VERIFIED | n/a | YES | `004` breaks the tie; union of `004`/`009` plus `tracking`. ADR-004 promoted to Accepted; C-5 closed |
| Migration `000003` — core entity model | VERIFIED | n/a | YES | up → down → up observed |
| Job as the universal work abstraction (`04`) | VERIFIED | 2 | YES | one `jobs` table, five types; **no per-service booking entity** |
| Job state machine matches `015` | VERIFIED | 6 | YES | documented main flow walkable; skips, reversals and resurrection refused |
| Assignment state machine | VERIFIED | 2 | YES | an answered offer cannot be answered again |
| `pkg/statemachine` engine (ADR-010) | VERIFIED | n/a | YES | table-driven; refusals name what was allowed |
| **Compare-and-set transitions** | VERIFIED | 1 | YES | 8 concurrent transitions on one job → exactly 1 winner, 2 history rows |
| **Two drivers cannot hold one job** | VERIFIED | 2 | YES | 10 concurrent offers → exactly 1 claim; partial unique index, not an application check |
| Job stops as ordered rows, PostGIS geography | VERIFIED | 3 | YES | lat/lon survive the round trip; multi-stop dropoff resolution correct |
| Atomic creation | VERIFIED | 1 | YES | a bad stop leaves no orphan job |
| Status history (`015`) | VERIFIED | 2 | YES | from/to, actor and metadata recorded on every transition |
| Quotes as integer minor units (ADR-008) | VERIFIED | 2 | YES | no numeric or float amount column anywhere in the schema |
| Schema constraints as invariants | VERIFIED | 3 | YES | undocumented type/status, negative or foreign-currency quote, inverted range, second primary vehicle all refused by the database |
| Cursor pagination (ADR-009) | VERIFIED | 1 | YES | newest-first, no repeated row across pages |
| Repository layer | VERIFIED | n/a | YES | `internal/jobs/store.go`; no SQL outside it |

**Verification evidence (observed, 2026-08-28):**

```text
gofmt clean · go vet ok · 127 Go test functions
go test -tags=integration   38 tests, ok — real Postgres + PostGIS
migrate up → down → up      version 3 → all rolled back → version 3
```

**Verification level:** 3 for entities and repositories, **5** for every transition and the
assignment claim — those are concurrency-sensitive and are tested under real contention
rather than reasoned about.

**A modelling correction.** `Job.Live()` initially returned true for `COMPLETED`, because
document 15 does not list it as terminal — it is reachable by `DISPUTED`. Correct by the
letter, wrong in effect: dispatch and tracking ask "is this still running?", and answering yes
for a finished trip keeps a driver marked busy. `Live()` and `Finished()` are now distinct.

**Not done, deliberately:** no driver or vehicle *behaviour* (Phase 5), no dispatch (Phase 8),
no pricing logic — the quote table exists, CAP-1's boundary is created by the ride slice
(Phase 7). No proof model yet; it arrives with proof of delivery in Phase 9. Sixteen of the
seventeen modules are not created: empty directories are not architecture.

## Phase 5 — Providers, Vehicles and Service Eligibility

Verified 2026-08-28. Documents 16, 29, 30, 41, 108.

| Task | Status | Tests | Verified | Notes |
|------|--------|-------|----------|-------|
| Migration `000004` | VERIFIED | n/a | YES | up → down → up observed, including data mapping both ways |
| **Vehicle taxonomy as configuration (`030`)** | VERIFIED | 1 | YES | `vehicle_types`/`capabilities` reference tables replace `000003`'s CHECK constraints; a new type is a row, not a migration |
| Capabilities derived, never submitted (`030`) | VERIFIED | 1 | YES | `RegisterVehicle` has no capability input; `source` records DERIVED vs ADMIN |
| Driver verification machine (`029`) | VERIFIED | 3 | YES | seven states; rejection is not terminal, so resubmission works |
| Driver availability machine (`016`) | VERIFIED | 2 | YES | separate column and machine from verification, per `029`'s objective |
| Vehicle verification machine (`030`) | VERIFIED | 2 | YES | six states |
| Onboarding resumable and idempotent | VERIFIED | 1 | YES | "become a driver" twice does not reset progress |
| Active vehicle must be verified and owned | VERIFIED | 1 | YES | ownership and status checked inside the statement, not before it |
| Suspension clears the active vehicle | VERIFIED | 1 | YES | otherwise a driver keeps working on a suspended vehicle |
| Document model and expiry sweep (`029`) | VERIFIED | 2 | YES | one table for driver and vehicle documents; expiry is a single indexed sweep |
| **One shared eligibility implementation (`041`)** | VERIFIED | 16 | YES | `internal/eligibility`; dispatch and acceptance both call `Evaluate`, neither has a copy |
| Hard constraints reject candidates (`041`) | VERIFIED | 8 | YES | every constraint in `041`'s list, each exercised individually |
| Expired mandatory document blocks work (`016`) | VERIFIED | 3 | YES | inclusive at the boundary — a document expiring today is expired today |
| **Missing** mandatory document blocks work | VERIFIED | 1 | YES | requirements are LEFT JOINed *from* the requirement, so an unsubmitted document is a failure rather than an absent row |
| Stale location excludes from dispatch only | VERIFIED | 1 | YES | acceptance does not check it — the driver is present and answering |
| Every failure reported, not just the first | VERIFIED | 1 | YES | a driver fixing their profile learns all of it in one pass |
| Review actions audited (`029`) | VERIFIED | 1 | YES | `verification_reviews`; counts asserted |
| Concurrent availability transitions | VERIFIED | 1 | YES | 6 racers → exactly 1 winner |
| BD-14 handled structurally | VERIFIED | n/a | YES | `document_requirements` ships **empty**; the mechanism requires nothing until a market's list is supplied |

**Verification evidence (observed, 2026-08-28):**

```text
gofmt clean · go vet ok · 153 Go test functions · 14 unit packages ok
go test -tags=integration   50 tests, ok — real Postgres + PostGIS
migrate up → down → up      version 4 → rolled back → version 4
```

**A correction to Phase 4.** Migration `000003` encoded vehicle types and capabilities as CHECK
constraints, using the narrower vocabulary from the roadmap. Document `030` requires the
taxonomy be configuration-driven — "local names/categories can evolve" — and gives eight types
and eight capabilities, not seven and five. `000004` converts both to reference tables with
foreign keys and maps the existing rows in both directions. C-7 (verification vocabularies)
recorded and resolved at the same time.

**Not done, deliberately:** no HTTP surface for the `030` endpoints yet — the store and rules
are the phase's substance and the handlers follow the same pattern as Phase 3's. No signed
upload URLs (needs object storage wiring). No scheduled expiry worker; the sweep exists,
the scheduler is the worker framework's job. No service-area eligibility — zones are CAP-2
and arrive with Phase 6.

## Phase 6 — Location and Realtime Foundation

Verified 2026-08-28. Documents 18, 47, 48, 95, 96, 98, 102, 103.

| Task | Status | Tests | Verified | Notes |
|------|--------|-------|----------|-------|
| Migration `000005` | VERIFIED | n/a | YES | full chain down → up → down → up observed |
| **CAP-2 boundary created (`095`)** | VERIFIED | 12 | YES | `route`/`Matrix`/`EstimateETA`, normalized response, one provider behind it |
| Routing modes by vehicle (`095`) | VERIFIED | 2 | YES | a truck does not get a car route |
| Fallback never presented as exact (`096`) | VERIFIED | 3 | YES | every estimate is labelled `estimated`; a total provider outage still ranks candidates |
| `Drivers × Pickup` matrix (`096`) | VERIFIED | 2 | YES | the reduction Phase 8's `eta_score` performs |
| Location validation (`048`) | VERIFIED | 12 | YES | every check `048` lists: impossible coordinates, future and stale timestamps, impossible speed, unrealistic jumps |
| Jump detection tolerates real gaps | VERIFIED | 2 | YES | a tunnel exit is not a spoof; 60 km/h movement is not rejected |
| Out-of-order fixes dropped | VERIFIED | 2 | YES | buffered delivery reorders; an older fix must not move the driver backwards |
| Redis current state + geo pool (`018`) | VERIFIED | 4 | YES | only AVAILABLE drivers are in the dispatch pool; leaving is immediate, not TTL-based |
| Nearby bounded and ordered (`042`) | VERIFIED | 1 | YES | radius-bounded, nearest first — dispatch's candidate discovery |
| Durable history, batched (`048`) | VERIFIED | 2 | YES | batch-only signature; there is no synchronous single-fix insert to reach for |
| Retention mechanism, no default (BD-15) | VERIFIED | 1 | YES | cutoff is an argument; nothing decides how long location is kept |
| Tracking sessions unique per live job | VERIFIED | 1 | YES | two sessions would mean two answers to "who may watch this" |
| **Location access scoped and audited (`102`)** | VERIFIED | 2 | YES | a stranger is refused; granted *and* denied attempts are both logged |
| Visibility ends with the job (`102`) | VERIFIED | 1 | YES | "only location needed for active service" |
| Realtime channels and envelope (`047`) | VERIFIED | 3 | YES | document `047`'s exact envelope; `version` present from the first event |
| Subscription authorization mandatory (`047`) | VERIFIED | 4 | YES | strict channel parsing; **fails closed** with no membership check configured |
| Backpressure — slow clients (`047`) | VERIFIED | 1 | YES | 1000 events to a stalled client never block the publisher |
| Location coalescing (`047`) | VERIFIED | 1 | YES | 500 positions collapse to the newest one, not a backlog of stale ones |
| Connection limits (`047`) | VERIFIED | 1 | YES | per-user cap; quota released on disconnect |
| Concurrency safety | VERIFIED | 2 | YES | `go test -race` clean, including publish racing disconnect |

**Verification evidence (observed, 2026-08-28):**

```text
gofmt clean · go vet ok · 198 Go test functions · 17 unit packages ok
go test -race ./internal/realtime/   ok
go test -tags=integration            ok — real Postgres, PostGIS and Redis
migrate down → up → down → up        clean both ways
```

**Two defects found and fixed.**

*A migration that could not be rolled back.* `000004`'s down migration mapped vehicle types
back by name. A market that had configured a new type — exactly what `030` requires be
possible — left rows the restored CHECK constraint rejected, and the rollback failed partway,
leaving the database dirty. Rollback is now total rather than name-by-name: anything outside
the earlier vocabulary maps to a default. Lossy by nature, since the earlier schema had
nowhere to put it, but a rollback that fails the moment a market adds a type is not a rollback.

*A test that blamed the wrong code.* Phase 4's atomicity check counted stopless jobs across the
whole database. Phase 6's fixtures legitimately create jobs without stops, so the assertion
began failing for something it was not testing. Now scoped to its own requester.

**Not done, deliberately:** no WebSocket transport — the hub, envelope, authorization and
backpressure are the substance; binding it to a socket is Phase 12's client work. No NATS
fan-out between gateway instances (`018`'s horizontal scaling) until there is more than one.
No geocoding or map display (CAP-2, Phase 12). BD-17 sampling frequencies remain unset —
`182` requires they come from real-device measurement, not guesswork.

## Phase 7 — Ride Booking

Verified 2026-08-28. Documents 05, 15, 34, 35, 36.

| Task | Status | Tests | Verified | Notes |
|------|--------|-------|----------|-------|
| Migration `000006` | VERIFIED | n/a | YES | up → down → up observed |
| **CAP-1 boundary created** | VERIFIED | 15 | YES | `internal/pricing`; service-parameterized from the first line |
| A service is a rule set, not an engine | VERIFIED | 2 | YES | a second service registered in a test reuses the shared distance rule |
| **No rate value in Go** (`034`) | VERIFIED | n/a | YES | every number comes from a `pricing_tariffs` row; the engine holds a clock and nothing else |
| Document `034` breakdown complete | VERIFIED | 3 | YES | lines always sum to the total — asserted on every quote in every pricing test |
| Rounds once, half away from zero | VERIFIED | 2 | YES | rational arithmetic throughout; 3,333 successive quotes accumulate without drift |
| Minimum fare tops up, never reduces | VERIFIED | 2 | YES | applied before demand and tax |
| Demand computed and capped (BD-02) | VERIFIED | 7 | YES | ratio of waiting requests to available drivers, clamped to 1.5x by both the tariff and the platform ceiling |
| **Demand bounded by tariff** (`034`) | VERIFIED | 1 | YES | 3× against a 1.5× cap is refused; so is 0.5× — surge must not become a silent discount |
| Tax applies last | VERIFIED | 1 | YES | on what the customer actually pays |
| Discount never makes a fare negative | VERIFIED | 1 | YES | the platform would owe money for a ride |
| **Price lock** (`034`) | VERIFIED | 2 | YES | doubling the tariff does not move a confirmed price; a new quote does pick it up |
| Quote expiry enforced | VERIFIED | 1 | YES | |
| Quote ownership enforced (`035`) | VERIFIED | 1 | YES | otherwise one customer books at another's price |
| Quote is single-use | VERIFIED | 1 | YES | two jobs must not share one locked price |
| **Idempotent create** (`035`, `185`) | VERIFIED | 2 | YES | a retry returns the same job; exactly one row exists |
| Key reused with different content refused | VERIFIED | 1 | YES | replaying the first response would discard the second request |
| Cancellation tiers follow `005`, fee charged (BD-01) | VERIFIED | 9 | YES | free for 2 minutes from driver acceptance, then PKR 100; a job that never found a driver is never charged |
| Cancellation is state-aware (`036`) | VERIFIED | 2 | YES | a trip in progress cannot be cancelled |
| Concurrent cancellations | VERIFIED | 1 | YES | 6 racers → one cancellation, one history row |
| Driver commands validate ownership (`035`) | VERIFIED | 1 | YES | a driver cannot command a job they do not hold |
| Driver commands idempotent (`036`) | VERIFIED | 1 | YES | tapping "arrived" twice returns the job, not an error |
| **Commands walk the documented flow** | VERIFIED | 1 | YES | a trip cannot complete without passing through `ARRIVING` and `AT_DROPOFF` |
| C-8 resolved — state names | VERIFIED | n/a | YES | names from `015`, rules from `036` |

**Verification evidence (observed, 2026-08-28):**

```text
gofmt clean · go vet ok · 228 Go test functions · 18 unit packages ok
go test -tags=integration   76 tests, ok — real Postgres, PostGIS, Redis
migrate up → down → up      version 6
```

**A defect the tests caught.** Driver commands jumped straight to their target status, so
`arrive` moved a job `ACCEPTED → AT_PICKUP` and skipped `ARRIVING` entirely. The state machine
refused it — which is what a state machine is for — but the fix mattered more than the
refusal: commands now walk the main flow one documented transition at a time. Jumping would
have left intermediate states in the specification and never in the data, making "when did the
driver set off?" unanswerable.

**Not done, deliberately:** no HTTP handlers for the `035` endpoints yet. No dispatch — Phase 8
assigns drivers; `StartSearching` is the handoff. Cancellation **fees** and driver
compensation are unwired (BD-01). No payment capture at completion — Phase 11. Parcel, cargo
and grocery are refused rather than priced by the ride rule set; they arrive with their slices.

## Phase 8 — Dispatch Engine

Verified 2026-08-28. Documents 05, 38, 39, 40, 42, 43, 44, 45, 46, 49.

| Task | Status | Tests | Verified | Notes |
|------|--------|-------|----------|-------|
| Migration `000007` | VERIFIED | n/a | YES | up → down → up observed |
| Candidate pipeline (`039`) | VERIFIED | n/a | YES | geo → availability → capability → vehicle → freshness → eligibility → scoring, in cost order |
| **Eligibility is the Phase 5 implementation** | VERIFIED | n/a | YES | dispatch calls `eligibility.Evaluate`; it has no copy of the rules |
| Expanding radius rings (`039`, `044`) | VERIFIED | 1 | YES | configurable rings, bounded attempts |
| Weighted scoring (`040`) | VERIFIED | 11 | YES | every term in `040`'s formula, normalised before combination |
| ETA preferred over distance (`040`) | VERIFIED | 1 | YES | a driver across a river is close in metres and far in minutes |
| Unknown ETA is not instant | VERIFIED | 1 | YES | scoring a missing ETA as zero would make every unroutable driver the best candidate |
| New drivers not punished (`040`) | VERIFIED | 1 | YES | no history scores neutral; a driver punished for having no record never gets one |
| Fairness term (`040`) | VERIFIED | 1 | YES | idle time breaks ties, so the best drivers do not take every job |
| Weights are configuration (BD-03) | VERIFIED | 2 | YES | `dispatch_config`; changing them changes the ranking, proven |
| Scoring deterministic (`039`) | VERIFIED | 1 | YES | stable across 20 runs; ties break by id |
| **Explainability (`040`)** | VERIFIED | 2 | YES | every factor, weight and version stored with the decision — the inputs are volatile and gone by the time anyone asks |
| **One live assignment per job (`046`)** | VERIFIED | 2 | YES | asserted against **raw SQL**, so it holds for code that bypasses the application |
| **One active reservation per driver (`046`)** | VERIFIED | 1 | YES | 8 concurrent reservations of one driver → exactly 1 |
| **Atomic acceptance (`043`)** | VERIFIED | 2 | YES | verify offer → verify reservation → verify eligibility → assign → consume, all conditional writes in one transaction |
| Concurrent acceptance | VERIFIED | 1 | YES | 10 racers → 1 winner, 1 accepted assignment, job holds the winner |
| Expired offers cannot be accepted (`043`) | VERIFIED | 1 | YES | |
| Authoritative re-check at accept (`043`) | VERIFIED | 1 | YES | a driver suspended between offer and tap does not win, and the job is not left assigned |
| Rejection returns the job (`045`) | VERIFIED | 1 | YES | driver freed, job back to SEARCHING, **assignment history preserved** |
| Expiry sweep (`043`) | VERIFIED | 1 | YES | offer → EXPIRED, reservation → RELEASED, job → SEARCHING; durable, not a process timer |
| Event deduplication (`046`) | VERIFIED | 2 | YES | per consumer, durable; 8 concurrent deliveries → 1 processor |
| Outbox for atomic state + event (`046`) | VERIFIED | n/a | YES | written in the caller's transaction; an event cannot describe a rolled-back state |
| **Concurrency tests repeat (`046`)** | VERIFIED | n/a | YES | `-count=5` on every race test, green |

**Verification evidence (observed, 2026-08-28):**

```text
gofmt clean · go vet ok · 252 Go test functions · 19 unit packages ok
go test -tags=integration                    88 tests, ok
go test -tags=integration -count=5 (races)   ok — document 046's "repeatedly under parallel execution"
migrate up → down → up                       version 7
```

**Verification level: 5**, mandatory — dispatch assignment and concurrency both trigger it.

**A defect the tests caught.** `Reserve` updated `dispatch_attempts` unconditionally, casting
an empty attempt id to `uuid` and aborting the whole transaction. Every direct reservation
failed with "commit unexpectedly resulted in rollback" — a transaction that silently poisoned
itself rather than reporting the real error.

**A test that could not be written, which is the point.** An attempt to fabricate two live
offers for one job — simulating a defect elsewhere — was refused by the partial unique index.
The test now asserts that refusal directly, against raw SQL, because the guarantee has to hold
for a migration or an operator script as much as for this code.

**BD-04 was resolved on 2026-08-28** — B-5 is closed. Three rounds over ninety seconds, then
the job ends as `EXPIRED` with a `NO_SUPPLY` reason and nothing is charged; a sweeper catches
searches whose worker died. Retries are bounded
and the engine reports `ErrNoSupply` when the rings are exhausted; it deliberately does not
expire the job, because how long to search and what the customer sees are product decisions
document 044 leaves open.

**Not done, deliberately:** no NATS publication loop yet — the outbox is written, the publisher
is a worker. No batch offers (`043`: MVP prefers one driver at a time). No zone restrictions
(`039`'s service zones) — zones are a Phase 14 capability. No operator escalation path (BD-04).

## Phase 9 — Delivery and Cargo

Verified 2026-08-28. Documents 79–91.

| Task | Status | Tests | Verified | Notes |
|------|--------|-------|----------|-------|
| Migration `000008` | VERIFIED | n/a | YES | up → down → up observed |
| **Parcel and cargo are Job types** | VERIFIED | n/a | YES | no parallel order entity; same dispatch, same lifecycle |
| **Pricing by rule set, not a new engine** | VERIFIED | 2 | YES | PARCEL and CARGO registered behind CAP-1; the distance component is the identical code the ride slice uses |
| Cargo prices loading, parcel does not | VERIFIED | 1 | YES | the difference is a component list, not a branch |
| Recipient OTP hashed and single-use (`083`) | VERIFIED | 2 | YES | 32-byte keyed hash; a replayed code would authorise a second handover of a parcel already gone |
| Concurrent OTP verification | VERIFIED | 1 | YES | 8 racers → exactly 1 handover |
| OTP attempts bounded | VERIFIED | 1 | YES | past the limit the correct code fails too |
| Proof audit fields (`083`) | VERIFIED | 2 | YES | method, actor, capture location, media **reference** — never the binary |
| Proof required per service (`083`) | VERIFIED | 2 | YES | a ride needs none; parcel, grocery and cargo each have a default method |
| Photo/signature proof needs media | VERIFIED | 1 | YES | a photo proof with no photo is not proof |
| **Deterministic failure actions (`084`)** | VERIFIED | 3 | YES | a wrong address escalates rather than retrying the same wrong address; a rejection returns rather than retrying at the door |
| Retries bounded (`084`) | VERIFIED | 2 | YES | configurable limit, then RETURN |
| **Customer-safe messages (`084`)** | VERIFIED | 1 | YES | asserted that no internal code leaks — a customer told `MERCHANT_ISSUE` learns nothing |
| Return is a stop, not a new job (`084`) | VERIFIED | 1 | YES | goes back to the original pickup; job count stays 1 |
| **Cargo capacity beyond weight (`080`, `041`)** | VERIFIED | 5 | YES | a 3.5m/800kg load is refused by a motorcycle on length *and* weight; equipment is a hard constraint |
| Unknown vehicle capacity fails | VERIFIED | 1 | YES | passing would surface at the pickup as a driver who cannot do the job |
| Waiting and loading times (`087`) | VERIFIED | 3 | YES | grace period respected; **seconds recorded, never money** |
| Arrival idempotent | VERIFIED | 1 | YES | a repeated tap does not reset the waiting clock |
| Helper is an explicit requirement (`087`) | VERIFIED | 1 | YES | `loading_assistance`, not inferred from vehicle type |
| Restricted goods (BD-13, `088`) | VERIFIED | 1 | YES | table ships **empty**; the check passes vacuously and works the moment a list exists |

**Verification evidence (observed, 2026-08-28):**

```text
gofmt clean · go vet ok · 276 Go test functions · 20 unit packages ok
go test -tags=integration   99 tests, ok
migrate up → down → up      version 8
```

**Business decisions handled without inventing anything.** BD-10 (failed-delivery financial
treatment): the failure states, next actions and return stop are built; no fee or return-leg
price is written anywhere. BD-13 (cargo waiting/loading rates, restricted goods): time is
recorded in seconds and priced only if a tariff configures a rate — zero by default — and the
restricted-goods table ships empty because the list is legal and the owner's. BD-16 (proof
retention): proofs store object-storage references, so a retention policy is a lifecycle rule
on the bucket rather than a schema change here.

**Not done, deliberately:** no HTTP handlers. No signed upload URLs for proof media (needs
object-storage wiring). No multi-item packing optimisation — document 080 defers it explicitly.
No scheduled delivery windows.

## Phase 10 — Grocery and Merchant Platform

Verified 2026-08-28. Documents 65–78.

| Task | Status | Tests | Verified | Notes |
|------|--------|-------|----------|-------|
| Migration `000009` | VERIFIED | n/a | YES | up → down → up observed |
| **Order and delivery stay separate (`070`)** | VERIFIED | n/a | YES | an Order is merchant fulfilment and *produces* a Job; merging them would put PREPARING into the lifecycle every ride uses |
| Order state machine (`070`) | VERIFIED | 2 | YES | documented flow walkable; skipping fulfilment refused |
| Merchant rejection before preparation only | VERIFIED | 1 | YES | after picking starts, stock and staff time are consumed |
| Customer cancellation is state-aware | VERIFIED | 1 | YES | |
| **BD-12: 10 minutes, then auto-cancel** | VERIFIED | 4 | YES | platform default with a per-merchant override; a sweeper cancels unanswered orders, compare-and-set against a merchant accepting at the same moment |
| Acceptance timeout sweep | VERIFIED | 2 | YES | enforces the merchant's own configured deadline; an accepted order is not swept |
| Merchant timestamps (`072`) | VERIFIED | 1 | YES | `accepted_at`, `preparation_started_at`, `ready_at` set by the transitions that earn them |
| **Price snapshot on order lines (`068`)** | VERIFIED | 2 | YES | doubling a catalogue price does not move a stored line — referencing the live price would rewrite every past receipt |
| Order total always matches its lines | VERIFIED | 2 | YES | summed in the database from the stored snapshots |
| Cart is idempotent | VERIFIED | 1 | YES | one live cart per customer per store |
| **Inventory cannot oversell (`069`)** | VERIFIED | 3 | YES | 20 concurrent reservations against 5 units → exactly 5; a CHECK constraint backs the predicate |
| Reservations release on cancellation | VERIFIED | 1 | YES | |
| **Customer preference is authoritative (`074`)** | VERIFIED | 3 | YES | a merchant proposing a substitution for a `DO_NOT_ALLOW` item gets a removal instead |
| `ASK_ME` becomes a question, not a decision | VERIFIED | 1 | YES | removal still needs no permission |
| **Original order lines never mutated (`074`)** | VERIFIED | 2 | YES | name and price survive a substitution; the line is marked, not deleted |
| Removed items leave the total | VERIFIED | 1 | YES | the total stops including what nobody received |
| **BD-11: customer pays the substitute's price** | VERIFIED | 5 | YES | both directions; only settled substitutions reprice; the original order line is never mutated (`074`) — the total reads the substitute price from the issue row |
| Store hours decide availability | VERIFIED | 2 | YES | closing time exclusive; **no configured hours means closed**, because a store that has not said it is open should not take orders |
| Concurrent order transitions | VERIFIED | 1 | YES | 6 racers → exactly 1 |

**Verification evidence (observed, 2026-08-28):**

```text
gofmt clean · go vet ok · 302 Go test functions · 21 unit packages ok
go test -tags=integration   115 tests, ok
migrate up → down → up      version 9
```

**A schema defect the tests caught.** `inventory` had `variant_id` inside its primary key, so a
product without variants — most of them — could not have a stock row at all. Fixed with a
`NULLS NOT DISTINCT` unique index, which gives the same uniqueness while letting "no variant"
be a real single row.

**Not done, deliberately:** no HTTP handlers. No merchant payouts — settlement is Phase 11.
No `READY_FOR_PICKUP → create delivery job` wiring yet; the link column exists and the event
that drives it belongs with the worker framework. No add-ons or option groups beyond variants.

## Phase 11 — Payments and Financial System

Verified 2026-08-28. Documents 19, 51–64. **Level 5 throughout, mandatory.**

| Task | Status | Tests | Verified | Notes |
|------|--------|-------|----------|-------|
| Migration `000010` | VERIFIED | n/a | YES | up → down → up observed |
| **Double-entry ledger balances (`053`)** | VERIFIED | 4 | YES | checked in Go *and* re-summed in SQL before commit; the whole ledger sums to zero |
| Signed amounts, one column | VERIFIED | n/a | YES | "does this balance?" is `sum() = 0` — nobody can fill in the wrong column |
| **Entries are immutable (`053`)** | VERIFIED | 1 | YES | database **trigger**, so UPDATE and DELETE fail for psql and migrations too |
| Corrections are reversals | VERIFIED | 2 | YES | every account nets to zero across the pair; both transactions remain |
| No mixed currencies in a transaction | VERIFIED | 1 | YES | two currencies summing to zero would balance numerically and mean nothing |
| Earnings split gross exactly | VERIFIED | 2 | YES | net + commission = gross, asserted over 2,000 random splits |
| Commission never exceeds the earning | VERIFIED | 2 | YES | a driver must not owe money for working |
| **Concurrent captures (`059`)** | VERIFIED | 1 | YES | 10 racers → 1 capture, 1 ledger transaction, books still balance |
| **Concurrent refunds bounded (`059`)** | VERIFIED | 2 | YES | 10 × 2000 against a 10000 capture → exactly 5; the schema refuses an over-refund even by direct SQL |
| One live intent per job | VERIFIED | 1 | YES | two would let a customer be charged twice for one ride |
| Idempotent intent creation | VERIFIED | 1 | YES | a retry returns the original |
| **Webhook signatures verified (`058`)** | VERIFIED | 1 | YES | constant-time; tampered payload, wrong secret and empty signature all refused |
| Webhook deduplication is durable (`052`) | VERIFIED | 2 | YES | 8 concurrent deliveries → 1 processor; an in-memory set forgets exactly when providers replay |
| Capture and ledger commit together | VERIFIED | n/a | YES | a captured payment with no ledger entry is money the books do not know about |
| **BD-05: flat 20% commission** | VERIFIED | 2 | YES | seeded for all five services; an unconfigured combination still refuses, so a new service cannot silently inherit a rate |
| Balances derived, not stored (`053`) | VERIFIED | 1 | YES | a cached balance is a second source of truth that can disagree |
| One payout per subject per period (`059`) | VERIFIED | 1 | YES | paying a driver twice for one week is found by whoever reconciles the bank |
| **BD-08: zero tolerance (`058`)** | VERIFIED | 1 | YES | a one-paisa mismatch opens a case and adjusts nothing |
| **BD-09: COD allocates no liability** | VERIFIED | 1 | YES | cash sits in `CASH_IN_TRANSIT` against the driver holding it; nothing decides whose loss it is |

**Verification evidence (observed, 2026-08-28):**

```text
gofmt clean · go vet ok · 330 Go test functions · 22 unit packages ok
go test -tags=integration              131 tests, ok
money races with -count=3              ok — document 059's parallel/retry requirement
migrate up → down → up                 version 10
```

**Business decisions handled without inventing money.** BD-05 (commission rates): configuration
only; earnings return `ErrNoCommission` until a rate exists. BD-06 (refund policy): the refund
*mechanism* is built and bounded; no automatic policy decides when one happens. BD-08
(reconciliation tolerance): zero tolerance, the register's recommended default, adopted
explicitly — any discrepancy raises a case. BD-09 (COD liability): recorded as cash in transit
held by a driver, with no allocation of loss.

**Not done, deliberately:** no real payment provider adapter — the abstraction, signature
verification and webhook path exist; a provider contract does not. No automated payout
execution (needs banking integration). No maker/checker on admin financial actions (`059`
prefers it; it needs the admin console, Phase 13).

## Phase 12 — Mobile Production Features · **PARTIAL**

Documents 17, 28, 48, 116, 179.

**Built and verified**

| Task | Status | Tests | Notes |
|------|--------|-------|-------|
| Secure refresh-token storage (`028`) | VERIFIED | n/a | `expo-secure-store` → Keychain / Keystore. Document 28 forbids plain AsyncStorage |
| Shared auth flow, one copy for both platforms (`048`) | VERIFIED | 6 | a wrong code keeps the user on the code step rather than invalidating the code in their hand |
| User-facing error mapping | VERIFIED | 2 | asserted to leak no internal code; preserves the server's deliberate ambiguity about whether an account exists |
| ADR-006 consequences handled | VERIFIED | n/a | mobile Jest maps `@platform/*` to source, so transitive deps and ESM `.js` specifiers both needed handling |

**Not built.** Screens and navigation for booking, tracking, order history, notifications and
profile. Driver onboarding, availability, offer acceptance, trip workflow, earnings. Background
location and its native module. Offline mutation queue. Push registration. Performance budgets
(BD-19 requires real-device measurement).

## Phase 13 — Operational Dashboards · **PARTIAL**

Documents 135–147.

**Built and verified**

| Task | Status | Tests | Notes |
|------|--------|-------|-------|
| Dashboard API client | VERIFIED | n/a | refresh token held **in memory**, not localStorage — an operator console is the highest-privilege surface and a persisted token is readable by any script on the page |
| Operational job list with cursor paging | VERIFIED | 4 | same generated client as the mobile apps; a console with its own API layer drifts from what it explains |
| Operational triage | VERIFIED | 2 | `SEARCHING` first, because a job nobody has taken is the one needing attention |

**Not built.** Every admin screen beyond the job list: providers, vehicles, dispatch console,
payments, settlements, audit, feature flags. The entire merchant dashboard. `merchant-dashboard`
remains a reserved directory.

## Phase 14 — Production Infrastructure · **PARTIAL**

Documents 163–176, 301.

**Built and verified**

| Task | Status | Notes |
|------|--------|-------|
| Production container | IMPLEMENTED, **BUILD FAILS** | multi-stage, distroless, non-root, static binary; migrations travel with the binary that applies them. The build **fails at `go mod download`** (stage 4 of 6) — one run exited 1, others hung until killed. The cause is **not established**: it is plausibly the emulated amd64 Alpine layer and network on this machine, but that is a guess and nothing rules out a defect in the Dockerfile. **The image has never been built end to end and must not be relied on until it builds in CI on a native runner.** |
| Migrations never run at startup | VERIFIED | separate `migrate` command, so a rolling deploy cannot have two instances migrating at once |
| **Contract gate in CI** (ADR-007) | IMPLEMENTED | a Go type changed without regenerating fails the build instead of shipping a client describing a response the server no longer sends |
| Structured logging, tracing, health probes | VERIFIED | Phase 1; unchanged |

**Also unverified — and worse than unverified.** The container image does not currently build:
`go mod download` fails inside the build. I could not establish why on this machine, and I am
not claiming it is only an emulation artefact. Building it on a native CI runner is the next
step, and until that passes the Dockerfile should be treated as untested code, not as
infrastructure.

**Not built.** Terraform, AWS networking, ECS services and scaling, production Postgres/Redis
operations, secrets management, monitoring and alerting, backups and disaster recovery, CDN.
These need cloud credentials and an account that is not billing-locked.

## Phase 15 — Hardening and Release · **PARTIAL**

**Verification performed (observed, 2026-08-28)**

```text
gofmt clean · go vet ok
go test -race ./...          21 packages ok — race detector clean
go test -tags=integration    ok — real Postgres, PostGIS, Redis
pnpm typecheck 17/17 · lint 10/10 · test 17/17 (70 tests) · build 10/10
pnpm format:check clean
migrate up → down → up       version 10, reversible twice
```

**Security properties enforced and tested, not merely reviewed:** OTP and refresh tokens
hashed at rest; refresh rotation with reuse detection that revokes every session; constant-time
comparison on tokens, OTPs and webhook signatures; rate limiting by phone and IP; role *and*
resource authorization; realtime subscription authorization that fails closed; location access
scoped and audited; ledger entries immutable by database trigger; no client-trusted capability,
price or payment state.

**Not done.** Load and stress testing. Mobile performance measurement. Accessibility audit.
E2E suite — no journey harness exists. Remote CI has still never executed: the account has been
billing-locked since 2026-08-27, which is external to this repository.

## Remaining Non-Blocking Work

Phase numbers below use the `IMPLEMENTATION_PLAN.md` spine. See
`MASTER_IMPLEMENTATION_ROADMAP.md` for the governing order and the translation table.

| Phase | Status | Notes |
|---|---|---|
| 2 — infrastructure hardening | NOT_STARTED | local infrastructure landed in Phase 1; cloud is Phase 15 |
| 3 — backend foundation | NOT_STARTED | BD-07 due before financial code |
| 4 — authentication | NOT_STARTED | |
| 5 — canonical domain | NOT_STARTED | ADR-004 to resolve here |
| 6 — pricing / quote | NOT_STARTED | |
| 7 — dispatch | NOT_STARTED | BD-03, BD-04 |
| 8 — location + realtime | NOT_STARTED | BD-15, BD-17 |
| 9 — ride vertical slice | NOT_STARTED | BD-01 … BD-06 |
| 10 — delivery | NOT_STARTED | BD-10, BD-16 |
| 11 — grocery | NOT_STARTED | BD-11, BD-12 |
| 12 — cargo | NOT_STARTED | BD-13 |
| 13 — financial completeness | NOT_STARTED | BD-08, BD-09 |
| 14 — operations console | NOT_STARTED | |
| 15 — production readiness | NOT_STARTED | BD-14, BD-15, BD-19 |

## Blocked

| Item | Blocks | Tracked |
|---|---|---|
| B-1 — control docs 366/367/368 empty | AGENTS.md protocol as written | mitigated by `IMPLEMENTATION_PLAN.md`; **Phase 1 unaffected** |
| ~~B-2 — Go ↔ TS type strategy~~ | — | **CLOSED 2026-08-28** — ADR-007; Go authoritative, TypeScript generated |
| B-3 — 19 business rules | Phase 7 onward (roadmap numbering) | `BUSINESS_DECISION_REGISTER.md`; BD-07 now implemented as ADR-008 |
| ~~B-4 — four domains without a phase~~ | — | **CLOSED 2026-08-28** — resolved as CAP-2…CAP-5 |
| B-4 — maps/ETA, safety, notifications, analytics have no roadmap phase | Phase 6 onward | `BLOCKED_TASKS.md`; C-6 / R-6 in `DOCUMENT_CONFLICTS.md` |

## CI Execution Attempt

| | |
|---|---|
| **Run** | [33077085568](https://github.com/sarmadkung/rideme/actions/runs/33077085568) — pull request [#1](https://github.com/sarmadkung/rideme/pull/1) |
| **Date** | 2026-08-27 13:29 UTC |
| **Trigger** | `pull_request`, branch `feat/phase-1-foundation` → `main` |
| **Result** | **FAILURE — no job executed** |
| **Cause** | `The job was not started because your account is locked due to a billing issue.` |

| Job | Status |
|---|---|
| Detect affected surfaces | failure — runner never started |
| JavaScript workspace | skipped (depends on the above) |
| Go API | skipped |
| Go API — integration | skipped |

**What this does and does not tell us.** GitHub parsed the workflow, registered
it as active, and triggered it on the pull request, so the YAML is valid and the
triggers are correct. Beyond that, nothing was proven: no step ran, so the
install, lint, typecheck, test, build, `gofmt`, `go vet` and migration
up/down/up jobs are all unexecuted remotely.

This is an account-level billing lock, not a repository, permission or
configuration defect. It cannot be fixed from within the repository.

**Acceptance criterion 12 — "CI runs both paths and is green on a pull request"
— remains UNVERIFIED.** It is the only Phase 1 criterion in that state; the
other thirteen were verified locally with observed command output. Per the
implementation execution policy, remote CI verification is deferred to a
milestone rather than blocking implementation.

## Carried Into Phase 2

| Item | Why it matters |
|---|---|
| CI has never executed | Attempted 2026-08-27 and blocked by an account billing lock, not by the configuration. Re-attempt when billing is restored; until then Phase 1 is complete on local evidence with criterion 12 outstanding. |
| Error taxonomy duplicated across Go and TypeScript | Two hand-maintained lists that must agree. Harmless now, expensive when a payment payload drifts. B-2. |
| `postgis/postgis:16-3.4` is amd64-only | Runs under emulation on Apple Silicon. Works, but slower; a multi-arch image is worth evaluating if it bites. |
| MinIO runs with no consumer | Deliberate — the local environment should match the deployed one — but nothing verifies it beyond the container health check. |


## Remaining Non-Blocking Work

Carried out of Phase 2. None blocks Phase 3.

| Item | Why it is open | Due |
|---|---|---|
| Event name versioning scheme | Document `150` states that event names are versioned but specifies no scheme. None was invented. The `domain.action` shape is validated; the version convention is not. | Before the first event producer (Phase 4 at the earliest) |
| Event `properties` has no schema | No event has a documented property schema, so `properties` is `Record<string, unknown>`. Constraining it now would be authoring specification. A monetary value inside it must use the `Money` type — BD-07 admits no exception. | With the first events that carry properties |
| Cursor encoding | ADR-009 fixes the pagination envelope, not how a cursor is built. No list endpoint exists yet. | First list endpoint |
| `contractgen` registration list is hand-written | Generation removes hand-maintained *shapes*; the list of registered types is still hand-maintained. Guarded by tests that fail when a Go constant is added without registration — a smaller surface than three duplicated files, not zero. | Ongoing |
| Remote CI has still never executed | Account billing lock, external to the repository. Unchanged since 2026-08-27. `make contracts-check` now runs inside `make verify`, so CI will enforce the contract gate once it can run at all. | Phase 15, or sooner if billing is restored |


---

## Business Decisions Resolved — 2026-08-28

The owner answered the six decisions that were blocking go-live. Every mechanism for them was
already built and refusing to act; what changed is that the values now exist.

| | Decision | Configured in |
|---|---|---|
| BD-01 | Free to cancel for 2 minutes after driver acceptance, then PKR 100 | `platform_settings.cancellation.*` |
| BD-02 | Surge is demand-triggered, capped at 1.5x | `platform_settings.surge.*`, `pricing_tariffs.demand_max_bps` |
| BD-04 | 3 dispatch rounds over 90s, then `EXPIRED` / `NO_SUPPLY`, no charge | `dispatch_config.max_attempts`, `platform_settings.dispatch.*` |
| BD-05 | Flat 20% platform commission on every service | `commission_rates` |
| BD-11 | Customer pays the substitute's actual price, up or down | `order_item_issues`, read by the order total |
| BD-12 | Merchants have 10 minutes to accept, then auto-cancel | `platform_settings.merchant.*`, `merchant_config` |

### What was built

| Component | Purpose |
|---|---|
| `migrations/000011_business_decisions` | `platform_settings`, the seeded values, and the schema the decisions need |
| `internal/settings` | Reads platform values with a short cache; a missing key is an error, never a zero |
| `internal/booking/cancellation.go` | BD-01's policy, measured from driver acceptance |
| `internal/pricing/demand.go` | BD-02's multiplier, integer basis points, capped twice |
| `internal/dispatch/runner.go` | BD-04's search policy and its sweep |
| `internal/sweeper` | Runs BD-04's and BD-12's deadline passes on an interval, wired into the server |

### Verification

| Gate | Result |
|---|---|
| `go test ./...` | pass |
| `go test -tags integration ./tests/` | pass |
| `go test -race` (unit and integration) | pass, no races reported |
| Concurrency tests at `-count=3` | pass |
| `make verify` (both toolchains) | pass |
| Migration 000011 down/up round trip | pass |

### Design decisions worth recording

**Values are rows, not constants.** Every decided number lives in the database, so changing a
commission rate is an edit with an audit trail rather than a deploy.

**The refusals were kept.** BD-05's rate table is seeded for the five services that exist, but a
combination with no row still returns `ErrNoCommission` — a service added later cannot silently
inherit a rate. A missing settings key is an error rather than a zero, because zero is meaningful
for most of these: a zero grace window charges every cancellation, a zero timeout cancels every
order instantly.

**Deadlines needed something to act on them.** BD-04 and BD-12 both set a clock, and a stored
deadline that nothing sweeps is a timestamp rather than a rule. Both sweeps compare-and-set
against the state they are ending, so a merchant accepting as its deadline passes — or a driver
accepting as a search gives up — wins cleanly rather than racing.

**BD-11 versus document 074.** The customer must pay the substitute's price, and the original
order line must never be mutated. Repricing the line in place satisfied the first and broke the
second, which an existing test caught. The order total now reads the substitute price from the
issue row instead, so what was ordered and what was charged live in separate rows and both stay
readable.

**Failing to measure demand is not a reason to refuse a quote.** BD-02's multiplier returns
neutral on every error path. An unreachable database costs the platform a surge it might have
charged; it does not cost the customer a fare.


---

## Phase 12 — Customer Booking Flow · 2026-08-29

The first product surface over the finished backend: quote → confirm → track →
cancel, in the customer mobile app.

| Task | Status | Tests | Evidence |
|---|---|---|---|
| Booking flow as shared state (`useBooking`) | VERIFIED | 10 | planning → quoted → tracking, one copy for both platforms |
| Idempotent confirmation | VERIFIED | 2 | one key per quote, reused across confirm attempts; a new quote takes a new key |
| Quote breakdown on screen | VERIFIED | 6 | every fare line rendered, including BD-02's demand line |
| Route confidence surfaced | VERIFIED | 2 | document 096 — an estimated route is labelled, a measured one is not |
| Live job tracking | VERIFIED | 1 | polled every 5s, stops at every terminal state including `EXPIRED` |
| Cancellation with its fee | VERIFIED | 3 | BD-01's amount is shown; a free cancellation says so explicitly |
| BD-04 in plain words | VERIFIED | 1 | `EXPIRED` reads "No drivers available … you have not been charged" |
| Money formatting | VERIFIED | 7 | integer minor units divided once, at display |
| Sign-in screen | VERIFIED | — | phone → code, over the existing `useAuth` |

**Defect found and fixed.** `QuoteResponse` declared `total_minor` and `currency`;
the handler wrote an ad-hoc map carrying `total` and `lines`. The generated
TypeScript client followed the struct, so `client.quote()` would have thrown a
parse error against a valid response — the endpoint had no client and no
HTTP-level test, so nothing had exercised the pair. The contract generator
cannot catch this class of bug: it compares Go structs to TypeScript and never
sees what a handler writes. `TestTheQuoteEndpointSendsExactlyItsDeclaredShape`
now decodes a real response with `DisallowUnknownFields`.

### What this deliberately does not do

| Not built | Why |
|---|---|
| Map selection | No map provider is integrated (CAP-2). Pickup and destination are chosen from named places rather than a pin. The flow behind the control is the real one. |
| Navigation stack | The flow is linear with no back destination worth preserving. A navigator before a second flow is scaffolding without a user. |
| Realtime tracking | The gateway exists; no client transport does. Polling every 5s, stopping at terminal states. |
| Driver location on a map | Follows map selection. The job's assignment is shown, not its position. |
| Scheduled rides, fare estimates history, saved places | Not in this slice. |

`EXPO_PUBLIC_CITY` selects the market's tariff and is deliberately **not**
defaulted: tariffs are per city and the platform ships none, so inventing one
would ask the server for a fare in a market nobody configured. Unset, quoting
fails with the server's own "not available here yet".


---

## Phase 12 — Driver Trip Flow · 2026-08-29

The other half of the loop. Without it nothing can accept a ride, so the
customer flow could not complete end to end.

### The gap this closed

A driver could be *offered* a job and had no way to learn about one. Every
piece of domain logic existed and was tested — availability is a state machine
in `providers`, location validation is in `tracking`, the trip commands are in
`booking` — but no endpoint exposed any of it.

| Endpoint | Purpose |
|---|---|
| `GET /driver/me` | the driver's own record |
| `POST /driver/online` | join dispatch, with a position |
| `POST /driver/offline` | leave dispatch |
| `POST /driver/location` | report a batch of fixes |
| `GET /driver/assignment` | the offer or trip being held |

| Task | Status | Tests | Evidence |
|---|---|---|---|
| Going online joins the dispatch pool | VERIFIED | 3 | status **and** geo pool, or neither — a failed pool write rolls the status back |
| Going online requires a vehicle and a position | VERIFIED | 2 | dispatch matches on vehicle capability; "online" with no location is not a usable state |
| Going offline withdraws from dispatch | VERIFIED | 1 | pool first, then status — a stale label beats a driver still receiving offers |
| Batched location reports | VERIFIED | 2 | one bad fix does not cost the batch, and a rejected fix never becomes the next baseline |
| Current assignment | VERIFIED | 2 | idle is not an error; the offer carries the server's expiry |
| Shift state as a shared hook | VERIFIED | 9 | mirrors the server rather than tracking a parallel copy |
| Offer countdown from server expiry | VERIFIED | 2 | a local TTL would let a driver accept an offer that has already gone |
| One command at a time (doc 035) | VERIFIED | 6 | ACCEPTED → arrive → start → complete, never skipping |
| Blocked-driver explanations | VERIFIED | 3 | verification, vehicle and location each named specifically |

### Shared mobile package

`useAuth` and the Keychain-backed token storage moved to `@platform/mobile`
rather than being copied into the second app. Document 048 forbids duplicating
business logic per platform, and an auth flow copied into two apps differs in
exactly the ways nobody tests. Screens stay in the apps; only what is
genuinely identical is shared.

### Known gap

**`expo-location` is not wired up.** Going online reports a fixed coordinate.
A driver going online from the wrong place would be offered jobs across the
city, so this must be replaced before the app is put in front of anyone. It is
marked in the source at `apps/driver-mobile/App.tsx`.

Also absent, and deliberately: no map, no navigation hand-off, no push
notification for an offer (the app polls while online and stops when offline),
no earnings screen.
