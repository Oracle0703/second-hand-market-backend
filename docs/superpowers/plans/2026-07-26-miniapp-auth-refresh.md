# F-05 Miniapp Auth Refresh Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make buyer miniapp HTTP 401 and business-code `10002` responses perform one concurrency-safe token refresh, replay each protected request once, and fail closed without resurrecting or clearing a replaced session.

**Architecture:** Keep refresh orchestration inside `miniapp/src/services/request.ts`, before generic non-2xx rejection. Represent refresh completion as `refreshed`, `failed`, or `stale`; persist tokens only when the captured refresh token still owns the current session, and expose a stable `AuthExpiredError` while leaving navigation to existing page/login guards. Verify the source locally and in a separately authorized server directory using a no-database acceptance script.

**Tech Stack:** TypeScript 5.9, Taro 3.6.34, Zustand 5, Vitest 3.2, Node 22.22.2, npm 10.9.7, Bash.

## Global Constraints

- Implement only F-05; do not claim F-12 real-provider identity, F-14 server-side access revocation, or another open finding is closed.
- The request module must not call `Taro.navigateTo`, `Taro.redirectTo`, `Taro.reLaunch`, `Taro.switchTab`, toast, or modal APIs.
- Refresh only protected requests. `skipAuth` requests never refresh and never clear an unrelated session.
- An original request is unauthorized when HTTP status is `401` or a valid API envelope has code `10002`.
- One unauthorized request may be replayed at most once; a replay must never refresh recursively.
- Concurrent requests for the same captured refresh token share exactly one refresh request.
- A delayed refresh response must not update or clear a session whose refresh token changed while the request was pending.
- Never log or persist test tokens, request bodies, refresh payloads, device identifiers, or `AuthExpiredError` details beyond the existing session store contract.
- Do not send requests to production, deploy miniapp bundles, modify production data, or use a database for F-05 acceptance.
- Never modify, stage, or commit `.tmp/`, `docs/architecture-evolution-plan-2026-07-24.md`, `docs/first-round-fix-review-2026-07-24.md`, or `docs/second-round-fix-review-2026-07-24.md`.
- Run every behavior change RED -> GREEN and use focused Conventional Commits.

---

## File Map

- `miniapp/tests/request-refresh.test.ts`: real request/session behavior tests with only the external Taro transport and storage boundary mocked.
- `miniapp/src/services/request.ts`: unauthorized classification, refresh outcome state machine, session ownership checks, one replay, and `AuthExpiredError`.
- `deploy/acceptance/miniapp-auth-refresh-smoke.sh`: guarded Node/npm/test/build verification that makes no application API or database calls.
- `deploy/acceptance/README.md`: exact F-05 isolated server workflow and prohibited actions.
- `Makefile`: guarded `acceptance-miniapp-auth-refresh-smoke` entry point.
- `docs/superpowers/reviews/2026-07-26-miniapp-auth-refresh-isolated-acceptance.md`: sanitized local/remote hashes, commands, results, and non-production statement.
- `docs/full-project-code-review-2026-07-24.md`: append-only dated F-05 status evidence without rewriting the historical finding.
- `docs/release-readiness.md`: distinguish code-side/test-server completion from an unreleased production miniapp.
- `docs/production-hardening-repair-plan-2026-07-24.md`: update the open-finding table after evidence exists.

### Task 1: Prove and fix the reachable single-flight refresh success path

**Files:**
- Create: `miniapp/tests/request-refresh.test.ts`
- Modify: `miniapp/src/services/request.ts`

**Interfaces:**
- Consumes: `Taro.request`, `ensureDeviceID()`, `useSessionStore.getState()`, HTTP status `401`, API code `10002`, and internal `RequestOptions<T>.retrying`.
- Produces: `export class AuthExpiredError extends Error`; internal `type RefreshOutcome = 'refreshed' | 'failed' | 'stale'`; `refreshAccessToken(capturedRefreshToken: string): Promise<RefreshOutcome>`.

- [ ] **Step 1: Create a controlled Taro transport/storage test harness**

In `request-refresh.test.ts`, reset module state after every test and provide real in-memory storage behavior:

```ts
import { afterEach, describe, expect, test, vi } from 'vitest'

type TaroResponse = {
  statusCode: number
  data: unknown
  header: Record<string, string>
}

function response(statusCode: number, code: number, data: unknown = null): TaroResponse {
  return { statusCode, header: {}, data: { code, message: code === 0 ? 'OK' : 'unauthorized', request_id: 'req-test', data } }
}

async function loadRequestModule(request: ReturnType<typeof vi.fn>) {
  const storage = new Map<string, unknown>()
  const taro = {
    request,
    getStorageSync: vi.fn((key: string) => storage.get(key)),
    setStorageSync: vi.fn((key: string, value: unknown) => storage.set(key, value)),
    removeStorageSync: vi.fn((key: string) => storage.delete(key))
  }
  vi.stubGlobal('__DEV_MODE__', false)
  vi.doMock('@tarojs/taro', () => ({ default: taro }))
  const requestModule = await import('../src/services/request')
  const sessionModule = await import('../src/stores/session')
  return { ...requestModule, ...sessionModule, storage, taro }
}

afterEach(() => {
  vi.resetModules()
  vi.unmock('@tarojs/taro')
  vi.unstubAllGlobals()
})
```

The fake's methods cause the real Zustand session methods to persist/remove values; tests assert resulting behavior rather than asserting that a mock exists.

- [ ] **Step 2: Write RED HTTP 401 and business-code tests**

Add one table-driven test for:

```ts
[
  { name: 'HTTP 401', status: 401, code: 10002 },
  { name: '2xx business unauthorized', status: 200, code: 10002 }
]
```

For each case, seed `old-access` / `old-refresh`, make the first protected request return the unauthorized response, make `/buyer/auth/refresh` return `new-access` / `new-refresh`, and make the replay return literal data `{ item_id: 7 }`. Assert:

```ts
expect(result).toEqual({ item_id: 7 })
expect(request).toHaveBeenCalledTimes(3)
expect(request.mock.calls[2][0].header.Authorization).toBe('Bearer new-access')
expect(storage.get('buyer_access_token')).toBe('new-access')
expect(storage.get('buyer_refresh_token')).toBe('new-refresh')
```

Also assert the refresh call has no `Authorization` header and retains the device header.

- [ ] **Step 3: Write the RED concurrent single-flight test**

Use a refresh gate so two protected requests both return 401 before refresh resolves:

```ts
let releaseRefresh!: () => void
const refreshGate = new Promise<void>((resolve) => { releaseRefresh = resolve })
let refreshCalls = 0

request.mockImplementation(async (options: { url: string; header?: Record<string, string> }) => {
  if (options.url.endsWith('/buyer/auth/refresh')) {
    refreshCalls += 1
    await refreshGate
    return response(200, 0, { access_token: 'new-access', refresh_token: 'new-refresh' })
  }
  if (options.header?.Authorization === 'Bearer old-access') return response(401, 10002)
  return response(200, 0, { path: options.url })
})
```

Start two `apiRequest` calls, wait until both original attempts and one refresh are observed, release the gate, and require one refresh, two replays, and two independent return values.

- [ ] **Step 4: Run focused tests and verify RED**

Run:

```bash
cd miniapp
npm test -- --run tests/request-refresh.test.ts
```

Expected: FAIL because `request.ts` throws its generic HTTP status error at HTTP 401 before refresh and does not expose `AuthExpiredError` or the three-state refresh behavior.

- [ ] **Step 5: Implement unauthorized classification and refresh success**

Add:

```ts
export class AuthExpiredError extends Error {
  readonly code = 10002

  constructor() {
    super('登录已过期，请重新登录')
    this.name = 'AuthExpiredError'
  }
}

type RefreshOutcome = 'refreshed' | 'failed' | 'stale'
let refreshingPromise: Promise<RefreshOutcome> | null = null

function isUnauthorized(statusCode: number, payload: unknown): boolean {
  return statusCode === 401 || (isAPIResponse(payload) && payload.code === 10002)
}
```

Change `refreshAccessToken` to accept the captured token, require a 2xx valid success envelope and nonempty string tokens, re-read current session ownership before committing, retain the profile read at commit time, and return the three outcomes. Do not clear in the `stale` branch.

Move unauthorized handling before the generic non-2xx block. On `refreshed`, replay with `{...options, retrying: true}`. Reset `refreshingPromise` in `finally`.

- [ ] **Step 6: Run focused tests and verify GREEN**

Run: `cd miniapp && npm test -- --run tests/request-refresh.test.ts`

Expected: PASS for HTTP 401, business code, replay header, storage rotation, and concurrent single-flight cases.

- [ ] **Step 7: Commit the success path**

```bash
git add miniapp/src/services/request.ts miniapp/tests/request-refresh.test.ts
git commit -m "fix(miniapp): refresh concurrent unauthorized requests"
```

### Task 2: Fail closed without clearing or resurrecting a replaced session

**Files:**
- Modify: `miniapp/tests/request-refresh.test.ts`
- Modify: `miniapp/src/services/request.ts`

**Interfaces:**
- Consumes: `RefreshOutcome`, captured refresh-token ownership, `AuthExpiredError`, and `useSessionStore.clearSession()`.
- Produces: session cleanup only when the captured token still owns the store; stale waiters reject without replay or mutation.

- [ ] **Step 1: Write RED refresh-failure matrix tests**

Add table cases for refresh transport rejection, HTTP 401, malformed envelope, nonzero business code, empty access token, and empty refresh token. For every case require:

```ts
await expect(result).rejects.toMatchObject({ name: 'AuthExpiredError', code: 10002 })
expect(useSessionStore.getState().accessToken).toBe('')
expect(useSessionStore.getState().refreshToken).toBe('')
expect(useSessionStore.getState().profile).toBeUndefined()
expect(storage.has('buyer_access_token')).toBe(false)
expect(storage.has('buyer_refresh_token')).toBe(false)
expect(storage.has('buyer_profile')).toBe(false)
```

For two concurrent waiters on one failing refresh, assert one refresh call and both rejections.

- [ ] **Step 2: Write RED no-token, replay, and skipAuth tests**

Cover these independent contracts:

- An unauthorized protected request with no refresh token clears stale access/profile state, sends no refresh request, and throws `AuthExpiredError`.
- A successful refresh followed by a second 401 sends one refresh total, makes two protected attempts total, clears that session, and throws `AuthExpiredError`.
- A `skipAuth: true` request returning 401 sends no refresh, preserves a seeded unrelated session, and throws the existing generic status error rather than `AuthExpiredError`.

- [ ] **Step 3: Write RED stale-session race tests**

While refresh is gated, test both state changes:

1. Call `clearSession()`, then resolve the old refresh. The old request rejects, session remains empty, and there is no replay.
2. Call `setSession('other-access', 'other-refresh', otherProfile)`, then resolve the old refresh. The old request rejects, the other session/storage remains byte-for-byte unchanged, and there is no replay with `other-access`.

- [ ] **Step 4: Run focused tests and verify RED**

Run: `cd miniapp && npm test -- --run tests/request-refresh.test.ts`

Expected: FAIL in cleanup, second-401, or stale-session cases until failure handling is ownership-aware.

- [ ] **Step 5: Implement ownership-aware failure handling**

Use a helper that only clears the session still owned by the captured refresh token:

```ts
function clearSessionIfOwned(capturedRefreshToken: string): void {
  const current = useSessionStore.getState()
  if (current.refreshToken === capturedRefreshToken) current.clearSession()
}
```

For an original unauthorized request without a refresh token, call the current store's `clearSession()` and throw `AuthExpiredError`. In `refreshAccessToken`, return `stale` without clearing whenever ownership changed; for all other refresh failures call `clearSessionIfOwned` once and return `failed`. For a replay 401, clear only the replay's captured/current session and throw without another refresh.

- [ ] **Step 6: Add ordinary-error regression tests**

Require a 403, a 500, a Taro request rejection, and a malformed 2xx response to retain their existing error categories and never call refresh. These tests protect the placement of the new unauthorized branch.

- [ ] **Step 7: Run focused tests and verify GREEN**

Run: `cd miniapp && npm test -- --run tests/request-refresh.test.ts`

Expected: all success, failure, recursion, skipAuth, stale-session, and ordinary-error cases PASS.

- [ ] **Step 8: Commit failure hardening**

```bash
git add miniapp/src/services/request.ts miniapp/tests/request-refresh.test.ts
git commit -m "fix(miniapp): fail closed when auth refresh expires"
```

### Task 3: Run complete miniapp regressions and both production builds

**Files:**
- Modify only if a verified regression requires it: `miniapp/src/services/request.ts`, `miniapp/tests/request-refresh.test.ts`

**Interfaces:**
- Consumes: the completed F-05 request contract.
- Produces: local test/build evidence under the locked toolchain.

- [ ] **Step 1: Verify the declared toolchain**

Run:

```bash
cd miniapp
node --version
npm --version
```

Expected: `v22.22.2` and `10.9.7`. If the shell uses another version, switch to the repository-supported runtime before interpreting build failures.

- [ ] **Step 2: Run the full miniapp suite**

Run: `cd miniapp && npm test`

Expected: every `tests/**/*.test.ts` file passes, including the new request-refresh suite and existing session startup tests.

- [ ] **Step 3: Build WeChat and Douyin bundles with a non-production base URL**

Run:

```bash
cd miniapp
TARO_APP_API_BASE_URL=https://example.invalid/api/v1 npm run build:weapp
TARO_APP_API_BASE_URL=https://example.invalid/api/v1 npm run build:tt
```

Expected: both commands exit 0. The commands compile bundles but do not run them or call an application API.

- [ ] **Step 4: Verify repository cleanliness**

Run:

```bash
git status --short
git diff --check
```

Expected: build output remains ignored; no lockfile, generated output, protected document, or unrelated source changed.

### Task 4: Add a guarded no-database test-server acceptance harness

**Files:**
- Create: `deploy/acceptance/miniapp-auth-refresh-smoke.sh`
- Modify: `deploy/acceptance/README.md`
- Modify: `Makefile`

**Interfaces:**
- Consumes: `MINIAPP_AUTH_REFRESH_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_RUNS_ONLY_ISOLATED_MINIAPP_TESTS`, Node `v22.22.2`, npm `10.9.7`, and the committed miniapp source.
- Produces: final marker `isolated miniapp auth refresh acceptance passed` and ignored sanitized files under `deploy/acceptance/evidence/miniapp-auth-refresh/`.

- [ ] **Step 1: Create the guarded script**

The script must use `set -euo pipefail`, reject a missing confirmation before `npm ci`, require exact Node/npm versions, set `TARO_APP_API_BASE_URL=https://example.invalid/api/v1`, and run:

```bash
npm ci
npm test -- --run tests/request-refresh.test.ts
npm test
npm run build:weapp
npm run build:tt
```

Capture version/test/build output under the ignored evidence directory, hash the evidence with `sha256sum`, and print the final marker only after every command succeeds. Do not read `.env`, call Docker, accept a DSN, invoke curl, or start a development server.

- [ ] **Step 2: Add the Make target**

Add `.PHONY` and:

```make
acceptance-miniapp-auth-refresh-smoke:
	@test "$${MINIAPP_AUTH_REFRESH_ACCEPTANCE_CONFIRM:-}" = "I_UNDERSTAND_THIS_RUNS_ONLY_ISOLATED_MINIAPP_TESTS" || { echo "set MINIAPP_AUTH_REFRESH_ACCEPTANCE_CONFIRM for isolated miniapp auth refresh tests" >&2; exit 1; }
	./deploy/acceptance/miniapp-auth-refresh-smoke.sh
```

- [ ] **Step 3: Document the isolated workflow**

In `deploy/acceptance/README.md`, document the exact confirmation variable, required Node/npm versions, evidence path, `example.invalid` build base URL, absence of database/API calls, and retained source directory. State explicitly that this is test-server review, not a production miniapp release.

- [ ] **Step 4: Verify script syntax and fail-closed guard**

Run:

```bash
bash -n deploy/acceptance/miniapp-auth-refresh-smoke.sh
make acceptance-miniapp-auth-refresh-smoke
```

Expected: syntax exits 0; the unconfirmed Make invocation exits nonzero before `npm ci` with the confirmation message.

- [ ] **Step 5: Commit the harness**

```bash
git add deploy/acceptance/miniapp-auth-refresh-smoke.sh deploy/acceptance/README.md Makefile
git commit -m "test(acceptance): verify miniapp auth refresh"
```

### Task 5: Obtain scoped authorization and run test-server acceptance

**Files:**
- No source edits before authorization.
- Evidence generated remotely and copied back only after sanitization: `deploy/acceptance/evidence/miniapp-auth-refresh/`.

**Interfaces:**
- Consumes: exact committed whitelist and explicit user authorization.
- Produces: matching local/remote source SHA-256 values and a successful isolated test-server evidence set.

- [ ] **Step 1: Request exact transfer authorization**

Obtain authorization for remote path:

```text
/home/yu/services/secondhand-miniapp-auth-refresh-acceptance-20260726
```

Whitelist only tracked source required by the harness:

```text
miniapp/babel.config.js
miniapp/config/
miniapp/package.json
miniapp/package-lock.json
miniapp/project.config.json
miniapp/project.tt.json
miniapp/src/
miniapp/tests/
miniapp/tsconfig.json
miniapp/vitest.config.mjs
deploy/acceptance/miniapp-auth-refresh-smoke.sh
Makefile
```

Explicitly exclude `.env`, secrets, databases, uploads, evidence, `.git`, caches, `node_modules`, `miniapp/dist`, `miniapp/project.private.config.json`, `backend/app.db`, and the three protected review documents. Earlier F-02 or F-04/F-13 authorizations do not authorize this new path or whitelist.

- [ ] **Step 2: Transfer only the authorized committed whitelist**

Create local and remote SHA-256 manifests over the transferred regular files, normalize only the manifest's directory prefix, and require exact equality before execution. Do not transfer local dependencies or generated output.

- [ ] **Step 3: Run the guarded target only in the isolated directory**

Run:

```bash
MINIAPP_AUTH_REFRESH_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_RUNS_ONLY_ISOLATED_MINIAPP_TESTS \
make acceptance-miniapp-auth-refresh-smoke
```

Expected final line: `isolated miniapp auth refresh acceptance passed`.

- [ ] **Step 4: Verify non-production scope and retain review artifacts**

Confirm the script used `example.invalid`, did not receive environment credentials or a DSN, did not start a process that serves traffic, and did not contact the production application API. Retain the isolated source directory and sanitized evidence for review; do not deploy either built bundle.

### Task 6: Record evidence, update F-05 status, and pass final review gates

**Files:**
- Create: `docs/superpowers/reviews/2026-07-26-miniapp-auth-refresh-isolated-acceptance.md`
- Modify: `docs/full-project-code-review-2026-07-24.md`
- Modify: `docs/release-readiness.md`
- Modify: `docs/production-hardening-repair-plan-2026-07-24.md`

**Interfaces:**
- Consumes: exact commit SHAs, local/remote source hashes, Node/npm versions, focused/full test output, two build outputs, evidence hashes, and retained remote path.
- Produces: authoritative status distinguishing `code-side closed`, `isolated test-server review passed`, and `production miniapp not released`.

- [ ] **Step 1: Write the tracked acceptance report**

Record the design/plan paths, branch and commit range, source-hash equality, toolchain versions, exact commands and test counts, both build results, sanitized evidence hashes, retained remote path, and explicit non-actions. Do not include IP addresses, usernames, tokens, `.env` values, request payloads, or raw storage contents.

- [ ] **Step 2: Append dated status evidence**

Append a `2026-07-26 F-05 follow-up verification` section to `docs/full-project-code-review-2026-07-24.md` without rewriting the original finding. Update readiness/hardening status only after evidence exists. State that F-05 is code-side closed and passed isolated test-server review, while the production miniapp remains unreleased and F-12/F-14 remain open.

- [ ] **Step 3: Run complete local verification**

Run:

```bash
(cd miniapp && npm test)
(cd miniapp && TARO_APP_API_BASE_URL=https://example.invalid/api/v1 npm run build:weapp)
(cd miniapp && TARO_APP_API_BASE_URL=https://example.invalid/api/v1 npm run build:tt)
bash -n deploy/acceptance/miniapp-auth-refresh-smoke.sh
git diff --check
```

Expected: every command exits 0. If a failure appears, use `superpowers:systematic-debugging`, add a focused regression test, and rerun the complete gate.

- [ ] **Step 4: Perform specification and code-quality reviews**

Use `superpowers:requesting-code-review` for two passes:

1. Specification compliance against `docs/superpowers/specs/2026-07-26-miniapp-auth-refresh-design.md` and this plan.
2. Code quality/security review focused on refresh ordering, single-flight behavior, replay count, session ownership, token leakage, mock fidelity, and evidence claims.

Resolve every High/Medium issue and rerun focused/full gates before marking F-05 closed.

- [ ] **Step 5: Audit scope before the documentation commit**

Run:

```bash
git status --short
git diff --name-only HEAD
git diff --check
```

Confirm no `.env`, credential, database, upload, evidence directory, `.git`, cache, `node_modules`, build output, `.tmp/`, or protected review document is staged.

- [ ] **Step 6: Commit documentation and evidence**

```bash
git add docs/superpowers/reviews/2026-07-26-miniapp-auth-refresh-isolated-acceptance.md \
  docs/full-project-code-review-2026-07-24.md docs/release-readiness.md \
  docs/production-hardening-repair-plan-2026-07-24.md
git commit -m "docs(acceptance): record miniapp auth refresh evidence"
```

- [ ] **Step 7: Verify the review-ready F-05 range**

Run:

```bash
git status --short --branch
git log --oneline --decorate -10
git diff --stat 9a29763^..HEAD
git diff --check 9a29763^..HEAD
```

Expected: only the known untracked `.tmp/` and three protected documents remain; all F-05 source, tests, harness, and evidence documents are committed on `codex/reconcile-code-reviews`; no production action is represented as completed.

## Plan Self-Review Record

- Spec coverage: Tasks 1-2 cover every refresh success/failure/concurrency/session-ownership rule; Task 3 covers the complete local matrix; Tasks 4-5 cover reproducible authorized test-server review; Task 6 covers status precision and final reviews.
- Placeholder scan: no deferred implementation markers or unspecified error-handling steps remain.
- Type consistency: `AuthExpiredError`, `RefreshOutcome`, and `refreshAccessToken(capturedRefreshToken: string)` keep the same signatures across tests and implementation tasks.
- Concurrency check: changed sessions produce `stale`, are neither cleared nor overwritten, and old requests never replay under a new buyer identity.
- Scope check: F-12 and F-14 remain explicitly open; F-05 has no database, production API, deployment, or navigation changes.
