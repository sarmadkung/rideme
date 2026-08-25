# 204 — Agent Execution Protocol

## Objective
Provide a deterministic protocol for the coding agent to implement the platform from the documentation.

## Starting point
Read:
1. README.md
2. Documents 191–204
3. The current repository state

Then identify the earliest incomplete implementation document.

## Execution loop

```text
Read document
 ↓
Read prerequisites
 ↓
Inspect existing code
 ↓
Identify reusable implementation
 ↓
Plan changes
 ↓
Implement
 ↓
Run tests
 ↓
Run typecheck/lint
 ↓
Review acceptance criteria
 ↓
Update implementation status
 ↓
Commit
 ↓
Move to next document
```

## Mandatory rules

### Do not duplicate
Before creating a module, route, table, component, worker, or utility, search the repository for an existing implementation.

### Preserve architecture
Do not silently change architectural decisions. Record proposed changes and their impact.

### Tests are part of implementation
Do not mark a document complete while its required tests are missing.

### Database safety
Use migrations. Never make undocumented production schema changes.

### API safety
Maintain documented contracts and version breaking changes.

### Security
Never weaken authorization or expose secrets to make a task easier.

### Idempotency
Critical commands must be safe to retry.

### Observability
Production-critical operations require logs/metrics/tracing as appropriate.

### Mobile
Prefer shared React Native logic. Isolate native functionality behind interfaces.

### Completion
A document is complete only when:
- implementation exists
- tests pass
- acceptance criteria pass
- no known blocker remains
- documentation/status is updated

## Handling conflicts
If the current repository contradicts a document:
1. Stop the conflicting implementation.
2. Identify the conflict.
3. Determine whether the code or specification is newer.
4. Record the decision.
5. Continue only after the source of truth is clear.

## Working continuously
The agent may proceed through documents sequentially without requesting approval for routine implementation decisions, provided it stays within the architecture and acceptance criteria.

For ambiguous high-impact architectural decisions, create a decision record rather than guessing.

## Final objective
The agent should transform the written specification into a tested, deployable logistics platform without losing traceability between requirements and implementation.
