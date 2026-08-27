# internal/

Domain modules live here, one package per bounded context, each layered
`handler → application → domain → repository` (document 25).

Empty in Phase 1 by design: the foundation slice implements no domain logic.
The module list itself is still open — document 09 and document 25 disagree,
tracked as ADR-004 / conflict C-5, and is settled when the first domain module
lands in Phase 5.
