# Acceptance Provenance Capability Addendum Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the final path-replacement, mutable-input, and ambiguous-publication findings in all four provenance harnesses without adding a runtime dependency or overstating Bash guarantees.

**Architecture:** Bind acquired directories through cwd and device/inode identity, publish evidence by validating the final directory while an owned lock remains, and treat lock release as the commit point. Docker consumes only verified private Compose and descriptor-backed `.env` snapshots; every ambiguity fails closed and retains its lock.

**Tech Stack:** Bash 3.2-compatible shell, Go 1.22 contract tests, Git archive plumbing, Docker Compose command stubs, POSIX file descriptors, SHA-256.

## Execution Status (2026-07-29)

- F-04/F-13 capability-publication work is **paused and handed off** at the
  user's direction. It is not code-side fixed by this addendum and is not
  test-server approved.
- The exact partial state, passing focused test, open finding, and continuation
  commands are recorded in
  `docs/superpowers/reviews/2026-07-29-f04-capability-publication-handoff.md`.
- Do not mark Task 3 or the integrated gates complete from the focused result.
  No SSH, remote Docker, server database, production deployment, migration, or
  production-data change occurred during this attempt.

## Global Constraints

- Follow `docs/superpowers/specs/2026-07-28-acceptance-provenance-capability-addendum.md` and its parent design.
- Modify only the four provenance harness scripts and their four Go contract files until the code review is clean.
- Do not add or source a shared runtime helper and do not change package whitelists or application behavior.
- Do not read, modify, stage, commit, or transfer any protected review document, `.tmp`, `backend/app.db`, `.env`, secret, database, upload, backup, cache, `node_modules`, existing evidence, or `miniapp/project.private.config.json`.
- Do not use SSH, transfer source, run remote Docker, inspect a server, or perform any production action.
- Every behavior change requires a real-script RED, the minimal GREEN implementation, and a mutation-capable assertion.
- Run `gofmt` on Go tests, `bash -n` on every shell script, and `git diff --check` before committing.

---

### Task 1: Bind Export Acquisition And Cleanup

**Files:**
- Modify: `backend/tests/license_file_privacy_acceptance_contract_test.go`
- Modify: `backend/tests/miniapp_auth_refresh_acceptance_contract_test.go`
- Modify: `backend/tests/anonymous_upload_governance_acceptance_contract_test.go`
- Modify: `backend/tests/session_revocation_acceptance_contract_test.go`
- Modify: the four matching `deploy/acceptance/*-smoke.sh` scripts

**Interfaces:**
- Consumes: each existing `<PREFIX>_SOURCE_EXPORT_DIR` mode.
- Produces: four unchanged package artifacts or a failed, non-destructive retained destination.

- [ ] **Step 1: Add acquisition and cleanup RED cases**

Run each real exporter with wrappers that replace the destination during its first identity observation and immediately before failure cleanup. Require a nonzero exit and require persistent external marker files to remain byte-identical.

- [ ] **Step 2: Capture RED**

```bash
cd backend
go test ./tests -run 'AcceptanceSourceExport.*(Identity|Cleanup|Replacement)' -count=1
```

Expected: at least one harness writes through or recursively cleans a replacement.

- [ ] **Step 3: Implement capability-relative export**

Enter and bind the destination parent, create and immediately enter the child, derive its identity from `.`, and keep artifact writes and bounded cleanup relative to the child capability. Never recursively delete through the requested destination pathname; retain state on identity ambiguity.

- [ ] **Step 4: Capture GREEN**

Run the Step 2 command and all four existing source-export test groups. Expected: all pass and every external marker remains unchanged.

### Task 2: Replace Rename Publication With Lock Commit

**Files:** the same eight code and contract files.

**Interfaces:**
- Consumes: an already schema-validated sanitized evidence candidate.
- Produces: a validated final evidence directory with no lock, or an ambiguous final directory with its lock retained.

- [ ] **Step 1: Add publication RED cases**

Exercise evidence-parent replacement, lock replacement, final-target symlink or directory collision, signal delivery during publication, and a nonzero lock-release result. Require external markers to survive; any uncertain path must retain the final directory and lock and return nonzero or preserve `130`/`143`.

- [ ] **Step 2: Capture RED**

```bash
cd backend
go test ./tests -run 'Acceptance.*(Evidence|Publication|Signal).*Replacement' -count=1
```

Expected: staging/rename cleanup follows at least one mutable child path or removes an ambiguity lock.

- [ ] **Step 3: Implement lock-as-commit publication**

Acquire and bind the lock, create and bind the absent final directory with `mkdir`, copy and validate through the final cwd, and remove the owned empty lock only as the final commit operation. Remove staging rename and every recursive cleanup through lock, staging, or final pathnames.

- [ ] **Step 4: Capture GREEN**

Run the Step 2 command plus the complete evidence/reuse groups for all four harnesses. Expected: all pass; a mutation deleting the retained-lock branch makes at least one test fail.

### Task 3: Snapshot Compose And `.env` Inputs

**Files:**
- Modify the F-04/F-13, F-06, and F-14 scripts and matching Go contracts.

**Interfaces:**
- Consumes: verified build-context `docker-compose.yml` and received remote-only `.env`.
- Produces: Compose arguments containing only private runtime paths.

- [ ] **Step 1: Add mutable-input RED cases**

After package and received-file validation, replace received `docker-compose.yml`; during `.env` copy, replace and restore the source and separately mutate the same inode. Record Compose arguments and require that no `-f` or `--env-file` value points into the received tree.

- [ ] **Step 2: Capture RED**

```bash
cd backend
go test ./tests -run 'Acceptance.*(Compose|Env).*(Snapshot|Replacement|ABA)' -count=1
```

Expected: a received path is consumed or F-06 accepts the copy-time mutation.

- [ ] **Step 3: Implement descriptor-backed snapshots**

Use the verified private build-context Compose file. Open `.env` through two independent fixed read-only descriptors, require both to match the prevalidated regular-file identity, record both descriptor signatures, copy them to separate mode-`0600` runtime candidates, and require stable signatures plus byte-identical candidates. Validate one candidate and make it the only `--env-file`; remove the duplicate.

- [ ] **Step 4: Capture GREEN**

Run the Step 2 command and all three Docker harness preflight suites. Expected: all pass and Docker is never reached on a detected mutation.

### Task 4: Run Integrated Gates And Fixed-Range Review

**Files:** the eight implementation/test files; no status document changes in the code commit.

**Interfaces:**
- Consumes: Tasks 1-3 GREEN implementation.
- Produces: one focused code commit and a fixed-range independent review result.

- [ ] **Step 1: Run all four complete harness suites and migration chain**

```bash
cd backend
go test ./tests -run '^(TestLicenseFilePrivacyAcceptance|TestMiniappAuthRefreshAcceptance|TestAnonymousUploadGovernanceAcceptance|TestSessionRevocationAcceptance|TestSessionCurrentMigrationChain)' -count=1
```

- [ ] **Step 2: Run repository gates**

```bash
cd backend
go test ./... -count=1
go test -race ./internal/app ./tests -count=1
go vet ./...
```

- [ ] **Step 3: Run static and scope gates**

Run `bash -n` for all four scripts, `gofmt -d` for all four Go contracts, `git diff --check`, and inspect the exact changed/staged paths against the Global Constraints.

- [ ] **Step 4: Commit only the eight code/test files**

```bash
git add backend/tests/anonymous_upload_governance_acceptance_contract_test.go \
  backend/tests/license_file_privacy_acceptance_contract_test.go \
  backend/tests/miniapp_auth_refresh_acceptance_contract_test.go \
  backend/tests/session_revocation_acceptance_contract_test.go \
  deploy/acceptance/anonymous-upload-governance-smoke.sh \
  deploy/acceptance/license-file-privacy-smoke.sh \
  deploy/acceptance/miniapp-auth-refresh-smoke.sh \
  deploy/acceptance/session-revocation-smoke.sh
git commit -m "fix(acceptance): bind publication capabilities"
```

- [ ] **Step 5: Review the fixed commit range**

Generate one immutable review package from the pre-fix base through the code commit. An independent reviewer must find no open Critical or Important issue before status documentation or package export begins.

### Task 5: Record Status And Prepare Final Packages

**Files:**
- Modify: the parent design and implementation plan status sections.
- Modify: this design addendum and plan only for exact commit/test evidence.

**Interfaces:**
- Consumes: clean code review and fresh gate output.
- Produces: truthful code-side status and final-HEAD package digests; no server action.

- [ ] **Step 1: Commit only status documentation**

Record the exact code commit, review range, test commands, durations, and separate `code-side fixed`, `test-server pending`, and `production unchanged` states.

- [ ] **Step 2: Export and validate packages outside the repository**

From a clean final `HEAD`, regenerate the four provenance packages and the F-15 package, validate all artifact hashes and forbidden-path scans, and record only their final digests.

- [ ] **Step 3: Draft one consolidated server authorization**

Name every digest, remote directory, Compose project, one-run boundary, retained-resource rule, evidence audit, and prohibition. Stop before SSH until that exact authorization is granted.
