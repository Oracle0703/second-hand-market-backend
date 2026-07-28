# F-04/F-05/F-06/F-14 Acceptance Provenance Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bind every F-04/F-13, F-05, F-06, and F-14 isolated acceptance run to an immutable committed-`HEAD` package and retain only sanitized, hashed success or failure evidence.

**Architecture:** Keep four separate protocol-compatible shell implementations. Each script exports a four-file `git archive HEAD` package, verifies the out-of-band digest and received source before Docker/npm, builds and tests only a temporary verified extraction, and publishes only schema-validated evidence. The four harness tasks are independent and may execute in parallel; integration documentation and final gates run only after all four reviews pass.

**Tech Stack:** Bash 3.2-compatible shell, Git archive plumbing, GNU `sha256sum`, Docker Compose, MySQL 8.4, Go 1.22 tests, Node 22.22.2, npm 10.9.7, Vitest, Vite/Taro.

## Execution Status (2026-07-28)

- Tasks 1-4 are implemented and their scoped Critical/Important review loops
  are closed through `f507a7d`. The reviewed code range starts at `d90c8e6`;
  the final whole-branch review still remains before package export.
- The final four-harness combined contract command passed at `bf45902` in
  172.461s. Repository backend full/race/vet gates passed at `2dffb36`; the
  later `f507a7d` changes only reviewed F-06 migration-contract assertions and
  the complete migrations package passed in 8.853s.
- Committed-`HEAD` temporary frontend and miniapp exports passed frontend 12
  files/27 tests plus build, and miniapp 12 files/36 tests plus WeChat/Douyin
  builds with `TARO_APP_API_BASE_URL=https://example.invalid/api/v1`.
- Local Node `v19.7.0` and npm `9.5.0` do not match the locked F-05 versions;
  engine, dependency, React Router, circular-chunk, and chunk-size warnings are
  recorded in the SDD ledger. These local results are not the locked F-05 gate.
- Task 5 documentation integration is in progress. Task 6 package generation
  has not started. Task 7 has no current server authorization and has not run.
- F-04/F-13, F-05, F-06, and F-14 remain **not test-server approved** under
  this plan. Production migrations/deployments/releases and production data,
  files, services, and configuration remain unchanged.

## Global Constraints

- Follow `docs/superpowers/specs/2026-07-28-acceptance-provenance-hardening-design.md` exactly.
- Runtime shell code is duplicated per harness; do not create or source a shared shell library and do not refactor the F-15 harness.
- Source modes are `<PREFIX>_SOURCE_LIST_ONLY=1` and `<PREFIX>_SOURCE_EXPORT_DIR=<absent absolute directory>`; normal mode requires absolute `<PREFIX>_SOURCE_PACKAGE_DIR` and a 64-character lowercase `<PREFIX>_SOURCE_PACKAGE_MANIFEST_SHA256`.
- Each package contains exactly `source-files.z`, `source-sha256.txt`, `source.tar`, and `package-sha256.txt`; `package-sha256.txt` contains exactly the first three names in that order.
- Derive paths with `git ls-tree -r --name-only -z HEAD` and bytes with `git archive HEAD`; never source transferable bytes from the index or working tree.
- Validate the package digest, all three artifact hashes, sorted unique path list, allowed/required paths, regular non-symlink received files, exact archive file list, and per-file hashes before Docker, npm, network, or tests.
- Build and test only a mode-`0700` temporary extraction. Raw command output and generated dependencies/builds stay there and are removed on every exit.
- Refuse a pre-existing retained evidence directory. Docker harnesses refuse an exact-project container, volume, or network before touching the project.
- Retain only validated classifications, approved fingerprints/snapshots, `evidence-leak-scan.txt`, and `evidence-sha256.txt`; controlled failures use a fixed stage and sanitization fallback.
- Docker harnesses may inspect only name, ID, state, and restart count for `secondhand-market-api`, `secondhand-market-web`, and `secondhand-market-mysql`; before/after snapshots must match.
- Stop only the exact isolated Compose project. Do not prune or delete unrelated Docker resources.
- F-05 uses no Docker, API, or database and must use Node `v22.22.2`, npm `10.9.7`, `https://registry.npmmirror.com`, and `TARO_APP_API_BASE_URL=https://example.invalid/api/v1`.
- Fixed remote boundaries are `/home/yu/services/secondhand-license-privacy-acceptance-20260726` with project `secondhand-license-privacy-acceptance`, `/home/yu/services/secondhand-miniapp-auth-refresh-acceptance-20260726` with no Compose project, `/home/yu/services/secondhand-upload-governance-acceptance-20260726` with project `secondhand-upload-governance-acceptance`, and `/home/yu/services/secondhand-session-revocation-acceptance-20260727` with project `secondhand-session-revocation-acceptance`.
- Preserve every existing migration, test, HTTP-boundary, and application pass criterion. Harness hardening must not change application behavior.
- Do not read, modify, stage, commit, or transfer repository `.tmp`, `backend/app.db`, `.env`, secrets, databases, uploads, backups, caches, `node_modules`, existing evidence, `miniapp/project.private.config.json`, `docs/architecture-evolution-plan-2026-07-24.md`, `docs/first-round-fix-review-2026-07-24.md`, or `docs/second-round-fix-review-2026-07-24.md`.
- Do not transfer source or operate the test server until a fresh final-`HEAD` package and consolidated exact authorization exist. Never execute production SQL or modify production data/services.
- Use RED/GREEN tests and focused Conventional Commits. Run `gofmt` on Go tests, `bash -n` on shell changes, and `git diff --check` before every commit.
- Parallel workers share one worktree: they may edit only their assigned disjoint files and must not stage or commit. The primary agent performs reviews and serializes the four task commits after each file pair passes its gate.

---

## File Map

- Create `backend/tests/license_file_privacy_acceptance_contract_test.go`: F-04/F-13 source, package, collision, evidence, and behavior-order contracts.
- Modify `deploy/acceptance/license-file-privacy-smoke.sh`: standalone F-04/F-13 package and evidence protocol.
- Create `backend/tests/miniapp_auth_refresh_acceptance_contract_test.go`: F-05 immutable package, npm tripwire, temporary-copy, cleanup, and evidence contracts.
- Modify `deploy/acceptance/miniapp-auth-refresh-smoke.sh`: standalone Node-only package and evidence protocol.
- Create `backend/tests/anonymous_upload_governance_acceptance_contract_test.go`: F-06 package, Docker refusal, failure evidence, and existing-boundary preservation contracts.
- Modify `deploy/acceptance/anonymous-upload-governance-smoke.sh`: standalone F-06 package import and classified evidence protocol.
- Modify `backend/tests/session_revocation_acceptance_contract_test.go`: extend F-14's existing safety and migration-order tests with package/evidence contracts.
- Modify `deploy/acceptance/session-revocation-smoke.sh`: replace working-tree provenance with the F-14 four-file protocol and add sanitized failure publication.
- Modify `deploy/acceptance/README.md`: document exact source modes, package format, normal-mode variables, and final-`HEAD` run rules for all four harnesses.
- Modify `docs/superpowers/specs/2026-07-28-acceptance-provenance-hardening-design.md`: record written-specification approval and later code-side/remote status without overstating production state.
- Modify `docs/release-readiness.md`: only after evidence exists, distinguish code-side closure, isolated test-server approval, and production status.
- Create four tracked remote review reports only after successful authorized runs; never commit raw remote evidence.

---

### Task 1: Harden F-04/F-13 License Privacy Acceptance

**Files:**
- Create: `backend/tests/license_file_privacy_acceptance_contract_test.go`
- Modify: `deploy/acceptance/license-file-privacy-smoke.sh`

**Interfaces:**
- Consumes: `LICENSE_FILE_PRIVACY_SOURCE_LIST_ONLY`, `LICENSE_FILE_PRIVACY_SOURCE_EXPORT_DIR`, `LICENSE_FILE_PRIVACY_SOURCE_PACKAGE_DIR`, `LICENSE_FILE_PRIVACY_SOURCE_PACKAGE_MANIFEST_SHA256`, the existing confirmation, `ACCEPTANCE_DB_ENGINE=mysql8.4`, and fixed Compose project.
- Produces: an immutable four-file package; normal-run classifications in `deploy/acceptance/evidence/license-file-privacy`; unchanged F-04/F-13 migration/API behavior.

- [ ] **Step 1: Add the committed-whitelist RED tests**

Create tests with these exact names and responsibilities:

```go
func TestLicenseFilePrivacyAcceptanceSourceListContainsOnlyCommittedWhitelist(t *testing.T)
func TestLicenseFilePrivacyAcceptanceSourceExportUsesImmutableHEAD(t *testing.T)
```

The first invokes the script with only `LICENSE_FILE_PRIVACY_SOURCE_LIST_ONLY=1`, parses NUL output, requires `Makefile`, `backend/Dockerfile`, `backend/go.mod`, `backend/go.sum`, all three `backend/migrations/0007_license_file_privacy.*.sql` files, `backend/migrations/license_file_privacy_migration_test.go`, `backend/tests/file_schema_mysql_test.go`, `backend/tests/license_file_privacy_test.go`, the new contract test, `deploy/acceptance/docker-compose.yml`, and the script itself, then rejects every common forbidden path. Build a temporary Git repository containing one committed Go file, a dirty replacement, an untracked allowed-looking Go file, and a staged-only allowed-looking Go file; require only the committed bytes/path set to export.

The second passes an absent absolute export directory and asserts exactly four mode-regular artifacts, strict `package-sha256.txt` names/order, `sha256sum -c` success, archive/list equality, and committed bytes. Set Docker and npm tripwires in `PATH` and assert neither is called. Also table-test relative, `/`, pre-existing, and list-plus-export destinations as failures.

- [ ] **Step 2: Run the source tests and capture RED**

Run:

```bash
cd backend
go test ./tests -run '^TestLicenseFilePrivacyAcceptanceSource(List|Export)' -count=1
```

Expected: FAIL because the two source variables and four-file exporter do not exist.

- [ ] **Step 3: Implement standalone F-04/F-13 source export**

Add the state and exact prefix near the script header:

```bash
retained_evidence_dir="$base_dir/evidence/license-file-privacy"
runtime_dir=""
evidence_dir=""
project_touched=0
evidence_eligible=0
success=0
current_stage="preflight"
```

Implement prefix-local `source_path_is_forbidden`, `source_path_is_allowed`, `write_source_file_list`, `validate_source_list`, `write_context_file_list`, `validate_received_source_files`, `write_directory_manifest`, `validate_package_checksums`, and `export_head_source`. Required paths are the paths named in Step 1. Allowed paths are only `Makefile`, backend Go/module/Docker/migration files, and the needed top-level/`sql` acceptance source extensions. Reject non-portable, absolute, traversal, empty-component, forbidden, and missing required paths.

Dispatch source modes before any confirmation, `.env`, Docker, evidence, or runtime setup:

```bash
if [[ "${LICENSE_FILE_PRIVACY_SOURCE_LIST_ONLY:-0}" == "1" &&
  -n "${LICENSE_FILE_PRIVACY_SOURCE_EXPORT_DIR:-}" ]]; then
  echo "choose one license privacy source mode" >&2
  exit 1
fi
if [[ "${LICENSE_FILE_PRIVACY_SOURCE_LIST_ONLY:-0}" == "1" ]]; then
  write_source_file_list
  exit 0
fi
if [[ -n "${LICENSE_FILE_PRIVACY_SOURCE_EXPORT_DIR:-}" ]]; then
  export_head_source "$LICENSE_FILE_PRIVACY_SOURCE_EXPORT_DIR"
  exit 0
fi
```

Export exactly the four artifacts, validate them through a temporary extraction, use modes `0700`/`0600`, and remove an incomplete destination on any export failure.

- [ ] **Step 4: Run source tests GREEN**

Run the Step 2 command plus:

```bash
bash -n deploy/acceptance/license-file-privacy-smoke.sh
```

Expected: all source tests PASS and Bash syntax exits 0.

- [ ] **Step 5: Add package refusal, collision, and failure-evidence RED tests**

Add:

```go
func TestLicenseFilePrivacyAcceptanceMetadataFreePackageRefusesOrProgressesBeforeDocker(t *testing.T)
func TestLicenseFilePrivacyAcceptanceRefusesEvidenceAndProjectReuse(t *testing.T)
func TestLicenseFilePrivacyAcceptanceRetainsSanitizedFailureEvidence(t *testing.T)
func TestLicenseFilePrivacyAcceptancePreservesBehaviorMatrix(t *testing.T)
```

Use a temporary committed repository and exported package. A valid no-`.git` received tree with correct digest must reach a Docker tripwire. Wrong digest, changed package artifact, changed received source, missing/archive-extra file, symlink, unsorted/duplicate/forbidden list, missing required path, and mismatched per-file hash must not reach it. Simulate each exact-project resource type and an existing evidence directory separately.

The controlled Docker failure injects a secret marker into raw output after the permitted production-before snapshot. Retained evidence may contain only `acceptance-results.txt`, validated available production snapshots, `evidence-leak-scan.txt`, and `evidence-sha256.txt`; hashes must verify and neither the secret marker nor a raw migration/test filename may remain. The behavior test requires all 14 dirty-preflight invocations, clean `0007..0009` markers, both AutoMigrate focused tests, and production comparison in the original order.

- [ ] **Step 6: Run refusal/evidence tests and capture RED**

Run:

```bash
cd backend
go test ./tests -run '^TestLicenseFilePrivacyAcceptance(Metadata|Refuses|Retains|Preserves)' -count=1
```

Expected: FAIL because normal mode does not import a package and current evidence is written directly/reused.

- [ ] **Step 7: Implement verified normal mode and classified evidence**

Require absolute package directory and lowercase manifest digest, verify the four regular non-symlink artifacts and strict three-line manifest, extract into a new temporary build context, compare exact file list/hashes, and compare received listed files/hashes before checking `.env` or Docker. Replace the Compose build context through a temporary override.

Move every raw command output to `$runtime_dir/*.raw`. Append only these schemas to temporary `acceptance-results.txt` after the existing assertions pass:

```text
classification=source_package|result=PASS|count=<source_count>|sha256=<source_manifest_sha256>
classification=mysql_version|result=PASS|count=1
classification=license_preflight_failures|result=PASS|count=14
classification=clean_migration|result=PASS|count=1
classification=api_auto_migrate_false|result=PASS|count=1
classification=api_auto_migrate_true|result=PASS|count=1
classification=production_snapshot|result=PASS|count=3
```

Add strict checkpoint/snapshot validators, per-harness leak patterns, deterministic hashing, atomic evidence publication, a hardcoded sanitization fallback, and an exit trap. The trap attempts a permitted production-after snapshot on runtime failure, stops only `secondhand-license-privacy-acceptance`, removes temporary files, and never publishes preflight failure evidence.

- [ ] **Step 8: Run the complete F-04/F-13 contract GREEN**

Run:

```bash
gofmt -w backend/tests/license_file_privacy_acceptance_contract_test.go
cd backend
go test ./tests -run '^TestLicenseFilePrivacyAcceptance' -count=1
cd ..
bash -n deploy/acceptance/license-file-privacy-smoke.sh
git diff --check
```

Expected: all focused tests PASS; syntax and diff checks exit 0.

- [ ] **Step 9: Commit the isolated deliverable**

```bash
git add backend/tests/license_file_privacy_acceptance_contract_test.go \
  deploy/acceptance/license-file-privacy-smoke.sh
git commit -m "test(acceptance): bind license privacy source"
```

---

### Task 2: Harden F-05 Miniapp Auth Refresh Acceptance

**Files:**
- Create: `backend/tests/miniapp_auth_refresh_acceptance_contract_test.go`
- Modify: `deploy/acceptance/miniapp-auth-refresh-smoke.sh`

**Interfaces:**
- Consumes: `MINIAPP_AUTH_REFRESH_SOURCE_LIST_ONLY`, `MINIAPP_AUTH_REFRESH_SOURCE_EXPORT_DIR`, `MINIAPP_AUTH_REFRESH_SOURCE_PACKAGE_DIR`, `MINIAPP_AUTH_REFRESH_SOURCE_PACKAGE_MANIFEST_SHA256`, and existing F-05 confirmation.
- Produces: a metadata-free package and Node-only sanitized evidence; generated dependencies/builds exist only in the temporary verified extraction.

- [ ] **Step 1: Add source/export RED tests**

Create:

```go
func TestMiniappAuthRefreshAcceptanceSourceListContainsOnlyCommittedWhitelist(t *testing.T)
func TestMiniappAuthRefreshAcceptanceSourceExportUsesImmutableHEAD(t *testing.T)
```

Require exactly the documented miniapp roots plus `Makefile` and the F-05 script. Explicit required files are `.nvmrc`, both package files, `src/services/request.ts`, `tests/request-refresh.test.ts`, both public project configs, the script, and `Makefile`. Assert `project.private.config.json`, `.swc`, `dist`, `node_modules`, `.env`, caches, and staged/dirty/untracked bytes cannot enter. Export tests use npm/node/network tripwires and the same exact four-artifact/tamper checks as the F-05 prefix contract.

- [ ] **Step 2: Run source/export RED**

```bash
cd backend
go test ./tests -run '^TestMiniappAuthRefreshAcceptanceSource(List|Export)' -count=1
```

Expected: FAIL because F-05 has no source modes.

- [ ] **Step 3: Implement F-05 source package modes**

Implement the same named package functions locally in the F-05 script using the F-05 prefix. The allowlist is exactly:

```text
Makefile
miniapp/.nvmrc
miniapp/babel.config.js
miniapp/config/**
miniapp/package.json
miniapp/package-lock.json
miniapp/project.config.json
miniapp/project.tt.json
miniapp/src/**
miniapp/tests/**
miniapp/tsconfig.json
miniapp/vitest.config.mjs
deploy/acceptance/miniapp-auth-refresh-smoke.sh
```

Source modes run before confirmation and before checking `node`, `npm`, or `sha256sum`. Export failure removes its incomplete destination.

- [ ] **Step 4: Run source/export GREEN**

```bash
gofmt -w backend/tests/miniapp_auth_refresh_acceptance_contract_test.go
cd backend
go test ./tests -run '^TestMiniappAuthRefreshAcceptanceSource(List|Export)' -count=1
cd ..
bash -n deploy/acceptance/miniapp-auth-refresh-smoke.sh
```

Expected: tests PASS and syntax exits 0.

- [ ] **Step 5: Add normal-mode, temporary-tree, and evidence RED tests**

Add:

```go
func TestMiniappAuthRefreshAcceptanceMetadataFreePackageRefusesOrProgressesBeforeNPM(t *testing.T)
func TestMiniappAuthRefreshAcceptanceRefusesExistingEvidenceBeforeNPM(t *testing.T)
func TestMiniappAuthRefreshAcceptanceUsesTemporaryVerifiedTree(t *testing.T)
func TestMiniappAuthRefreshAcceptanceRetainsSanitizedFailureEvidence(t *testing.T)
func TestMiniappAuthRefreshAcceptancePreservesCommandMatrix(t *testing.T)
```

The npm/node stubs record working directory and arguments. A valid package reaches exact version checks and then npm; every digest/list/archive/received-file tamper refuses first. The working directory must end in `/build-context/miniapp`, never the received tree. The stub creates `node_modules`, `dist`, and a secret-bearing raw log, then fails; the trap must remove that temporary tree, leave received source byte-identical without generated paths, and retain only classified/hash evidence. Existing evidence refuses before node/npm. The command matrix asserts the mirror registry flags, focused test, full test, WeChat build, Douyin build, and `example.invalid` environment in order.

- [ ] **Step 6: Run normal-mode tests and capture RED**

```bash
cd backend
go test ./tests -run '^TestMiniappAuthRefreshAcceptance(Metadata|Refuses|Uses|Retains|Preserves)' -count=1
```

Expected: FAIL because current F-05 runs and logs directly in received source.

- [ ] **Step 7: Implement temporary Node execution and evidence publication**

Verify package/received/archive state before node/npm. Refuse existing retained evidence. Run all npm commands from `$runtime_dir/build-context/miniapp`, with raw output under `$runtime_dir`, and retain:

```text
classification=source_package|result=PASS|count=<source_count>|sha256=<source_manifest_sha256>
classification=toolchain|result=PASS|count=2
classification=npm_ci|result=PASS|count=1
classification=focused_tests|result=PASS|count=1
classification=full_tests|result=PASS|count=1
classification=build_weapp|result=PASS|count=1
classification=build_tt|result=PASS|count=1
```

Set `current_stage` before each command. On failure, publish only validated completed classifications plus a fixed failure-stage line, leak-scan result, and hash manifest. On sanitization uncertainty, publish only the two hardcoded failure classifications and hashes. Always remove the runtime directory.

- [ ] **Step 8: Run complete F-05 contract GREEN**

```bash
gofmt -w backend/tests/miniapp_auth_refresh_acceptance_contract_test.go
cd backend
go test ./tests -run '^TestMiniappAuthRefreshAcceptance' -count=1
cd ..
bash -n deploy/acceptance/miniapp-auth-refresh-smoke.sh
git diff --check
```

Expected: all focused contracts PASS.

- [ ] **Step 9: Commit the isolated deliverable**

```bash
git add backend/tests/miniapp_auth_refresh_acceptance_contract_test.go \
  deploy/acceptance/miniapp-auth-refresh-smoke.sh
git commit -m "test(acceptance): bind miniapp refresh source"
```

---

### Task 3: Harden F-06 Anonymous Upload Governance Acceptance

**Files:**
- Create: `backend/tests/anonymous_upload_governance_acceptance_contract_test.go`
- Modify: `deploy/acceptance/anonymous-upload-governance-smoke.sh`

**Interfaces:**
- Consumes: `ANONYMOUS_UPLOAD_GOVERNANCE_SOURCE_LIST_ONLY`, `ANONYMOUS_UPLOAD_GOVERNANCE_SOURCE_EXPORT_DIR`, `ANONYMOUS_UPLOAD_GOVERNANCE_SOURCE_PACKAGE_DIR`, `ANONYMOUS_UPLOAD_GOVERNANCE_SOURCE_PACKAGE_MANIFEST_SHA256`, existing confirmation, MySQL 8.4, and fixed Compose project.
- Produces: immutable backend/frontend/acceptance package, temporary Compose build contexts, and classified F-06 evidence without raw test/HTTP output.

- [ ] **Step 1: Add source/export RED tests**

Create:

```go
func TestAnonymousUploadGovernanceAcceptanceSourceListContainsOnlyCommittedWhitelist(t *testing.T)
func TestAnonymousUploadGovernanceAcceptanceSourceExportUsesImmutableHEAD(t *testing.T)
```

Require `Makefile`, backend Docker/module files, all three `backend/migrations/0008_anonymous_upload_governance.*.sql` files, `backend/migrations/anonymous_upload_governance_migration_test.go`, `backend/internal/app/upload_governance.go`, `backend/internal/app/upload_governance_mysql_test.go`, the new contract test, `frontend/package.json`, `frontend/package-lock.json`, `frontend/index.html`, all three frontend TypeScript/Vite configs, `frontend/src/utils/upload.ts`, `frontend/src/utils/upload.test.ts`, `deploy/acceptance/docker-compose.yml`, `deploy/acceptance/frontend.Dockerfile`, `deploy/acceptance/nginx.conf`, the F-06 script, and acceptance SQL. Explicitly reject dependency/build/cache/evidence/upload/database/private paths and staged/dirty/untracked bytes. Assert four exact artifacts and no Docker/network action in export mode.

- [ ] **Step 2: Run source/export RED**

```bash
cd backend
go test ./tests -run '^TestAnonymousUploadGovernanceAcceptanceSource(List|Export)' -count=1
```

Expected: FAIL because current source hashes come from working-tree `find`.

- [ ] **Step 3: Replace working-tree manifest generation with F-06 package modes**

Add prefix-local package functions and dispatch source modes before confirmation. The F-06 allowlist covers only `Makefile`, required backend Go/module/Docker/migrations, frontend `src` and exact package/config/index files, plus required acceptance source. Delete `write_source_manifest`; no normal path may hash the working tree as its source authority. Export from `git archive HEAD`, validate through a temporary extraction, and remove incomplete export state.

- [ ] **Step 4: Run source/export GREEN**

```bash
gofmt -w backend/tests/anonymous_upload_governance_acceptance_contract_test.go
cd backend
go test ./tests -run '^TestAnonymousUploadGovernanceAcceptanceSource(List|Export)' -count=1
cd ..
bash -n deploy/acceptance/anonymous-upload-governance-smoke.sh
```

Expected: source contracts PASS.

- [ ] **Step 5: Add normal-mode and evidence RED tests**

Add:

```go
func TestAnonymousUploadGovernanceAcceptanceMetadataFreePackageRefusesOrProgressesBeforeDocker(t *testing.T)
func TestAnonymousUploadGovernanceAcceptanceRefusesEvidenceAndProjectReuse(t *testing.T)
func TestAnonymousUploadGovernanceAcceptanceRetainsSanitizedFailureEvidence(t *testing.T)
func TestAnonymousUploadGovernanceAcceptancePreservesUploadBoundaryMatrix(t *testing.T)
```

Cover every package/list/archive/received-file tamper and the three project resource types before Docker. Controlled failure must prove no raw migration, Go, frontend, bootstrap, curl response, token, object key, upload path, or injected secret reaches retained evidence. Require hash verification. The behavior contract requires skipped-`0007`, four dirty-`0008` cases, clean migration markers, both MySQL AutoMigrate modes, backend and frontend gates, and seven exact upload/request boundary outcomes.

- [ ] **Step 6: Run normal/evidence RED**

```bash
cd backend
go test ./tests -run '^TestAnonymousUploadGovernanceAcceptance(Metadata|Refuses|Retains|Preserves)' -count=1
```

Expected: FAIL because normal mode has no package binding and currently publishes raw logs.

- [ ] **Step 7: Implement verified temporary Compose contexts and classified evidence**

Verify the package and received source before `.env`/Docker, extract only into `$runtime_dir/build-context`, and append a temporary Compose override for every source build context. Preserve the existing volume used for synthetic upload files but never bind a received source path into a test container.

Keep all existing assertions, then emit only:

```text
classification=source_package|result=PASS|count=<source_count>|sha256=<source_manifest_sha256>
classification=mysql_version|result=PASS|count=1
classification=skipped_0007_preflight|result=PASS|count=1
classification=dirty_0008_preflights|result=PASS|count=4
classification=clean_migration|result=PASS|count=1
classification=mysql_auto_migrate_false|result=PASS|count=1
classification=mysql_auto_migrate_true|result=PASS|count=1
classification=backend_tests|result=PASS|count=1
classification=frontend_tests_build|result=PASS|count=1
classification=upload_boundaries|result=PASS|count=7
classification=historical_rows_files|result=PASS|count=2
classification=production_snapshot|result=PASS|count=3
```

Approved historical fingerprint files and production snapshots may be separate retained files after strict schema validation. Move raw command/HTTP bodies into runtime storage, add failure-stage sanitization/fallback, and attempt production-after comparison on failure. Stop only `secondhand-upload-governance-acceptance`.

- [ ] **Step 8: Run complete F-06 contract GREEN**

```bash
gofmt -w backend/tests/anonymous_upload_governance_acceptance_contract_test.go
cd backend
go test ./tests -run '^TestAnonymousUploadGovernanceAcceptance' -count=1
cd ..
bash -n deploy/acceptance/anonymous-upload-governance-smoke.sh
git diff --check
```

Expected: all F-06 harness contracts PASS.

- [ ] **Step 9: Commit the isolated deliverable**

```bash
git add backend/tests/anonymous_upload_governance_acceptance_contract_test.go \
  deploy/acceptance/anonymous-upload-governance-smoke.sh
git commit -m "test(acceptance): bind upload governance source"
```

---

### Task 4: Harden F-14 Session Revocation Acceptance

**Files:**
- Modify: `backend/tests/session_revocation_acceptance_contract_test.go`
- Modify: `deploy/acceptance/session-revocation-smoke.sh`

**Interfaces:**
- Consumes: `SESSION_REVOCATION_SOURCE_LIST_ONLY`, `SESSION_REVOCATION_SOURCE_EXPORT_DIR`, `SESSION_REVOCATION_SOURCE_PACKAGE_DIR`, `SESSION_REVOCATION_SOURCE_PACKAGE_MANIFEST_SHA256`, existing confirmation, MySQL 8.4, and fixed Compose project.
- Produces: committed backend/acceptance package and sanitized F-14 evidence while retaining the existing migration/API ordering contract.

- [ ] **Step 1: Add source/export RED tests without weakening existing tests**

Append:

```go
func TestSessionRevocationAcceptanceSourceListContainsOnlyCommittedWhitelist(t *testing.T)
func TestSessionRevocationAcceptanceSourceExportUsesImmutableHEAD(t *testing.T)
```

Require `Makefile`, backend Docker/module files, all current migrations including the three `0009_buyer_intent_open_uniqueness.*.sql` files, `backend/tests/session_revocation_mysql_test.go`, the contract test, `backend/internal/app/auth_handlers.go`, `backend/internal/app/admin_handlers.go`, `backend/internal/app/server.go`, Compose/README/script/SQL files. Test immutable committed bytes and the exact four-artifact export; Docker must not be called. Do not remove or relax `TestSessionRevocationAcceptanceUsesCurrentMigrationChain` or either false-mode ordering mutation test.

- [ ] **Step 2: Run source/export RED**

```bash
cd backend
go test ./tests -run '^TestSessionRevocationAcceptanceSource(List|Export)' -count=1
```

Expected: FAIL because current source selection uses working-tree `find`/`tar`.

- [ ] **Step 3: Implement F-14 source package modes**

Replace `write_source_file_list` and working-tree tar use with prefix-local `git ls-tree`/`git archive` export and normal package import. Use the backend/acceptance allowlist and explicit required paths. Source modes run before confirmation, `.env`, and Docker. Delete the old working-tree manifest/build-context branch after new tests pass.

- [ ] **Step 4: Run source/export and existing ordering GREEN**

```bash
gofmt -w backend/tests/session_revocation_acceptance_contract_test.go
cd backend
go test ./tests -run '^TestSession(RevocationAcceptanceSource|RevocationAcceptanceUsesCurrentMigrationChain|CurrentMigrationChain)' -count=1
cd ..
bash -n deploy/acceptance/session-revocation-smoke.sh
```

Expected: new source tests and all existing migration-order tests PASS.

- [ ] **Step 5: Add normal-mode and failure-evidence RED tests**

Append:

```go
func TestSessionRevocationAcceptanceMetadataFreePackageRefusesOrProgressesBeforeDocker(t *testing.T)
func TestSessionRevocationAcceptanceRefusesEvidenceAndProjectReuse(t *testing.T)
func TestSessionRevocationAcceptanceRetainsSanitizedFailureEvidence(t *testing.T)
func TestSessionRevocationAcceptancePreservesRuntimeGateOrder(t *testing.T)
```

Cover all digest/artifact/list/archive/received-file/symlink/required-path tampering before Docker, evidence/project collisions, controlled raw-output secret injection, deterministic evidence hashes, and temporary cleanup. Runtime order requires MySQL 8.4, clean `0001..0009`, false-mode focused API, clean reset, true-mode focused API, full backend test, vet, and production comparison.

- [ ] **Step 6: Run normal/evidence RED**

```bash
cd backend
go test ./tests -run '^TestSessionRevocationAcceptance(Metadata|Refuses|Retains|Preserves)' -count=1
```

Expected: FAIL because normal mode has no authorized package digest and failure publication is incomplete.

- [ ] **Step 7: Implement verified normal mode and failure evidence**

Verify received/package/archive bytes before `.env` or Docker, use the temporary extraction for Compose builds, retain current raw-to-classified focused/full/vet behavior, and normalize all results into:

```text
classification=source_package|result=PASS|count=<source_count>|sha256=<source_manifest_sha256>
classification=mysql_version|result=PASS|count=1
classification=migration_chain|result=PASS|count=1
classification=session_auto_migrate_false|result=PASS|count=1
classification=session_auto_migrate_true|result=PASS|count=1
classification=backend_tests|result=PASS|count=1
classification=go_vet|result=PASS|count=1
classification=production_snapshot|result=PASS|count=3
```

Add strict failure-stage publication, sanitization fallback, deterministic hash manifest, production-after attempt on failure, and exact-project-only stop. Preserve absent production-container sentinels.

- [ ] **Step 8: Run complete F-14 contract GREEN**

```bash
gofmt -w backend/tests/session_revocation_acceptance_contract_test.go
cd backend
go test ./tests -run '^TestSession(RevocationAcceptance|CurrentMigrationChain)' -count=1
cd ..
bash -n deploy/acceptance/session-revocation-smoke.sh
git diff --check
```

Expected: all F-14 source, safety, evidence, and migration-order contracts PASS.

- [ ] **Step 9: Commit the isolated deliverable**

```bash
git add backend/tests/session_revocation_acceptance_contract_test.go \
  deploy/acceptance/session-revocation-smoke.sh
git commit -m "test(acceptance): bind session revocation source"
```

---

### Task 5: Integrate Documentation And Run Local Review Gates

**Files:**
- Modify: `deploy/acceptance/README.md`
- Modify: `docs/superpowers/specs/2026-07-28-acceptance-provenance-hardening-design.md`
- Modify: `docs/superpowers/plans/2026-07-28-acceptance-provenance-hardening.md`

**Interfaces:**
- Consumes: reviewed commits from Tasks 1-4.
- Produces: documented command contract, code-side status, and one locally verified commit range ready for final package generation.

- [ ] **Step 1: Review each task against the written specification**

For each harness, compare implementation to sections 6-10 and 13 of the design. Record findings with file/line references. Resolve every Critical/Important finding through its original implementer and rerun that harness's full focused command before proceeding.

- [ ] **Step 2: Document exact operator interfaces**

For each README harness section, add the four exact prefix variables, direct list/export examples, four artifact names, digest calculation, mandatory remote source extraction, normal-run environment, collision rules, temporary build behavior, evidence schema, and no-silent-rerun rule. State that source modes use no Docker/npm/network and that normal execution still needs a later exact authorization.

- [ ] **Step 3: Run all four contract suites together**

```bash
cd backend
go test ./tests -run '^(TestLicenseFilePrivacyAcceptance|TestMiniappAuthRefreshAcceptance|TestAnonymousUploadGovernanceAcceptance|TestSessionRevocationAcceptance|TestSessionCurrentMigrationChain)' -count=1
```

Expected: every source/package/refusal/evidence/order contract passes.

- [ ] **Step 4: Run repository-local behavior gates serially**

```bash
cd backend
go test ./... -count=1
go test -race ./internal/app ./tests -count=1
go vet ./...
cd ../frontend
npm test
npm run build
cd ../miniapp
npm test
TARO_APP_API_BASE_URL=https://example.invalid/api/v1 npm run build:weapp
TARO_APP_API_BASE_URL=https://example.invalid/api/v1 npm run build:tt
```

Expected: every command exits 0 under the repository's available locked toolchains. If the local Node version is not `22.22.2`/npm `10.9.7`, record the exact mismatch and do not claim the F-05 locked local gate; the authorized server remains the required locked-toolchain authority.

- [ ] **Step 5: Run static and repository-safety gates**

```bash
bash -n deploy/acceptance/license-file-privacy-smoke.sh
bash -n deploy/acceptance/miniapp-auth-refresh-smoke.sh
bash -n deploy/acceptance/anonymous-upload-governance-smoke.sh
bash -n deploy/acceptance/session-revocation-smoke.sh
git diff --check
git status --short
```

Inspect staged paths explicitly and prove no forbidden source, generated output, evidence, `.env`, secret, database, upload, backup, cache, `node_modules`, `.tmp`, `backend/app.db`, or protected review document is present.

- [ ] **Step 6: Update code-side status without claiming remote approval**

Set the design and plan execution status to the exact reviewed commit range and local commands. State separately that isolated F-04/F-05/F-06/F-14 server approval is pending and production is unchanged.

- [ ] **Step 7: Commit integration documentation**

```bash
git add deploy/acceptance/README.md \
  docs/superpowers/specs/2026-07-28-acceptance-provenance-hardening-design.md \
  docs/superpowers/plans/2026-07-28-acceptance-provenance-hardening.md
git commit -m "docs(acceptance): document immutable source protocol"
```

---

### Task 6: Generate Final-HEAD Packages And Prepare Consolidated Authorization

**Files:**
- Create only outside the repository: four temporary package directories.
- Modify no tracked source until remote evidence exists.

**Interfaces:**
- Consumes: clean final reviewed `HEAD` and all four export modes.
- Produces: four exact package digests, local artifact hashes, NUL source lists, and one consolidated authorization request; performs no transfer or server action.

- [ ] **Step 1: Prove the final worktree and commit identity**

```bash
git status --short
git rev-parse HEAD
git log -1 --oneline
```

Expected: clean worktree and one recorded final commit.

- [ ] **Step 2: Export fresh packages to absent temporary directories**

Create four directories with `mktemp -d`, remove only the four empty child destinations, then invoke each direct export mode into its absent destination. Do not use repository `.tmp` or write a package under the worktree.

```bash
LICENSE_FILE_PRIVACY_SOURCE_EXPORT_DIR="$license_export" \
  ./deploy/acceptance/license-file-privacy-smoke.sh
MINIAPP_AUTH_REFRESH_SOURCE_EXPORT_DIR="$miniapp_export" \
  ./deploy/acceptance/miniapp-auth-refresh-smoke.sh
ANONYMOUS_UPLOAD_GOVERNANCE_SOURCE_EXPORT_DIR="$upload_export" \
  ./deploy/acceptance/anonymous-upload-governance-smoke.sh
SESSION_REVOCATION_SOURCE_EXPORT_DIR="$session_export" \
  ./deploy/acceptance/session-revocation-smoke.sh
```

- [ ] **Step 3: Verify package artifacts and derive authorization digests**

For every package, list exactly four regular files, run `sha256sum -c package-sha256.txt`, compare `tar -tf` to the NUL source list after safe normalization, and compute:

```bash
sha256sum package-sha256.txt
sha256sum source-files.z source-sha256.txt source.tar package-sha256.txt
```

Record hashes in the local authorization draft only; do not commit packages.

- [ ] **Step 4: Draft one exact authorization without executing it**

The draft names all four final-`HEAD` digests, fixed remote directories/projects, exact source lists, allowed directory reset if needed, remote-only secret generation, one run per harness, locked F-05 registry/toolchain, permitted sanitized evidence retention/read-only audit, exact production snapshot boundary, and all continuing prohibitions. Include the separate corrected F-15 package/run only after regenerating it from the same final `HEAD` and explicitly naming its retained failed-run cleanup targets.

Stop here until the user grants that exact authorization.

---

### Task 7: Execute Authorized Isolated Runs And Record Evidence

**Files:**
- Create: `docs/superpowers/reviews/2026-07-28-license-file-privacy-isolated-acceptance.md`
- Create: `docs/superpowers/reviews/2026-07-28-miniapp-auth-refresh-isolated-acceptance.md`
- Create: `docs/superpowers/reviews/2026-07-28-anonymous-upload-governance-isolated-acceptance.md`
- Create: `docs/superpowers/reviews/2026-07-28-session-revocation-isolated-acceptance.md`
- Modify: `docs/release-readiness.md`
- Modify: design/plan status documents for the affected findings.

**Interfaces:**
- Consumes: an exact user authorization matching Task 6 and fresh final-`HEAD` packages.
- Produces: leak-scanned/hash-verified remote evidence summaries and truthful test-server status; production remains unchanged.

- [ ] **Step 1: Reconcile authorization before SSH**

Compare every requested remote action, path, package digest, Compose project, production observation, retention, and deletion against the user's exact grant. Do not infer missing authority.

- [ ] **Step 2: Transfer and independently verify only authorized artifacts**

Create/reset only named remote directories, transfer only four package files per harness, compare local/remote SHA-256, extract only listed source, and regenerate only remote-dedicated `.env`/secrets where authorized. Run a forbidden-path scan before execution.

- [ ] **Step 3: Run each harness once**

Supply its confirmation, fixed project/engine where applicable, absolute package directory, and exact out-of-band digest. Do not silently retry a failed one-run authorization. Stop and document the first failure classification.

- [ ] **Step 4: Audit only sanitized evidence**

Require `evidence-leak-scan.txt` PASS, run `sha256sum -c evidence-sha256.txt`, validate the allowed file schema, and compare Docker production snapshots byte-for-byte. Do not read raw logs or prohibited remote state.

- [ ] **Step 5: Write tracked review reports and status**

Each report records design/plan, final commit, package digest, remote directory/project, toolchain/database version, classifications, evidence hashes, before/after equality, retained isolated resources, and explicit non-actions. Mark test-server approved only for a fully passing audited run. Production deployment/release remains unchanged.

- [ ] **Step 6: Run final documentation and branch checks, then commit**

```bash
git diff --check
git status --short
git add docs/superpowers/reviews/2026-07-28-license-file-privacy-isolated-acceptance.md \
  docs/superpowers/reviews/2026-07-28-miniapp-auth-refresh-isolated-acceptance.md \
  docs/superpowers/reviews/2026-07-28-anonymous-upload-governance-isolated-acceptance.md \
  docs/superpowers/reviews/2026-07-28-session-revocation-isolated-acceptance.md \
  docs/release-readiness.md \
  docs/superpowers/specs/2026-07-28-acceptance-provenance-hardening-design.md \
  docs/superpowers/plans/2026-07-28-acceptance-provenance-hardening.md
git commit -m "docs(acceptance): record isolated provenance gates"
```

Inspect the exact staged list before committing; never stage remote raw evidence or a protected document.
