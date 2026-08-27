---
name: project-discovery
description: Establishes the ground truth of this repository — stack, layout, what exists, and which docs are real vs boilerplate. Use this FIRST in any session that will touch RideMe code, before reading docs or writing anything, and any time you catch yourself about to re-derive the architecture from scratch.
---

# Purpose

Give one cheap, repeatable answer to "what is this repo and what already exists?" so no session spends thousands of tokens rediscovering it.

# When to Use

- Start of any implementation session.
- Before picking up a task from the work queue.
- When a skill or doc references code you cannot find.

# Ground Truth (verified 2026-08-27)

**Stack is locked by `docs/12-technical-blueprint.md`:**

| Layer | Choice |
|---|---|
| Customer + Driver mobile | React Native + Expo + TypeScript |
| Admin + Merchant web | React + Vite + TypeScript |
| Marketing web | Next.js |
| Backend | **Go** (modular monolith) |
| API | REST + WebSocket |
| Database | PostgreSQL + PostGIS |
| Cache / ephemeral | Redis |
| Messaging | NATS |
| Storage | S3-compatible |
| Infra | Docker + AWS ECS/Fargate + Terraform |
| CI | GitHub Actions |
| Observability | Sentry + OpenTelemetry; PostHog analytics |

**Target layout** (`docs/09-project-structure.md`): `apps/`, `services/api/`, `packages/`, `infra/`, `docs/`.

**Backend modules** (`09`): identity, users, drivers, vehicles, documents, jobs, dispatch, pricing, payments, wallet, ratings, support, merchants, notifications, zones, fraud, analytics.

**Current repository state: documentation only.** No `apps/`, no `services/`, no `packages/`, no package manager, no Go module, no CI. Every implementation task is greenfield until this section says otherwise.

# The Documentation Tier Map

564 numbered documents in `docs/` (0–564; **11 is the only one absent**). They are **not** uniformly useful — verified by structural signature clustering, see `docs/DOCUMENT_AUDIT.md`:

- **Tier A — substantive (`000`–`190`).** 190 docs, 28,321 words. Real decisions: schemas, state machines, formulas, event names. Read these.
- **Tier B — thin restatement (`191`–`204`).** 14 docs, 105–338 words each. Read the Objective; expect no depth.
- **Tier C — template boilerplate (`205`–`564`).** **360 docs, 109,600 words, in nine batches of exactly 40** — each batch sharing one section-header signature and near-identical body text. Within a batch, only the title and a one-sentence Objective vary. Use as a topic index; never read in bulk expecting specification detail.

Tier C is 78% of the words and almost none of the information.

Practical consequence: to learn what the platform *does*, read Tier A. To learn what still needs *building*, scan Tier C titles.

# Rules

- Verify before trusting this file. It is a cache, not scripture — if it disagrees with the repo, the repo wins and you update this file.
- Never recreate something that exists. Search first.
- Do not read more than ~5 docs to answer a scoping question.

# Workflow

1. `ls` the repo root and `git log --oneline -5`.
2. Confirm the "current repository state" line above still holds.
3. Identify the target module from the backend module list.
4. Search for existing implementation before designing anything.
5. If ground truth has drifted, update this file in the same commit.

# Verification

Level 0 (see `verification-lite`) — discovery changes no application behavior.

# Blocking Conditions

- Repo contradicts the locked stack → stop, record in `docs/DOCUMENT_CONFLICTS.md`.
- A task needs a component whose owning module is undefined → stop, ask.

# Relevant Documentation

`docs/12-technical-blueprint.md` (stack) · `docs/09-project-structure.md` (layout) · `docs/04-domain-architecture.md` (domains) · `docs/365-master-document-index.md` (index intent — Tier C)
