# pkg/auth

Token issuance, verification and RBAC primitives (document 25).
Empty in Phase 1 — authentication is Phase 4. `JWT_SECRET` is already validated
at startup so the configuration seam exists before the implementation does.
