# F-05 Miniapp 401 Token Refresh Design

**Date:** 2026-07-26

**Branch:** `codex/reconcile-code-reviews`

**Status:** Design approved; implementation not started

**Finding:** F-05 - the miniapp throws on HTTP 401 before its token-refresh branch can run

## 1. Goal

Make authenticated buyer miniapp requests recover from an expired access token by refreshing exactly once for a concurrent group of unauthorized responses and replaying each original request once with the new access token.

When refresh cannot establish a valid session, the request layer must clear all buyer session credentials and throw one stable authentication-expired error. Navigation remains an upper-layer responsibility, as approved; the HTTP module must not manipulate the Taro page stack.

This design applies equally to WeChat and Douyin builds.

## 2. Non-goals

- Do not change backend access-token or refresh-token formats, TTLs, routes, or session schema.
- Do not implement F-12 real-provider login or migrate existing mock buyer identities.
- Do not implement F-14 server-side access-token revocation checks.
- Do not add navigation, toast, modal, or page-state behavior to the request module.
- Do not retry ordinary HTTP failures, business errors, malformed success responses, or network errors.
- Do not send requests to production, mutate production data, or persist test credentials.

## 3. Approaches Considered

### 3.1 Adopted: request-layer single-flight refresh, upper-layer navigation

`miniapp/src/services/request.ts` detects unauthorized responses before generic HTTP rejection, owns refresh serialization and one replay, clears invalid session state, and throws an exported `AuthExpiredError`. Pages and the existing login guard remain responsible for deciding when and where to navigate.

This keeps authentication transport policy in one place without coupling a reusable service module to the current Taro route or causing concurrent requests to issue duplicate navigation calls.

### 3.2 Rejected: navigate directly from the request module

Direct `Taro.navigateTo`, `redirectTo`, or `reLaunch` calls would couple transport behavior to page-stack policy. Concurrent failed requests could also race to navigate, lose the intended return path, or exceed page-stack limits.

### 3.3 Rejected: refresh independently in each page or service

Caller-owned refresh duplicates token logic across queries and mutations, cannot reliably serialize concurrent 401 responses, and makes infinite-retry prevention dependent on every caller implementing the same rule correctly.

## 4. Request and Response Model

The existing `apiRequest<T>` remains the only application request entry point. `RequestOptions<T>` keeps its internal `retrying` marker; callers do not set it directly.

An original response is unauthorized when either condition is true:

```text
HTTP status == 401
or
payload is a valid APIResponse and payload.code == 10002
```

Unauthorized detection occurs after `Taro.request` resolves but before the generic non-2xx status error. This ordering makes the backend's HTTP 401 / business code `10002` contract reachable while retaining existing diagnostics for other non-2xx responses.

Refresh is eligible only when all conditions hold:

- the request is not `skipAuth`;
- the request is not already a replay (`retrying !== true`);
- the current session has a nonempty refresh token.

The refresh endpoint continues to use `Taro.request` directly rather than `apiRequest`, so a refresh 401 cannot recurse into another refresh.

## 5. Refresh State Machine

The module retains one process-local `refreshingPromise`. The first eligible unauthorized request creates it; every other eligible unauthorized request awaits the same promise. The promise resolves to an internal outcome of `refreshed`, `failed`, or `stale` rather than a boolean so a changed session cannot be mistaken for an invalid current session.

The refresh operation captures the refresh token used for the request. A response may update the session only when all of these remain true:

- HTTP status is 2xx;
- the payload is a valid API envelope with `code == 0`;
- both returned tokens are nonempty strings;
- the store's current refresh token still equals the captured token.

The final equality check prevents a delayed refresh response from restoring a session that the user cleared or replaced while refresh was in flight.

On success, the store persists the returned access and refresh tokens while retaining the buyer profile read from the store at commit time. Each waiter then calls `apiRequest` once with the original method, path, data, and `skipAuth` value plus `retrying: true`. The replay builds headers from the current store, so it sends the new access token rather than the stale token captured by the original call.

If the current refresh token differs from the captured token, the outcome is `stale`. Old waiters throw `AuthExpiredError` without replaying and without modifying the current session. This prevents an old request from running under a newly logged-in buyer identity and prevents an old refresh from clearing or overwriting that newer session.

The single-flight promise is reset in `finally` after all waiters have received its result, allowing a later independent expiration event to start one new refresh.

## 6. Failure Semantics

Export a narrow error type from the request module:

```ts
export class AuthExpiredError extends Error {
  readonly code = 10002
}
```

Its message is always `登录已过期，请重新登录`. It must not contain access tokens, refresh tokens, device identifiers, request bodies, platform response bodies, or URLs.

The request layer clears the session and throws `AuthExpiredError` when any of these occurs:

- an authenticated request is unauthorized and no refresh token exists;
- refresh throws a network/runtime error;
- refresh returns non-2xx;
- refresh returns a malformed API envelope;
- refresh returns a nonzero business code;
- refresh succeeds structurally but returns an empty token;
- the replay is unauthorized again.

The refresh operation performs the shared clear once for a failed concurrent group, but only when the store still contains the captured refresh token. Individual waiters then throw their own `AuthExpiredError` without navigation. A `stale` outcome also throws that error but leaves the changed session intact. A replay that fails authorization clears the currently invalid session before throwing.

`skipAuth` requests never refresh or clear an unrelated existing session. Their 401 and business errors follow the ordinary request error path. Other HTTP non-2xx responses retain the existing status diagnostic, and malformed 2xx responses retain the existing malformed-response diagnostic.

## 7. Navigation Boundary

The HTTP layer does not call any Taro navigation API. Clearing the store makes `isLoggedIn()` false, so the existing `requireLoginFor(targetPath)` guard sends the next protected user action to `/pages/login/index` with its intended redirect path.

Pages may identify `AuthExpiredError` to suppress duplicate generic error text or provide page-specific presentation, but F-05 does not introduce a global redirect bus or change page layouts. Immediate forced navigation is explicitly outside the approved boundary.

## 8. Logging and Storage Safety

- Development logs may retain method, URL, HTTP status, response headers, and the existing payload summary only under `__DEV_MODE__`; the refresh change must not add token logging.
- Session updates continue through `useSessionStore.setSession`; failure cleanup continues through `clearSession` so storage keys and memory state change together.
- No refresh promise, request payload, token, or authentication error is persisted outside the existing session store.
- Test tokens are synthetic literals and must not be copied into documentation evidence.

## 9. Test Strategy

Add `miniapp/tests/request-refresh.test.ts` using a controlled `@tarojs/taro` request adapter and isolated module state. Tests exercise the real `apiRequest`, session store, and storage side effects; the mock is limited to the external Taro transport/storage boundary.

Required RED -> GREEN cases:

1. HTTP 401 / code `10002` refreshes once and replays with the returned access token.
2. A 2xx response with business code `10002` follows the same refresh path.
3. Two concurrent unauthorized requests share exactly one refresh and both replay with the new token.
4. Refresh network failure clears access, refresh, and profile storage and throws `AuthExpiredError` to every waiter.
5. Missing refresh token clears stale session state and does not call the refresh endpoint.
6. Malformed, non-2xx, nonzero-code, or empty-token refresh responses fail closed.
7. A replay that is unauthorized does not recurse, clears the session, and makes exactly one refresh attempt.
8. A `skipAuth` 401 does not refresh or clear an unrelated session.
9. Clearing or replacing the session while refresh is pending prevents the delayed response from restoring the captured session; replacing it also proves the newer session is not cleared and the old request is not replayed under the new identity.
10. Ordinary 403/500, network failure, and malformed 2xx response behavior remains unchanged.

Run:

```bash
cd miniapp
npm test
npm run build:weapp
npm run build:tt
```

Both platform builds must use the locked Node/npm toolchain declared in `miniapp/package.json` and must not modify tracked generated artifacts.

## 10. Test-Server Acceptance

F-05 has no database migration and does not require production or MySQL access. Test-server review uses an isolated checkout of the committed whitelist and runs the same full miniapp test suite plus WeChat and Douyin production builds. It must record:

- branch and exact commit SHA;
- local and remote source SHA-256 equality;
- Node and npm versions;
- focused refresh test count and result;
- full miniapp test result;
- both build exit codes;
- a statement that no production API request, deployment, credential transfer, or data mutation occurred.

Any source transfer requires separate authorization for its exact remote path and whitelist. `.env`, credentials, storage data, build output, caches, `node_modules`, `.git`, evidence directories, `backend/app.db`, and the three protected review documents are excluded.

## 11. Acceptance Criteria

- HTTP 401 reaches the refresh path before generic non-2xx rejection.
- One concurrent unauthorized group issues exactly one refresh request.
- Every successful waiter replays once with the new access token and receives its own original response data.
- Refresh failure or a second unauthorized response clears all buyer session credentials and throws `AuthExpiredError`.
- A stale refresh response cannot resurrect a cleared or replaced session.
- Public/`skipAuth` requests and non-authentication error contracts do not regress.
- Full miniapp tests and both platform builds pass locally and on the authorized test server.
- F-05 status distinguishes code-side closure, test-server approval, and production release; it is not marked fixed before implementation and verification evidence exist.
