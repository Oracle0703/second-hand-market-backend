# Frontend Server Logout Design

**Date:** 2026-07-26
**Finding:** F-08 - frontend logout clears only local credentials
**Scope:** Merchant/admin React frontend only

## Goal

When a signed-in merchant or administrator clicks logout, the frontend must ask the existing authenticated `POST /api/v1/auth/logout` endpoint to revoke the current server session before clearing local credentials. A network or server failure must never prevent local logout.

## Non-goals

- No backend route, middleware, token, or session schema changes.
- No buyer miniapp changes; it already follows the required failure-tolerant logout pattern.
- No inventory, migration, production configuration, or deployment changes.
- No new auth service abstraction for a single caller.

## Design

### API boundary

Add `api.logout()` in `frontend/src/services/api.ts`. It uses the existing authenticated Axios client and sends:

```text
POST /auth/logout
body: {}
Authorization: Bearer <current access token> (added by the existing interceptor)
```

The backend already resolves the access-token session ID and sets that session's `revoked_at`, preventing its refresh token from being used again.

### Layout behavior

`frontend/src/app/Layout.tsx` owns the merchant/admin logout control and remains the orchestration point.

On click:

1. Ignore duplicate clicks while logout is in progress.
2. Mark the logout button as loading.
3. Await `api.logout()` so navigation does not abort the revocation request.
4. In `finally`, clear the local auth store.
5. Navigate administrators to `/admin/login` and merchants to `/login`.

The local clear and navigation happen on both server success and failure. No error toast is shown because the user-requested local logout still succeeds; exposing a transient revocation failure would not provide an actionable recovery path.

### State and concurrency

The loading state is component-local. It prevents multiple logout requests from rapid clicks. The existing `isAdmin` value is computed before local state is cleared, so the correct destination remains available in the `finally` block.

## Testing

Add focused Layout tests that use the real `api.logout()` method with the HTTP client mocked at its boundary:

1. Merchant success: assert `POST /auth/logout` with `{}`, retain local credentials while the request is pending, disable/load the button, then clear credentials and navigate to `/login` after resolution.
2. Administrator failure: reject the HTTP request, then assert credentials are still cleared and navigation goes to `/admin/login`.
3. Extend the existing test-only Pro Components stub with the minimum `ProLayout` rendering needed to expose `actionsRender`; production code continues to use the real package.

Run the focused test, the complete frontend Vitest suite, and `npm run build` under Node `22.22.2`. The production build must not contain the test stub.

## Acceptance Criteria

- A successful logout revokes the server session before local navigation.
- A failed logout still clears all local auth state and navigates to the appropriate login page.
- Duplicate clicks cannot create concurrent logout requests.
- Existing frontend tests and production build pass.
- Only frontend API, Layout, and test files change; protected review documents remain untouched.
