# 0008 MySQL 8.4 Index HAVING Compatibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make all four grouped-index checks in migration 0008 parse and preserve their exact uniqueness semantics on MySQL 8.4, then produce local and isolated-server evidence without touching production.

**Architecture:** Keep the existing 0008 schema state machine and change only the projection/name resolution of grouped `information_schema.statistics` queries. Every query that groups by `index_name, non_unique` will project `non_unique AS is_non_unique` and use that alias in HAVING; a static RED/GREEN contract pins the exact `2/1/1` query distribution across preflight/up/postflight. Local code review and documentation complete first, while the retained failed Compose project remains untouched until a new exact cleanup-and-rerun authorization is granted.

**Tech Stack:** MySQL 8.4 SQL stored procedures, Go 1.22 `testing`, Bash, Docker Compose, Git.

## Global Constraints

- Implement only the approved F-11-discovered F-06/0008 HAVING compatibility follow-up.
- Change exactly four grouped-index queries: preflight has two, up has one, and postflight has one.
- Use `SELECT index_name, non_unique AS is_non_unique`, retain `GROUP BY index_name, non_unique`, and use `is_non_unique` only in the affected HAVING predicates.
- Do not alter any 0008 table, column, index, DDL, DML, fixed guard row, migration order, stable SQLSTATE 45000 message, or business-row behavior.
- Do not modify 0001..0007, 0009, application runtime code, dependencies, or add a down migration.
- The implementation commit may contain only the three 0008 SQL files and `anonymous_upload_governance_migration_test.go`; documentation uses separate commits.
- Use RED -> GREEN, `gofmt` every Go edit, and independently review spec compliance and code quality.
- Keep F-11 test-server status unapproved and F-12 blocked until a newly authorized isolated MySQL 8.4 run exits 0 and its sanitized evidence is audited.
- Do not read, modify, stage, commit, enumerate, or transfer `.tmp/`, `backend/app.db`, or the three protected review documents.
- Do not read or modify production data, credentials, uploads, configuration, logs, mounts, migrations, deployments, or services.
- Do not read, remove, overwrite, restart, or rerun the retained remote failure under the exhausted authorization.

---

## File Map

| Path | Responsibility |
| --- | --- |
| `backend/migrations/anonymous_upload_governance_migration_test.go` | Pin the exact grouped-query count, required alias projection, and HAVING alias predicates. |
| `backend/migrations/0008_anonymous_upload_governance.preflight.sql` | Fix capability-index and quota-guard preflight query parsing without changing rejection semantics. |
| `backend/migrations/0008_anonymous_upload_governance.up.sql` | Fix the resumable existing-quota-guard state query. |
| `backend/migrations/0008_anonymous_upload_governance.postflight.sql` | Fix final quota-guard index verification. |
| `docs/superpowers/reviews/2026-07-28-anonymous-upload-index-having-compatibility-local-verification.md` | Record the remote failure basis, local RED/GREEN, gates, review, and implementation commit. |
| `docs/superpowers/specs/2026-07-28-anonymous-upload-index-having-compatibility-design.md` | Advance compatibility design status only when corresponding evidence exists. |
| `docs/superpowers/specs/2026-07-26-anonymous-upload-resource-governance-design.md` | Record the F-06 compatibility follow-up while retaining test-server/production boundaries. |
| `docs/superpowers/specs/2026-07-27-buyer-intent-open-uniqueness-design.md` | Record that the F-11 run stopped in 0008 and that a new authorized rerun remains required. |
| `docs/release-readiness.md` | Keep F-06/F-11 code, test-server, and production states separate. |
| `docs/full-project-code-review-2026-07-24.md` | Append current follow-up evidence without rewriting the historical finding. |
| `docs/superpowers/reviews/2026-07-27-buyer-intent-open-uniqueness-isolated-acceptance.md` | Record the accepted MySQL 8.4 rerun only after success and leak-scan approval. |

---

### Task 1: Add the RED contract and minimal SQL compatibility fix

**Files:**
- Modify: `backend/migrations/anonymous_upload_governance_migration_test.go`
- Modify: `backend/migrations/0008_anonymous_upload_governance.preflight.sql`
- Modify: `backend/migrations/0008_anonymous_upload_governance.up.sql`
- Modify: `backend/migrations/0008_anonymous_upload_governance.postflight.sql`

**Interfaces:**
- Consumes: the approved `2/1/1` affected-query map and existing 0008 index definitions.
- Produces: `TestAnonymousUploadGovernanceGroupedIndexHavingProjectsNonUnique` and four MySQL-compatible grouped-index checks; no runtime API or schema interface changes.

- [ ] **Step 1: Verify the isolated implementation baseline**

Run from the F-11 linked worktree root:

```bash
git status --short --branch --untracked-files=no
git diff --quiet
git diff --cached --quiet
git merge-base --is-ancestor 50a991d321788ffe77eab9646f4888929b8f5e82 HEAD
```

Expected: branch `codex/f11-buyer-intent-open-uniqueness`, no tracked or staged diff, and the approved written-spec commit is an ancestor. Do not enumerate untracked files.

- [ ] **Step 2: Write the failing grouped-HAVING contract**

Add this test after `TestAnonymousUploadGovernanceMigrationArtifacts`; existing `os`, `regexp`, `strings`, and `testing` imports are sufficient:

```go
func TestAnonymousUploadGovernanceGroupedIndexHavingProjectsNonUnique(t *testing.T) {
	tests := []struct {
		name            string
		groupedQueries  int
		aliasPredicates int
	}{
		{"0008_anonymous_upload_governance.preflight.sql", 2, 4},
		{"0008_anonymous_upload_governance.up.sql", 1, 2},
		{"0008_anonymous_upload_governance.postflight.sql", 1, 2},
	}
	aliasPredicate := regexp.MustCompile(`\bis_non_unique\s*=\s*[01]\b`)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := os.ReadFile(tt.name)
			if err != nil {
				t.Fatalf("read %s: %v", tt.name, err)
			}
			normalized := strings.ToLower(strings.Join(strings.Fields(string(raw)), " "))
			grouped := strings.Count(normalized, "group by index_name, non_unique")
			if grouped != tt.groupedQueries {
				t.Fatalf("grouped index queries = %d, want %d", grouped, tt.groupedQueries)
			}
			projected := strings.Count(normalized,
				"select index_name, non_unique as is_non_unique from information_schema.statistics")
			if projected != tt.groupedQueries {
				t.Fatalf("projected grouped queries = %d, want %d", projected, tt.groupedQueries)
			}
			if predicates := len(aliasPredicate.FindAllString(normalized, -1)); predicates != tt.aliasPredicates {
				t.Fatalf("HAVING alias predicates = %d, want %d", predicates, tt.aliasPredicates)
			}
		})
	}
}
```

This test checks the real SQL artifacts, adds no mock, and fixes both the number of affected queries and all expected unique/non-unique predicates.

- [ ] **Step 3: Run the focused test and capture RED**

```bash
cd backend
gofmt -w migrations/anonymous_upload_governance_migration_test.go
go test ./migrations -run '^TestAnonymousUploadGovernanceGroupedIndexHavingProjectsNonUnique$' -count=1
```

Expected: FAIL in the preflight subtest with `projected grouped queries = 0, want 2`. Record the full command, exit code, and failure line in the task report before editing SQL. A compile error or file-read error is not an accepted RED result.

- [ ] **Step 4: Apply the exact preflight projection fix**

In the capability-index query, use:

```sql
SELECT index_name, non_unique AS is_non_unique
FROM information_schema.statistics
WHERE table_schema = DATABASE()
  AND table_name = 'file_records'
  AND index_name IN ('uk_file_capability_token', 'idx_file_capability_expires')
GROUP BY index_name, non_unique
HAVING (index_name = 'uk_file_capability_token'
    AND is_non_unique = 0
    AND COUNT(*) = 1
    AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = 'capability_token_hash')
  OR (index_name = 'idx_file_capability_expires'
    AND is_non_unique = 1
    AND COUNT(*) = 1
    AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = 'capability_expires_at')
```

In the quota-guard query, use:

```sql
SELECT index_name, non_unique AS is_non_unique
FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = 'file_quota_guards'
GROUP BY index_name, non_unique
HAVING (index_name = 'PRIMARY' AND is_non_unique = 0 AND COUNT(*) = 1
    AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = 'id')
  OR (index_name = 'uk_file_quota_guard_name' AND is_non_unique = 0 AND COUNT(*) = 1
    AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = 'guard_name')
```

Do not change the surrounding `SELECT COUNT(*) INTO v_count`, derived-table aliases, `IF v_count` checks, messages, or any WHERE-only `non_unique` filters.

- [ ] **Step 5: Apply the same exact quota-guard fix to up and postflight**

Replace each file's one affected inner query with the quota-guard query shown in Step 4. Preserve indentation local to the stored procedure and leave every DDL/DML statement unchanged.

- [ ] **Step 6: Run the focused test and migration suites for GREEN**

```bash
cd backend
gofmt -w migrations/anonymous_upload_governance_migration_test.go
go test ./migrations -run '^TestAnonymousUploadGovernanceGroupedIndexHavingProjectsNonUnique$' -count=1
go test ./migrations -run 'TestAnonymousUploadGovernance|TestBuyerIntentOpenUniqueness' -count=1
go test ./migrations -count=1
```

Expected: all three commands PASS. The focused test must report all three SQL-file subtests as passing.

- [ ] **Step 7: Prove script syntax and scope**

```bash
cd ..
bash -n deploy/acceptance/anonymous-upload-governance-smoke.sh
bash -n deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh
git diff --check
rg -c 'GROUP BY index_name, non_unique' \
  backend/migrations/0008_anonymous_upload_governance.preflight.sql \
  backend/migrations/0008_anonymous_upload_governance.up.sql \
  backend/migrations/0008_anonymous_upload_governance.postflight.sql
rg -c 'SELECT index_name, non_unique AS is_non_unique' \
  backend/migrations/0008_anonymous_upload_governance.preflight.sql \
  backend/migrations/0008_anonymous_upload_governance.up.sql \
  backend/migrations/0008_anonymous_upload_governance.postflight.sql
git diff --name-only
```

Expected: both scripts parse; both searches report preflight `2`, up `1`, postflight `1`; only the four Task 1 files appear in `git diff --name-only`. Inspect the SQL diff and reject any DDL, DML, message, table, index, or column change.

- [ ] **Step 8: Commit the implementation boundary**

```bash
git add backend/migrations/0008_anonymous_upload_governance.preflight.sql \
  backend/migrations/0008_anonymous_upload_governance.up.sql \
  backend/migrations/0008_anonymous_upload_governance.postflight.sql \
  backend/migrations/anonymous_upload_governance_migration_test.go
git diff --cached --check
git diff --cached --name-only
git commit -m "fix(migrations): make 0008 index checks MySQL compatible"
```

Expected: the staged-name list contains exactly the four implementation files and the commit succeeds.

---

### Task 2: Run full local gates, independent review, and local traceability docs

**Files:**
- Create: `docs/superpowers/reviews/2026-07-28-anonymous-upload-index-having-compatibility-local-verification.md`
- Modify: `docs/superpowers/specs/2026-07-28-anonymous-upload-index-having-compatibility-design.md`
- Modify: `docs/superpowers/specs/2026-07-26-anonymous-upload-resource-governance-design.md`
- Modify: `docs/superpowers/specs/2026-07-27-buyer-intent-open-uniqueness-design.md`
- Modify: `docs/release-readiness.md`
- Modify: `docs/full-project-code-review-2026-07-24.md`
- Modify only after a review finding: the four Task 1 implementation files

**Interfaces:**
- Consumes: the reviewed Task 1 implementation commit and captured RED/GREEN report.
- Produces: independently reviewed local evidence and an exact implementation commit range; server status remains pending.

- [ ] **Step 1: Run fresh focused and full backend gates**

```bash
git status --short --branch --untracked-files=no
cd backend
mkdir -p .cache/go/mod .cache/go/build
go test ./migrations -run '^TestAnonymousUploadGovernanceGroupedIndexHavingProjectsNonUnique$' -count=1
go test ./migrations -run 'TestAnonymousUploadGovernance|TestBuyerIntentOpenUniqueness' -count=1
env GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" go test ./... -count=1
env GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" go test -race ./... -count=1
env GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" go vet ./...
cd ..
bash -n deploy/acceptance/anonymous-upload-governance-smoke.sh
bash -n deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh
git diff --check
```

Expected: every command exits 0. The opt-in MySQL tests may report their documented local SKIP; no SSH, Docker, or remote evidence access occurs in this step.

- [ ] **Step 2: Request independent spec and code review**

Run `git rev-parse HEAD`, copy its full output verbatim into the review request,
then invoke `superpowers:requesting-code-review`. Give the reviewer:

```text
Scope base: 50a991d321788ffe77eab9646f4888929b8f5e82
Scope head: the exact full hash just returned by git rev-parse HEAD
Spec: docs/superpowers/specs/2026-07-28-anonymous-upload-index-having-compatibility-design.md
Prior failure: isolated MySQL 8.4.8 ERROR 1054 in 0008 preflight before 0009
Review priorities:
  exactly four grouped queries, distributed 2/1/1
  alias projection and HAVING use match the approved pattern
  index names, column order, uniqueness counts, messages, DDL and DML unchanged
  regression test fails when the SQL alias fix is removed
  no change outside the four implementation files
  no protected, remote, production, credential, upload or evidence access
Output: Spec verdict plus Critical, Important, and Minor findings with file:line evidence
```

The review must report both spec compliance and code quality. Test output from Step 1 is supplied in the task report; the reviewer does not need to rerun it.

- [ ] **Step 3: Resolve review findings through the review loop**

Invoke `superpowers:receiving-code-review` before applying feedback. For every confirmed Critical or Important finding:

1. Add or tighten a test and run it to an expected RED.
2. Apply the smallest in-scope correction.
3. Run the focused test and affected migration suite to GREEN.
4. Commit the correction separately.
5. Request a scoped re-review of the finding and correction diff.

Do not weaken the approved `2/1/1` contract or expand into another migration. A finding that conflicts with the written spec returns to the user for a governing decision.

- [ ] **Step 4: Re-run final local proof after review**

Repeat every command from Step 1 after the last review fix. Also run:

```bash
git show --check --stat HEAD
git status --short --branch --untracked-files=no
git log --oneline 50a991d..HEAD
```

Expected: gates exit 0, tracked worktree is clean, and the log contains only the compatibility implementation and any reviewed correction commits.

- [ ] **Step 5: Write the local verification report**

Create `docs/superpowers/reviews/2026-07-28-anonymous-upload-index-having-compatibility-local-verification.md` with these exact evidence categories and actual command outputs copied without secrets:

```text
Date and branch
Approved design/spec commit: 50a991d321788ffe77eab9646f4888929b8f5e82
Implementation and reviewed correction commit IDs from git log
Remote attempt basis: e55be6a, 120 files, manifest SHA-256
  b9b230c6706bfb399ad2679b92c4ca3a58d6f176ca18176dcd641c3a1cccc226
Remote result: MySQL 8.4.8, exit 2, 0008 preflight ERROR 1054, 0009 not entered
Local isolated parser proof: failing unprojected query; alias query exit 0/count 2
RED focused command, exit code, and expected projected-query failure
GREEN focused/migration/full/race/vet/bash/diff commands and results
Independent reviewer scope, verdict, and finding disposition
Four-query 2/1/1 count proof
Remote status: retained failure not read, removed, restarted, or rerun
Production status: 0008/0009 not run; no deployment or production data/config/service/upload change
```

Do not include DSNs, credentials, tokens, row identifiers, contacts, raw production details, raw remote logs, or retained evidence content.

- [ ] **Step 6: Advance only local code-side wording**

In the compatibility design, set status to:

```text
方案、完整设计与书面规格已批准；兼容性代码侧已修复并通过独立审阅；隔离 MySQL 8.4 重跑待授权；生产 0008/0009 未执行
```

In the F-06 design and release-readiness F-06 row, record:

```text
0008 HAVING 兼容性跟进已通过本地门禁和独立审阅；隔离 MySQL 8.4 测试服务器仍未审核；生产未执行 0008、未部署、未修改生产数据或文件
```

In the F-11 design and release-readiness F-11 row, record:

```text
F-11 code-side fixed; the authorized isolated run stopped in 0008 before 0009; the 0008 compatibility correction passed local gates; a new isolated MySQL 8.4 rerun is pending; production 0009 not executed.
```

Append the same dated facts to `docs/full-project-code-review-2026-07-24.md` without rewriting historical evidence. F-12 remains blocked.

- [ ] **Step 7: Commit the local evidence documentation**

```bash
git add docs/superpowers/reviews/2026-07-28-anonymous-upload-index-having-compatibility-local-verification.md \
  docs/superpowers/specs/2026-07-28-anonymous-upload-index-having-compatibility-design.md \
  docs/superpowers/specs/2026-07-26-anonymous-upload-resource-governance-design.md \
  docs/superpowers/specs/2026-07-27-buyer-intent-open-uniqueness-design.md \
  docs/release-readiness.md \
  docs/full-project-code-review-2026-07-24.md
git diff --cached --check
git commit -m "docs(migrations): record 0008 compatibility verification"
git show --check --stat HEAD
git status --short --branch --untracked-files=no
```

Expected: documentation commit succeeds and the tracked worktree is clean. Do not mark test-server acceptance passed.

---

### Task 3: Obtain new authorization and rerun the isolated F-11 MySQL 8.4 project once

**Files:**
- No repository files before the run.
- Read only after success and leak-scan pass: `/home/yu/services/secondhand-buyer-intent-acceptance-20260727/deploy/acceptance/evidence/buyer-intent-open-uniqueness/*.txt`

**Interfaces:**
- Consumes: clean reviewed compatibility HEAD, committed whitelist, retained failed project, and a new exact user authorization.
- Produces: one successful isolated run and sanitized retained evidence; any failure retains the new project unchanged and consumes the authorization.

- [ ] **Step 1: Stop and request a new exact cleanup-and-rerun authorization**

The request must name all of the following:

```text
Host: aliyun-server
Exact directory: /home/yu/services/secondhand-buyer-intent-acceptance-20260727
Exact Compose project: secondhand-buyer-intent-acceptance
Allowed cleanup: read-only resolve resources carrying exactly that Compose project label;
  delete only those validated container/volume/network IDs and only the exact directory;
  recreate the directory with mode 0700
Allowed transfer: only committed paths emitted by BUYER_INTENT_SOURCE_LIST_ONLY=1
Allowed run: regenerate remote-only secrets and run the dedicated Compose project once
Allowed evidence: only after exit 0 and forbidden_matches=0, read the newly generated sanitized evidence
Production read-only scope: only the three fixed container names and their name/ID/state/restart count
Forbidden: every other Docker resource, .env/secrets contents, databases, uploads, backups,
  prior evidence contents, .git, caches, node_modules, backend/app.db, .tmp, protected review docs,
  production SQL/logs/env/mounts/config/migrations/deployment/service/data changes
Failure rule: retain everything; no cleanup, overwrite, restart, or rerun without another exact authorization
```

Do not perform SSH or Docker actions until the user explicitly approves this new request.

- [ ] **Step 2: Verify the clean accepted local source after approval**

```bash
git diff --quiet
git diff --cached --quiet
git status --short --branch --untracked-files=no
git rev-parse HEAD
```

Create a fresh directory with `mktemp -d`. Generate new `source-files.z` and `local-source-sha256.txt` from the script's two read-only modes. For every NUL-delimited path, require:

```bash
git ls-files --error-unmatch -- "$source_path"
git cat-file -e "$(git rev-parse HEAD):$source_path"
```

Also require sorted uniqueness, regular committed files, working-tree blob equality, exact list/manifest path equality, and explicit rejection of `.env`, secrets, databases, uploads, evidence, backups, `.git`, caches, `node_modules`, `backend/app.db`, `.tmp`, and protected review documents. Record file count and the SHA-256 of the manifest file.

- [ ] **Step 3: Resolve and delete only the authorized retained project**

On `aliyun-server`, set literal values and validate them before deletion:

```bash
target=/home/yu/services/secondhand-buyer-intent-acceptance-20260727
project=secondhand-buyer-intent-acceptance
[[ "$target" == /home/yu/services/secondhand-buyer-intent-acceptance-20260727 ]]
[[ "$project" == secondhand-buyer-intent-acceptance ]]
[[ -d "$target" && ! -L "$target" ]]
```

Resolve and validate exactly the resources created by the failed run. The run
output already established one MySQL container, one named volume, and one named
network; any different count or name stops cleanup:

```bash
mapfile -t container_ids < <(
  docker container ls -aq --filter \
    "label=com.docker.compose.project=secondhand-buyer-intent-acceptance"
)
mapfile -t volume_names < <(
  docker volume ls -q --filter \
    "label=com.docker.compose.project=secondhand-buyer-intent-acceptance"
)
mapfile -t network_names < <(
  docker network ls -q --filter \
    "label=com.docker.compose.project=secondhand-buyer-intent-acceptance"
)
[[ "${#container_ids[@]}" -eq 1 ]]
[[ "${#volume_names[@]}" -eq 1 ]]
[[ "${#network_names[@]}" -eq 1 ]]

container_id="${container_ids[0]}"
container_record="$(docker inspect --type container --format \
  '{{.Id}}|{{.Name}}|{{ index .Config.Labels "com.docker.compose.project" }}|{{.State.Running}}' \
  "$container_id")"
IFS='|' read -r inspected_id container_name container_project container_running \
  <<<"$container_record"
[[ "$inspected_id" == "$container_id" ]]
[[ "$container_name" == /secondhand-buyer-intent-acceptance-mysql-1 ]]
[[ "$container_project" == secondhand-buyer-intent-acceptance ]]
[[ "$container_running" == false ]]

volume_name="${volume_names[0]}"
[[ "$volume_name" == secondhand-buyer-intent-acceptance_mysql-data ]]
[[ "$(docker volume inspect --format \
  '{{ index .Labels "com.docker.compose.project" }}' "$volume_name")" \
  == secondhand-buyer-intent-acceptance ]]

network_name="${network_names[0]}"
[[ "$network_name" == secondhand-buyer-intent-acceptance_acceptance ]]
[[ "$(docker network inspect --format \
  '{{ index .Labels "com.docker.compose.project" }}' "$network_name")" \
  == secondhand-buyer-intent-acceptance ]]

docker container rm -- "$container_id"
docker volume rm -- "$volume_name"
docker network rm -- "$network_name"

test -z "$(docker container ls -aq --filter \
  'label=com.docker.compose.project=secondhand-buyer-intent-acceptance')"
test -z "$(docker volume ls -q --filter \
  'label=com.docker.compose.project=secondhand-buyer-intent-acceptance')"
test -z "$(docker network ls -q --filter \
  'label=com.docker.compose.project=secondhand-buyer-intent-acceptance')"
```

Do not call a broad prune, `docker compose down` against another directory, or
delete any other Docker resource.

After Docker resources are empty, delete only the already validated literal target, recreate it, and verify its mode:

```bash
rm -rf -- /home/yu/services/secondhand-buyer-intent-acceptance-20260727
install -d -m 0700 /home/yu/services/secondhand-buyer-intent-acceptance-20260727
test "$(stat -c %a /home/yu/services/secondhand-buyer-intent-acceptance-20260727)" = 700
```

- [ ] **Step 4: Dry-run, transfer, and hash the exact whitelist**

Use the fresh NUL list as rsync's only source selector:

```bash
rsync -anvi --from0 --files-from="$source_files" --relative --out-format='%i|%n' ./ \
  aliyun-server:/home/yu/services/secondhand-buyer-intent-acceptance-20260727/
```

Parse the dry-run itemization and require its file set to equal the NUL whitelist byte-for-byte; every directory must be an ancestor of a whitelisted file. Then run the identical rsync command without `-n`, generate the remote manifest via `BUYER_INTENT_SOURCE_MANIFEST_ONLY=1`, and require `cmp -s` against the local manifest. Also require the complete remote regular-file list to equal the whitelist and zero remote symlinks. Stop before `prepare.sh` on any mismatch.

- [ ] **Step 5: Prepare and consume the one-run authorization exactly once**

Run:

```bash
ssh aliyun-server 'cd /home/yu/services/secondhand-buyer-intent-acceptance-20260727 && ./deploy/acceptance/prepare.sh'
```

Do not read generated `.env` or secrets. Create a local non-overwritable run marker, then execute exactly once:

```bash
ssh aliyun-server 'cd /home/yu/services/secondhand-buyer-intent-acceptance-20260727 && env COMPOSE_PROJECT_NAME=secondhand-buyer-intent-acceptance BUYER_INTENT_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_BUYER_INTENT_DATA ACCEPTANCE_DB_ENGINE=mysql8.4 make acceptance-buyer-intent-smoke'
```

If the command is nonzero, retain all new resources and evidence, do not read evidence, and stop for another exact authorization. Never rerun this command under the same approval.

- [ ] **Step 6: Gate and audit only successful sanitized evidence**

Only after the command exits 0, first require:

```text
evidence-leak-scan.txt: forbidden_matches=0
evidence-sha256.txt: every listed file verifies
production-before.txt and production-after.txt: byte-identical
```

Then read only the new files under
`deploy/acceptance/evidence/buyer-intent-open-uniqueness`. Prove:

```text
mysql-version.txt reports 8.4.x
legacy.txt, marker-only.txt, both-keys.txt and final-rerun.txt pass with unchanged row summaries
five invalid-state files plus duplicate-open, drifted-marker, drifted-key and unknown-partial report ERROR 1644 (45000) and unchanged summaries
api-matrix-schema.txt has the exact generated marker and unique key
mysql-auto-migrate-false.txt and mysql-auto-migrate-true.txt pass
backend-tests.txt, backend-race.txt and go-vet.txt pass
resource-retention.txt names only secondhand-buyer-intent-acceptance
source-sha256.txt matches the accepted local source manifest
production snapshots contain only allowed fixed-container fields and are identical
```

Keep the dedicated project resources and evidence retained after audit. Do not execute production 0008/0009 or deploy.

---

### Task 4: Record accepted server evidence and unblock F-12

**Files:**
- Create: `docs/superpowers/reviews/2026-07-27-buyer-intent-open-uniqueness-isolated-acceptance.md`
- Modify: `docs/superpowers/specs/2026-07-28-anonymous-upload-index-having-compatibility-design.md`
- Modify: `docs/superpowers/specs/2026-07-26-anonymous-upload-resource-governance-design.md`
- Modify: `docs/superpowers/specs/2026-07-27-buyer-intent-open-uniqueness-design.md`
- Modify: `docs/release-readiness.md`
- Modify: `docs/full-project-code-review-2026-07-24.md`

**Interfaces:**
- Consumes: audited successful Task 3 evidence, exact accepted HEAD/range, and local/remote manifests.
- Produces: accepted F-11 test-server status and the commit-range prerequisite for F-12; production remains pending.

- [ ] **Step 1: Write the sanitized acceptance report**

Record actual values for:

```text
date, host alias, exact remote path, Compose project, MySQL/tool versions
approved design/spec commit and complete accepted implementation HEAD/range
attempt 1 manifest SHA-256 8d7fed97ae7e295c8335b9b90ce2b29c0b2a1a8bfb6c48657d90ec52fadc9496 and pre-service Compose parse failure
attempt 2 manifest SHA-256 b9b230c6706bfb399ad2679b92c4ca3a58d6f176ca18176dcd641c3a1cccc226 and 0008 ERROR 1054 failure
successful rerun local/remote manifest SHA-256 and byte equality
0008 compatibility passage before the 0009 matrix
every success/rejection/API/full/race/vet evidence category and file hash
forbidden_matches=0 and evidence manifest verification
production snapshot equality and retained resource/evidence location
production 0008/0009 not run; no deployment or production data/config/service/upload/session change
```

Do not include DSNs, credentials, tokens, contacts, row identifiers, raw request data, raw logs, or production rows.

- [ ] **Step 2: Advance server status without claiming production completion**

Use this compatibility status:

```text
0008 HAVING compatibility fixed and passed the isolated MySQL 8.4 test-server chain; production 0008 was not executed and no production deployment occurred.
```

Use this F-11 status:

```text
F-11 fixed and passed isolated MySQL 8.4 test-server review; production 0009 not executed and production not deployed.
```

Record the exact accepted F-11 commit range in the F-11 design and acceptance report. Change F-12 only from “blocked pending F-11 evidence” to “prerequisite satisfied by the recorded accepted range”; do not implement F-12 in this task.

- [ ] **Step 3: Commit accepted evidence and status**

```bash
git add docs/superpowers/reviews/2026-07-27-buyer-intent-open-uniqueness-isolated-acceptance.md \
  docs/superpowers/specs/2026-07-28-anonymous-upload-index-having-compatibility-design.md \
  docs/superpowers/specs/2026-07-26-anonymous-upload-resource-governance-design.md \
  docs/superpowers/specs/2026-07-27-buyer-intent-open-uniqueness-design.md \
  docs/release-readiness.md \
  docs/full-project-code-review-2026-07-24.md
git diff --cached --check
git commit -m "docs(buyer): record F-11 server acceptance"
```

- [ ] **Step 4: Run final compatibility/F-11 handoff proof**

```bash
git show --check --stat HEAD
git status --short --branch --untracked-files=no
git log --oneline 50a991d..HEAD
test -f backend/migrations/0008_anonymous_upload_governance.preflight.sql
test -f backend/migrations/0008_anonymous_upload_governance.up.sql
test -f backend/migrations/0008_anonymous_upload_governance.postflight.sql
test ! -e backend/migrations/0008_anonymous_upload_governance.down.sql
test -f backend/migrations/0009_buyer_intent_open_uniqueness.preflight.sql
test -f backend/migrations/0009_buyer_intent_open_uniqueness.up.sql
test -f backend/migrations/0009_buyer_intent_open_uniqueness.postflight.sql
test ! -e backend/migrations/0009_buyer_intent_open_uniqueness.down.sql
```

Expected: all commands exit 0, tracked worktree is clean, test-server status is accepted, production remains untouched, and F-12 may start from the recorded accepted range.

- [ ] **Step 5: Remove only recorded local transfer scratch after the report is committed**

The final report must contain the corresponding manifest hashes before removal. If these exact directories still exist and are non-symlinks under the macOS temporary root, remove only them:

```text
/var/folders/yz/syjs23ds7nx4cw57q5dbqdwh0000gn/T/f11-transfer.XXXXXX.BNN97wULF4
/var/folders/yz/syjs23ds7nx4cw57q5dbqdwh0000gn/T/f11-transfer.XXXXXX.2RYGLZYvrY
```

Also remove only Task 3's freshly recorded `mktemp` directory after validating its exact path and report hash. Do not remove repository `.tmp/`, remote resources, remote evidence, or any broader temporary directory. Report the exact local scratch paths removed.

Do not merge or finish the overall first-round branch here. Continue with F-12, F-15, F-10's separate authorization boundary, and the final requirement-by-requirement audit.
