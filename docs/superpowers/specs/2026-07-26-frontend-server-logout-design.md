# Frontend Server Logout Design

**Date:** 2026-07-26
**Finding:** F-08 - frontend logout clears only local credentials
**Status:** Implemented (2026-07-26)
**Scope:** Merchant/admin React frontend only

## Implementation status

| Item | Status |
| --- | --- |
| `api.logout()` → `POST /auth/logout` | Done — `frontend/src/services/api.ts` |
| Layout await + `finally` local clear / navigate | Done — `frontend/src/app/Layout.tsx` |
| Loading / duplicate-click guard | Done |
| Layout tests (success / failure / duplicate click) | Done — `frontend/src/app/Layout.test.tsx` |
| ProLayout test stub `actionsRender` | Done — `frontend/src/test/pro-components-stub.tsx` |
| `npm test` + `npm run build` | Passed (8 tests; stub not in production bundle) |
| Related tracked review docs marked fixed | Done — full-project F-08, deep-review R-08, repair-plan §9.2 |
| Production frontend release | Pending deploy |

## Goal

When a signed-in merchant or administrator clicks logout, the frontend must ask the existing authenticated `POST /api/v1/auth/logout` endpoint to mark the current auth session revoked so its refresh token cannot be reused, before clearing local credentials. A network or server failure must never prevent local logout.

## Non-goals

- No backend route, middleware, token, or session schema changes.
- No buyer miniapp changes; it already follows the required failure-tolerant logout pattern.
- No inventory, migration, production configuration, or deployment changes.
- No new auth service abstraction for a single caller.
- No merchant access-token immediate revocation (F-14 / R-16). This change only guarantees the current session's refresh token is unusable after a successful logout. Admin routes already check active sessions via existing middleware and therefore lose access sooner; that is existing backend behavior, not new work here.
- No change to admin password-change forced re-login (`SecurityPage`); that path already revokes sessions server-side on password update.

## Design

### API boundary

Add `api.logout()` in `frontend/src/services/api.ts`. It uses the existing authenticated Axios client (`http` from `frontend/src/services/http.ts`) and sends:

```text
POST /auth/logout
body: {}
Authorization: Bearer <current access token> (added by the existing interceptor)
```

The client base URL already includes `/api/v1`, matching other `api.*` methods.

The backend already resolves the access-token session ID and sets that session's `revoked_at`, preventing its refresh token from being used again.

Use the shared `http` client with its interceptors. Do **not** add `/auth/logout` to `AUTH_EXEMPT_PATHS`. If the access token is expired but the refresh token is still valid, the existing 401→refresh→retry path can still complete session revocation. Logout failures (network, timeout, business error, or failed refresh) still fall through to local clear in Layout.

Accept the default HTTP timeout for logout; do not introduce a logout-specific shorter timeout in this change.

### Layout behavior

`frontend/src/app/Layout.tsx` owns the merchant/admin logout control and remains the orchestration point.

On click:

1. Ignore duplicate clicks while logout is in progress.
2. Capture the post-logout destination from the current `isAdmin` value before any local clear (`/admin/login` or `/login`).
3. Mark the logout button as loading/disabled.
4. Await `api.logout()` so navigation does not abort the revocation request.
5. In `finally`, clear the local auth store.
6. Navigate to the captured destination.

The local clear and navigation happen on both server success and failure. No error toast is shown because the user-requested local logout still succeeds; exposing a transient revocation failure would not provide an actionable recovery path.

### State and concurrency

The loading state is component-local. It prevents multiple logout requests from rapid clicks (early return in the handler and a loading/disabled button). The destination is captured before local state is cleared, so the correct login page remains available in the `finally` block even if a re-render runs after `clear()`.

## Testing

Add focused Layout tests that use the real `api.logout()` method with the HTTP client mocked at its boundary:

1. Merchant success: assert `POST /auth/logout` with `{}`, retain local credentials while the request is pending, disable/load the button, then clear credentials and navigate to `/login` after resolution.
2. Administrator failure: reject the HTTP request, then assert credentials are still cleared and navigation goes to `/admin/login`.
3. Duplicate click: while the first logout request is in flight, a second click must not issue another `POST /auth/logout`.
4. Extend the existing test-only Pro Components stub with the minimum `ProLayout` rendering needed to expose `actionsRender`; production code continues to use the real package.

Run the focused test, the complete frontend Vitest suite, and `npm run build` under Node `22.22.2`. The production build must not contain the test stub.

## Acceptance Criteria

- A successful logout marks the current auth session revoked (refresh token unusable) before local navigation.
- A failed logout still clears all local auth state and navigates to the appropriate login page.
- Duplicate clicks cannot create concurrent logout requests.
- Existing frontend tests and production build pass.
- Product-code changes are limited to the frontend API and Layout; focused test/stub files and tracked F-08 status/design documents may also change. The three protected untracked review documents remain untouched.
