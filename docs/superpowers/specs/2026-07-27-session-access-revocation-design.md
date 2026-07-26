# F-14 Session Access Revocation Design

**Date:** 2026-07-27

**Branch:** `codex/reconcile-code-reviews`

**Status:** Design approved; implementation not started

**Finding:** F-14 - access tokens remain usable after their auth session is revoked

## 1. Goal

Make access-token authorization for administrators, merchants, and buyers depend on the current database session and account state.

After logout or another session revocation, the corresponding access token must stop authorizing requests immediately rather than remaining usable until its two-hour JWT expiry. A disabled account must also lose access immediately. Merchant review changes and current account roles must take effect without waiting for token refresh.

The change must preserve anonymous routes, existing JWT and database schemas, and the established API error envelope. It must not deploy to production, modify production data, or claim test-server approval before isolated evidence exists.

## 2. Current Behavior

All issued access JWTs already contain a nonzero `sid` that identifies `auth_sessions.id`. Logout sets `auth_sessions.revoked_at`, and refresh rejects missing, expired, revoked, or identity-mismatched sessions.

The global request chain currently parses access JWTs with `OptionalAuth`, then applies `RequireActiveAdminSession`. Only administrator actors are checked against `auth_sessions`; merchant and buyer requests trust the JWT until its own expiry. Account state and merchant review scope are read only at login or refresh, so those authorization claims can also become stale.

## 3. Non-goals

- Do not change access-token or refresh-token claims, signing algorithms, TTLs, or secrets.
- Do not add a revocation cache, account-version claim, Redis dependency, or token blacklist.
- Do not add or alter SQL migrations, tables, columns, or indexes.
- Do not make logout retry with an already revoked access token return success.
- Do not implement F-12 buyer identity migration, F-15 idempotency, or another open finding.
- Do not add administrator role policy that does not already exist. This change only refreshes the role stored in the request actor.
- Do not add authentication-middleware logs containing tokens, session IDs, account IDs, account state, or authorization query results. Existing business operation-audit records remain unchanged.
- Do not transfer source, run an isolated server project, deploy, restart production services, or modify production data without separate authorization.
- Do not modify, stage, commit, or transfer `.tmp/` or the three protected untracked review documents.

## 4. Approaches Considered

### 4.1 Adopted: authoritative session and account lookup on every authenticated request

After JWT parsing, query the session by primary key and query the corresponding account by primary key. For a merchant, join the merchant account to its merchant row in one account query. Reject invalid session or account state and replace stale JWT authorization fields in the request actor with current database values.

This provides immediate revocation and immediate account-state enforcement with the schema already present. Each authenticated request adds two indexed reads. Anonymous requests add no database read.

### 4.2 Rejected: account or session version inside the JWT

A version claim would require a schema change and still require an authoritative version lookup on each request unless a cache is introduced. It adds rollout compatibility and migration work without reducing the core database dependency.

### 4.3 Rejected: short access TTL plus checks only on selected routes

A shorter TTL limits but does not eliminate the revocation window. Route-specific checks also leave authorization gaps and cannot satisfy the approved requirement that every administrator, merchant, and buyer access token be revoked immediately.

## 5. Request Architecture

The global middleware order remains:

```text
Recovery
  -> RequestID
  -> OptionalAuth
  -> RequireActiveSession
  -> route middleware
  -> handler
```

`OptionalAuth` remains responsible for the cryptographic boundary:

1. If the Authorization header is absent, leave the context anonymous.
2. If the header is malformed or the JWT signature/registered claims are invalid, abort with HTTP 401 / code `10002`.
3. If the JWT is valid, place its actor identity and claims in the Gin context.

`RequireActiveSession` replaces `RequireActiveAdminSession` and owns the authoritative authorization-state boundary:

1. If the context has no actor, continue without querying the database.
2. Require a nonzero session ID.
3. Load `auth_sessions` by its primary key.
4. Require exact `user_type` and `user_id` equality between the session and JWT actor.
5. Require `revoked_at IS NULL` and `expired_at > now`.
6. Load and validate the current account state for the actor type.
7. Replace database-authoritative actor fields in the context before continuing.

There is no process-local or distributed cache. Immediate revocation depends on every authenticated request observing the database.

## 6. Authoritative Actor Loading

### 6.1 Administrator

Load the non-deleted `admin_users` row by `actor.UserID`.

- `status=ACTIVE` is accepted.
- `status=DISABLED` returns account disabled.
- Any unknown status or role is an internal invariant failure.
- Replace `actor.Role` with the current database role.
- Preserve the session-derived user identity and clear merchant-specific fields.

The accepted administrator roles are the existing `SUPER_ADMIN` and `ADMIN` constants. This design does not add new role-based route restrictions.

### 6.2 Merchant

Load the non-deleted `merchant_accounts` row by `actor.UserID` and join its non-deleted `merchants` row in the same query.

- Account `status=ACTIVE` is accepted; `DISABLED` returns account disabled.
- Review `APPROVED` produces `scope=full`.
- Review `PENDING` or `REJECTED` produces `scope=onboarding`.
- Review `DISABLED` returns account disabled.
- Unknown account status, review status, or account role is an internal invariant failure.
- Replace `actor.Role`, `actor.MerchantID`, and `actor.Scope` with current database values.

The accepted merchant account roles are the existing `OWNER` and `STAFF` constants. A JWT containing an old merchant ID, role, or scope is not itself authoritative; the current account relationship wins.

This makes an old `full` token lose full merchant routes immediately after the merchant review state changes away from `APPROVED`. Existing scope middleware then returns the established review-not-approved response for a valid onboarding actor.

### 6.3 Buyer

Load the non-deleted `buyer_users` row by `actor.UserID`.

- `status=ACTIVE` is accepted.
- `status=DISABLED` returns account disabled.
- Any unknown status is an internal invariant failure.
- Set the current actor role to `BUYER`, clear merchant-specific fields, and use `scope=full`.

### 6.4 Unsupported actor types

An access token with an unsupported `user_type` is unauthorized. `PUBLIC` is not a valid authenticated session actor.

## 7. Error Semantics

| Condition | HTTP | Code | Meaning |
| --- | ---: | ---: | --- |
| Missing Authorization header | Continue | n/a | Anonymous request; no session/account query |
| Invalid JWT or zero `sid` | 401 | `10002` | Unauthorized |
| Session absent, expired, revoked, or identity-mismatched | 401 | `10002` | Unauthorized |
| Account absent or soft-deleted | 401 | `10002` | Unauthorized |
| Unsupported actor type | 401 | `10002` | Unauthorized |
| Account or merchant review explicitly disabled | 403 | `10007` | Account disabled |
| Session/account query failure | 500 | `20001` | Internal error; fail closed |
| Unknown status, review state, or role | 500 | `20001` | Data invariant failure; fail closed |

A public route carrying an invalid or revoked access token returns its authorization error rather than silently downgrading to anonymous. This matches the existing behavior for malformed or expired JWTs. A public route with no Authorization header remains available and performs no session/account read.

No error response includes actor, token, session, account, or database details.

## 8. Logout Semantics

The logout handler continues to revoke only the session named by the authenticated actor. Tighten the update predicate to:

```text
id = actor.SessionID
AND user_type = actor.UserType
AND user_id = actor.UserID
AND revoked_at IS NULL
```

The handler requires exactly one affected row:

- One affected row returns success.
- Zero affected rows returns unauthorized. This covers a concurrent second logout that passed middleware before the first request committed its revocation.
- A database error returns internal error.

After the first successful update, subsequent requests using the old access token are rejected by `RequireActiveSession`, and refresh remains rejected by the existing refresh handler. Logout remains scoped to the current session and does not revoke other sessions for the same user.

## 9. Performance and Availability

An authenticated request performs exactly two authorization reads:

1. one `auth_sessions` primary-key lookup;
2. one actor primary-key lookup, with a merchant join when applicable.

Anonymous requests perform neither read. The implementation must select only fields needed for authorization and must not introduce a full-table scan. Isolated MySQL evidence records `EXPLAIN` plans showing primary-key access for the session and actor paths.

Database availability becomes a deliberate dependency for authenticated access. A database failure returns HTTP 500 / `20001`; it never falls back to trusting stale JWT claims.

## 10. Local Test Strategy

Behavior changes follow RED -> GREEN. Add focused middleware tests and API integration tests against the repository's real Gin/GORM stack.

### 10.1 Middleware and state matrix

Required cases:

1. An anonymous `/healthz` request succeeds after the database handle is unavailable, proving no actor means no authorization query.
2. A valid token on a public route is accepted only while its session and account are active.
3. A public request carrying a revoked token returns HTTP 401 / `10002` rather than becoming anonymous.
4. Zero session ID, missing session, expired session, revoked session, and session identity mismatch each return HTTP 401 / `10002`.
5. Missing or soft-deleted administrator, merchant, and buyer accounts return HTTP 401 / `10002`.
6. Explicitly disabled administrator, merchant, buyer, and merchant review state return HTTP 403 / `10007`.
7. Unknown account status, merchant review state, or unsupported role value returns HTTP 500 / `20001`.
8. Session query and account query failures return HTTP 500 / `20001` and do not reach the handler.
9. Administrator role, merchant role/ID/scope, and buyer role/scope in the downstream actor match current database values rather than stale JWT claims.
10. A merchant token issued with `scope=full` immediately loses full-route access after review changes to `REJECTED`, while the profile/onboarding boundary remains available.

### 10.2 Revocation contracts

For administrator, merchant, and buyer sessions:

1. Login and prove an authenticated route succeeds.
2. Logout with the access token and require success.
3. Reuse the old access token on an authenticated route and require HTTP 401 / `10002`.
4. Reuse the old refresh token and require HTTP 401 / `10002`.
5. Prove another session belonging to the same or a different actor remains active.

Also cover two concurrent logout requests: exactly one succeeds and the other is unauthorized; the session has one non-null revocation timestamp and no unrelated session changes.

### 10.3 Regression commands

Run from the repository root:

```bash
make test
cd backend && go vet ./...
cd ..
git diff --check
```

The complete Go suite must retain the administrator password-change revocation tests, frontend-server logout backend contract, miniapp refresh/logout contract, merchant onboarding boundaries, file privacy authorization, and all prior domain tests.

## 11. Isolated Test-Server Acceptance

F-14 has no schema migration but requires MySQL 8.4 behavior evidence. Use a dedicated remote directory and Compose namespace:

```text
Remote directory: /home/yu/services/secondhand-session-revocation-acceptance-20260727
Compose project:  secondhand-session-revocation-acceptance
```

No transfer or remote execution is authorized by design approval. Before transfer, obtain separate user authorization for that exact path, project, and whitelist.

The transfer whitelist is limited to:

- `backend/` source, tests, migrations, and Dockerfile;
- `deploy/acceptance/` source scripts and manifests, excluding `.env`, secrets, and evidence;
- `Makefile`;
- `backend/go.mod`;
- `backend/go.sum`.

Always exclude:

- `.env` files, credentials, and secrets;
- databases, upload files, and evidence directories;
- `.git`, caches, and `node_modules`;
- `backend/app.db` and miniapp private configuration;
- `.tmp/` and the three protected untracked review documents.

Acceptance uses an isolated MySQL 8.4.x container and synthetic fixtures only. It must prove:

1. local and remote committed-source SHA-256 equality;
2. all three actor types lose access and refresh immediately after logout;
3. all three explicit account-disable paths fail closed with the approved error;
4. merchant review downgrade immediately removes full scope;
5. missing, mismatched, expired, and revoked sessions are unauthorized;
6. database failure is internal error rather than authorization bypass;
7. session and actor queries use indexed primary-key plans;
8. full backend tests and `go vet ./...` pass in the isolated source tree;
9. the dedicated Compose project is distinct from production and production container identity, state, and restart counts remain unchanged.

The harness must not enable shell tracing while tokens exist. Tokens and actor identifiers remain in process variables only and are never printed or written to evidence. Retained evidence contains only source hashes, tool versions, test counts, exit codes, sanitized API codes/statuses, query-plan access types, and non-secret Compose identity.

## 12. Documentation and Status Rules

The authoritative status must distinguish three independent states:

| State | Required wording |
| --- | --- |
| Implementation and local gates pass | Code-side fixed; test-server review pending |
| Authorized isolated MySQL acceptance passes | Fixed and passed isolated test-server review |
| Production deployment/data | Not deployed; no production data modified |

After implementation evidence exists, update the tracked `docs/full-project-code-review-2026-07-24.md` and `docs/release-readiness.md`. Add the design and implementation-plan paths, commit range, exact local verification commands/results, and remaining server/production gates.

After authorized test-server evidence exists, add a sanitized report at `docs/superpowers/reviews/2026-07-27-session-access-revocation-isolated-acceptance.md` and update the two tracked status documents. Do not rewrite historical finding text, and do not modify the three protected review documents.

## 13. Expected File Scope

Implementation is expected to remain within:

- `backend/internal/middleware/auth.go` and focused middleware tests;
- `backend/internal/app/server.go`;
- `backend/internal/app/auth_handlers.go`;
- focused backend integration tests;
- an F-14 acceptance script and Makefile target;
- the F-14 implementation plan, acceptance report, and tracked status documents.

No frontend, miniapp, model, migration, production configuration, credential, database, or upload artifact is changed for F-14.

## 14. Acceptance Criteria

- Every valid administrator, merchant, and buyer access token requires an active identity-matched database session.
- Logout makes the current access and refresh token unusable immediately without affecting unrelated sessions.
- Disabled or deleted accounts cannot continue using an old access token.
- Current administrator role and merchant role/relationship/review scope replace stale JWT authorization fields on every request.
- Anonymous routes without a token remain independent of session/account database reads.
- Authorization database failures and data invariant failures fail closed.
- No JWT/schema/TTL change or production mutation occurs.
- Focused RED -> GREEN tests, full backend tests, `go vet ./...`, and `git diff --check` pass locally.
- Test-server approval is recorded only after separately authorized isolated MySQL 8.4 evidence passes.
- Status documents distinguish code-side closure, test-server approval, and production deployment accurately.

## 15. Approval Record

- Architecture approved by the user on 2026-07-27.
- Data flow and error semantics confirmed by the user on 2026-07-27.
- Complete design approved by the user on 2026-07-27.
