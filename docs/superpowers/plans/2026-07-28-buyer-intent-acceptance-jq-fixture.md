# F-11 Buyer Intent Acceptance jq Fixture Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the F-11 synthetic acceptance contract tests hermetic when the test host does not install `jq`, without changing the real acceptance safety gate.

**Architecture:** The temporary harness already owns a synthetic `docker` executable and a complete known-valid Compose JSON response. Add a sibling test-only `jq` success stub to that same controlled command boundary; leave the copied production acceptance script and its real `jq -e` invocation unchanged.

**Tech Stack:** Go 1.22 testing, Bash, Docker, Git.

## Global Constraints

- Modify only `backend/migrations/buyer_intent_open_uniqueness_migration_test.go` plus this issue's design and implementation documents.
- Do not modify the real `jq` requirement or structure expression in `deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh`.
- Do not add packages to `backend/Dockerfile` or any production/runtime image.
- Use the existing remote failure at commit `8dbd620` as RED: `backend/migrations` failed with `required command is unavailable: jq` while every other package passed.
- The fixture stub must be executable, silent, return zero, and live only in the temporary directory already prepended to the child process `PATH`.
- Do not read or modify `.env`, secrets, databases, uploads, backups, evidence, `.git`, caches, `node_modules`, `backend/app.db`, `.tmp`, protected review documents, or any production resource.
- Remote verification may operate only the exact isolated project `secondhand-buyer-intent-acceptance` under `/home/yu/services/secondhand-buyer-intent-acceptance-20260727`.

---

### Task 1: Declare jq At The Synthetic Harness Boundary

**Files:**
- Modify: `backend/migrations/buyer_intent_open_uniqueness_migration_test.go`
- Verify: `deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh` remains byte-unchanged

**Interfaces:**
- Consumes: `prepareBuyerIntentAcceptanceHarness(t *testing.T, dockerStub string) (string, []string)`.
- Produces: a temporary child-process `PATH` whose first directory contains both the existing synthetic `docker` command and an executable, silent `jq` success stub.

- [ ] **Step 1: Record RED from the exact isolated environment**

Retain this already captured failure as the test-first proof:

```text
go test ./... -count=1
backend/migrations: FAIL
required command is unavailable: jq
```

The exact same mounted source and resource limits produced passing results for every other
package. Do not rerun the full acceptance harness to recreate RED.

- [ ] **Step 2: Add the minimal fixture dependency**

Immediately after writing the existing synthetic `docker` executable in
`prepareBuyerIntentAcceptanceHarness`, write this sibling command:

```go
if err := os.WriteFile(filepath.Join(stubDir, "jq"), []byte(
	"#!/bin/sh\nexit 0\n",
), 0o700); err != nil {
	t.Fatal(err)
}
```

Do not change `buyerIntentAcceptanceComposeConfigStub`, the returned environment, the real
acceptance script, or any production file.

- [ ] **Step 3: Run focused local GREEN**

```bash
cd backend
GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" \
  go test ./migrations -run '^(TestBuyerIntentOpenUniquenessAcceptanceDoesNotRetainLeakedStagedEvidence|TestBuyerIntentOpenUniquenessAcceptanceFailsClosedWhenEvidenceScanErrors|TestBuyerIntentOpenUniquenessAcceptanceDistinguishesContainerAbsenceFromInspectFailure|TestBuyerIntentOpenUniquenessAcceptanceSnapshotsWithFormattedInspectOnly|TestBuyerIntentOpenUniquenessAcceptanceSignalsExitNonzero)$' -count=1 -v
```

Expected: all selected tests and subtests pass.

- [ ] **Step 4: Run package and full local GREEN**

```bash
cd backend
GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" \
  go test ./migrations -count=1
GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" \
  go test ./... -count=1
```

Expected: all packages pass, with opt-in MySQL tests skipped locally.

- [ ] **Step 5: Run focused and full remote GREEN against the retained test project**

Regenerate the temporary read-only context from `BUYER_INTENT_SOURCE_LIST_ONLY=1`; do not mount
the remote `.env`, secrets, backups, or evidence. Using the isolated builder image and the same
768 MiB/0.75 CPU limits, run the focused regex from Step 3, then:

```bash
go test ./... -count=1
```

Expected: the five former failures pass and the complete package set exits zero. Do not read
retained evidence and do not touch production containers.

- [ ] **Step 6: Verify scope and commit**

```bash
gofmt -w backend/migrations/buyer_intent_open_uniqueness_migration_test.go
git diff --check
git diff --exit-code 8dbd620 -- deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh
git add backend/migrations/buyer_intent_open_uniqueness_migration_test.go
git commit -m "test(acceptance): isolate jq fixture dependency"
```

Regenerate the committed source list and SHA-256 manifest at the new HEAD. Require every path to
be committed, zero forbidden paths, zero symlinks after transfer, and byte-identical local/remote
manifests before any later full acceptance run.

- [ ] **Step 7: Obtain an independent read-only review**

Review the design, this plan, the implementation report, and the diff from `9d0ac76` to the
implementation HEAD. Require spec compliance and code-quality approval with no Critical or
Important findings before rerunning the full acceptance harness.

---

## Traceability

| Requirement | Evidence |
| --- | --- |
| Host-independent synthetic harness | Fixture-owned `jq` executable in the temporary stub directory |
| Real safety gate unchanged | Byte comparison of the acceptance script against `8dbd620` |
| RED before implementation | Exact isolated `backend/migrations` failures at `8dbd620` |
| Local regression closure | Focused, package, and full Go test commands |
| Server regression closure | Same focused regex and `go test ./...` with the committed read-only mount |
| No production changes | Test-project-only container plus unchanged authorized production snapshots in the later full acceptance |
