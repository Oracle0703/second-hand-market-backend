# F-11 Acceptance Resource Identity Correction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the F-11 isolated MySQL 8.4 acceptance on `aliyun-server` after correcting Docker resource identity validation, without modifying production Docker or production data.

**Architecture:** Pin transferable source to commit `fc8b6a95c7be56e719f656614daa91f40b33fa06`, validate the one isolated Compose project's container and network by full IDs, and keep resource discovery separate from deletion. Run only one acceptance process at a time; direct iterative debugging is allowed inside the exact test project, while every attempt and mutation is recorded for the final evidence report.

**Tech Stack:** Bash, Git, rsync, SSH, Docker Compose, MySQL 8.4, Go 1.22.

## Global Constraints

- Host: `aliyun-server`.
- Exact directory: `/home/yu/services/secondhand-buyer-intent-acceptance-20260727`.
- Exact Compose project: `secondhand-buyer-intent-acceptance`.
- Transfer only the committed whitelist produced from source commit `fc8b6a95c7be56e719f656614daa91f40b33fa06`.
- Never run two acceptance attempts concurrently; create a distinct local marker before each attempt.
- Direct iterative debugging is allowed only inside the exact test directory and Compose project.
- Do not modify, restart, delete, reconfigure, migrate, deploy, or otherwise operate any production Docker resource.
- Production containers may only be read by the existing acceptance script for name, ID, state, and restart-count snapshots.
- Do not read `.env` or secret contents, production SQL/logs/environment/mounts/configuration, databases, uploads, backups, old evidence, `.git`, caches, `node_modules`, `backend/app.db`, `.tmp`, or the protected review documents.
- Do not execute production 0008/0009 or deploy production code.

---

### Task 1: Validate and transfer the pinned source

**Files:**
- Read: `deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh`
- Generate outside the repository: fresh NUL whitelist, source manifest, dry-run output, and remote file list
- No tracked repository file changes

**Interfaces:**
- Consumes: source commit `fc8b6a95c7be56e719f656614daa91f40b33fa06`.
- Produces: a mode-0700 remote directory containing exactly 120 regular files whose manifest matches the local manifest.

- [x] **Step 1: Verify the local source boundary**

Run from the linked worktree:

```bash
git diff --quiet
git diff --cached --quiet
test "$(git rev-parse HEAD)" = fc8b6a95c7be56e719f656614daa91f40b33fa06
```

Generate `source-files.z` and `local-source-sha256.txt` with the script's
`BUYER_INTENT_SOURCE_LIST_ONLY=1` and `BUYER_INTENT_SOURCE_MANIFEST_ONLY=1`
modes. Require 120 sorted unique committed regular files, blob equality with
`fc8b6a9`, manifest/list path equality, `sha256sum -c`, and zero forbidden paths.

Expected manifest file SHA-256:

```text
ee7694a2d5c7ceed37243cbfc2c1d24adc4843db9fd91ea0a3ceea452118740b
```

- [x] **Step 2: Validate the retained Docker identities with no mutation**

Use `docker container ls -aq --no-trunc` and
`docker network ls -q --no-trunc` with the exact project label. Require one
container, one volume, and one network. Inspect and compare full IDs, then require:

```text
container name: /secondhand-buyer-intent-acceptance-mysql-1
container running: false
volume name: secondhand-buyer-intent-acceptance_mysql-data
network name: secondhand-buyer-intent-acceptance_acceptance
project label: secondhand-buyer-intent-acceptance
```

- [x] **Step 3: Delete only validated test resources and rebuild the directory**

Delete by the validated full container ID, exact volume name, and full network
ID. Require all three exact-label queries to become empty. Delete only the
literal target directory, recreate it with `install -d -m 0700`, and require
`stat -c %a` to return `700`.

- [x] **Step 4: Prove the rsync dry-run selector**

Run:

```bash
rsync -anvi --from0 --files-from="$source_files" --relative \
  --out-format='%i|%n' ./ \
  aliyun-server:/home/yu/services/secondhand-buyer-intent-acceptance-20260727/
```

On the local macOS rsync, outgoing file entries use `<f`, not `>f`. Do not use
`path` as a zsh variable because it is tied to `PATH`; use `item_path`. Require
exactly 120 `<f` entries and allow only `cd` directory entries that are ancestors
of a whitelisted file.

- [x] **Step 5: Transfer and compare the remote source**

Run the identical rsync without `-n`. Require exactly 120 remote regular files,
zero symlinks, a byte-identical remote manifest, and target mode `700`.

---

### Task 2: Prepare, run, and directly debug the isolated project

**Files:**
- Remote generated only: `deploy/acceptance/.env`, `deploy/acceptance/secrets/`, `deploy/acceptance/backups/`, `deploy/acceptance/evidence/`
- Append: `.superpowers/sdd/2026-07-28-anonymous-upload-index-having-compatibility/task-3-report.md`
- No production or tracked source changes unless a reproducible source defect is found

**Interfaces:**
- Consumes: the verified 120-file remote source tree.
- Produces: one completed acceptance attempt with sanitized evidence, or a reproducible isolated failure with attempt-specific diagnostics.

- [x] **Step 1: Prepare remote-only secrets without reading them**

Run `./deploy/acceptance/prepare.sh` once. Verify only metadata under
`deploy/acceptance`: `.env` mode `600`, `secrets` mode `700`, and both password
files mode `600`. Never print their contents.

- [x] **Step 2: Start attempt 1 with a non-overwritable local marker**

Run exactly:

```bash
ssh aliyun-server \
  'cd /home/yu/services/secondhand-buyer-intent-acceptance-20260727 && env \
  COMPOSE_PROJECT_NAME=secondhand-buyer-intent-acceptance \
  BUYER_INTENT_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_BUYER_INTENT_DATA \
  ACCEPTANCE_DB_ENGINE=mysql8.4 make acceptance-buyer-intent-smoke'
```

Attempt 1 reached repeated successful 0008 MySQL 8.4.8 migration scenarios, then
stalled during the backend image's `go build`. After approximately 13 minutes the
user requested another attempt. Interrupt only the exact local SSH process and
record exit 255. Do not start attempt 2 until no attempt-1 process remains.

- [ ] **Step 3: Recover SSH and diagnose the stalled test build**

Retry SSH with a bounded `ConnectTimeout`. Once connected, read only host load,
memory, root filesystem availability, exact-project container status, and build
processes matching `go build`, `docker build`, or `buildkit`. Do not enumerate or
operate production containers.

If an attempt-1 build remains, terminate only that exact test build process and
wait until it exits. Record the root cause before changing source or build settings.

- [ ] **Step 4: Create attempt 2 marker and rerun one process**

Create `acceptance-attempt-2.marker` with `mkdir`; fail if it exists. Confirm no
acceptance or build process for the exact project is running, then execute the same
acceptance command once. Reuse the existing isolated project and Docker build cache.

- [ ] **Step 5: Debug only a reproducible isolated failure**

If attempt 2 reports an application, migration, test, or build error, capture the
exact command, exit code, failing line, and test-project state. Trace the failing
component before editing. Any tracked source fix must use RED -> GREEN, local focused
and full gates, a focused commit, a new exact manifest comparison, and no production
operation.

---

### Task 3: Gate evidence and record acceptance

**Files:**
- Create: `docs/superpowers/reviews/2026-07-27-buyer-intent-open-uniqueness-isolated-acceptance.md`
- Modify: `docs/superpowers/specs/2026-07-28-anonymous-upload-index-having-compatibility-design.md`
- Modify: `docs/superpowers/specs/2026-07-27-buyer-intent-open-uniqueness-design.md`
- Modify: `docs/release-readiness.md`
- Modify: `docs/full-project-code-review-2026-07-24.md`

**Interfaces:**
- Consumes: an exit-0 acceptance attempt and its newly generated sanitized evidence.
- Produces: accepted F-11 test-server status and the evidence prerequisite for F-12.

- [ ] **Step 1: Apply the three success-only evidence gates**

Only after acceptance exits 0, require `forbidden_matches=0`, verify every entry in
`evidence-sha256.txt`, and require byte-identical production before/after snapshots.
Stop evidence reading if any gate fails.

- [ ] **Step 2: Audit every required evidence category**

Verify MySQL 8.4.x, 0008 passage, all 0009 success and rejection states, exact error
1644/45000 semantics, API schema, AutoMigrate false/true, backend full/race/vet,
source manifest equality, exact-project retention, and allowed production snapshot
fields. Read only the new sanitized evidence directory.

- [ ] **Step 3: Write and commit the acceptance report**

Record all attempt dispositions, accepted source commit and manifest, versions,
evidence hashes, production snapshot equality, retained project location, and the
statement that production 0008/0009 were not run. Do not include credentials, tokens,
contacts, row identifiers, raw logs, request data, or production rows.

- [ ] **Step 4: Advance F-11 and F-12 status accurately**

Mark F-11 test-server status passed only after the evidence audit. Change F-12 only
from blocked to prerequisite satisfied; do not claim F-12 implementation complete.
Run `git diff --check`, `git show --check --stat HEAD`, and tracked-status verification
before moving to F-12.
