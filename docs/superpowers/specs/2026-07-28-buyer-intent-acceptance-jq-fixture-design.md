# F-11 Buyer Intent Acceptance jq Fixture Design

**Date:** 2026-07-28

**Branch:** `codex/f11-buyer-intent-open-uniqueness`

**Status:** Scheme A selected under the approved direct isolated-test debugging plan; implementation pending

## Problem And Evidence

The isolated MySQL 8.4 acceptance at commit `8dbd620` passed the migration chain and both
`TestBuyerIntentMySQLAcceptance` modes, then failed at `go test ./... -count=1`.
An exact rerun with the same committed read-only source mount and the same 768 MiB/0.75 CPU
limits showed that every package except `backend/migrations` passed. The failing acceptance
contract tests all exited before their intended branch because the synthetic harness inherited
an ambient `PATH` without `jq`:

```text
required command is unavailable: jq
```

The missing evidence paths and signal-test timeouts were downstream symptoms of that early
exit. The production acceptance script is correct to require and execute `jq -e`; the defect is
that its synthetic test fixture does not declare this external dependency.

## Goals

- Make the synthetic acceptance harness independent of the developer or container host's
  installed `jq` binary.
- Preserve the real acceptance script's fail-closed `jq -e` structure gate unchanged.
- Restore the cleanup, production-snapshot, and signal contract tests to the branches they are
  designed to exercise.
- Keep all production resources, data, configuration, migrations, and deployments unchanged.

## Non-Goals

- Do not install `jq` in the Go build image or change the production image.
- Do not remove, weaken, or bypass `jq` in a real acceptance run.
- Do not modify migration SQL, application runtime behavior, Compose configuration, evidence,
  secrets, or production Docker resources.
- Do not read protected review documents or retained evidence.

## Options

### Scheme A: Fixture-owned jq success boundary (selected)

`prepareBuyerIntentAcceptanceHarness` creates an executable `jq` stub beside its existing
synthetic `docker` command. The stub exits zero and represents the test precondition that the
hard-coded synthetic Compose JSON is valid. These tests then continue to exercise their actual
subjects: evidence publication, production snapshot handling, and signal propagation.

This keeps the unit boundary explicit and hermetic. Real JSON evaluation remains covered by the
isolated server acceptance, which runs the unchanged script with the host's real `jq`.

### Scheme B: Install jq in the build stage

This would make the tests pass but would add a network/package dependency and enlarge every
builder for a test-only need. It does not make the fixture contract explicit and is rejected.

### Scheme C: Remove the jq preflight or structure check

This would weaken the source/secret mount isolation gate and is rejected.

## Test Boundary And Data Flow

1. The Go test builds a temporary repository and writes the complete synthetic Compose JSON
   through its existing Docker stub.
2. The fixture-owned `jq` executable returns success for that known-valid boundary.
3. The copied real acceptance script advances to the branch selected by each test's Docker,
   grep, tar, signal, or evidence stub.
4. No production code, database, secret, or retained evidence is read or changed.

The fixture must add the stub only to the temporary directory already prepended to `PATH`; it
must not mutate the process-wide environment or invoke a real Docker daemon.

## Failure Semantics

- If the fixture cannot write the executable stub, setup fails immediately through `t.Fatal`.
- The stub must not emit JSON, arguments, environment values, or identifiers.
- A failure in the test's intended branch remains visible and is not converted into success.
- Real acceptance still fails closed if `jq` is absent or its expression returns nonzero.

## Verification

Use the existing remote failure as RED. After implementation:

1. Run the five previously failing `TestBuyerIntentOpenUniquenessAcceptance...` tests locally.
2. Run `go test ./migrations -count=1` locally.
3. Rerun the same five tests against the retained isolated server image and read-only committed
   source mount.
4. Rerun `go test ./... -count=1` under the same server resource limits.
5. Run `git diff --check`, regenerate the committed whitelist and manifest, and obtain an
   independent read-only review before another full acceptance attempt.

## Rollback

The change is test-only and can be reverted by reverting its focused commit. No schema, runtime,
remote database, or production rollback is required.

## Traceability

- Source defect: `backend/migrations/buyer_intent_open_uniqueness_migration_test.go`
- Real safety gate retained: `deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh`
- Parent approved execution plan:
  `docs/superpowers/plans/2026-07-28-buyer-intent-acceptance-resource-identity.md`, Task 2 Step 5
- Remote RED: `backend/migrations` failed while all other packages passed at source commit
  `8dbd620`.
