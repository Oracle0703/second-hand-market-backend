# F-04/F-05/F-06/F-14 Acceptance Provenance and Evidence Hardening Design

**Date:** 2026-07-28

**Branch:** `codex/f15-idempotency-atomicity`

**Status:** Architecture approved on 2026-07-28. This written specification is
awaiting approval. Implementation has not started.

**Approval record:** The user explicitly approved the complete architecture for
Unified Approach A on 2026-07-28. That approval authorizes this specification;
it does not authorize implementation or a test-server run before this written
specification is approved.

## 1. Problem Statement

The F-04/F-13, F-05, F-06, and F-14 behavior fixes have acceptance harnesses,
but those harnesses do not yet provide one consistent, reviewable proof that a
server run used only bytes from an exact committed `HEAD` and retained no raw or
sensitive output.

The current gaps are:

- `license-file-privacy-smoke.sh` has no committed-`HEAD` package binding,
  permits evidence-directory reuse, retains raw migration and test output, and
  has no leak-scanned failure-evidence path.
- `miniapp-auth-refresh-smoke.sh` has no immutable package, permits evidence
  reuse, runs `npm ci` and builds in transferred source, retains raw npm/test/
  build logs, and has no evidence leak scan or sanitized failure path.
- `anonymous-upload-governance-smoke.sh` already refuses project and evidence
  collisions, but derives its source manifest from working-tree `find`, has no
  immutable package import contract, and retains more raw output than is needed
  for review.
- `session-revocation-smoke.sh` already refuses collisions and classifies most
  results, but its source list and archive still come from working-tree
  `find`/`tar` and it has no four-file committed-`HEAD` package contract.

This design hardens provenance and evidence handling without changing the
business behavior exercised by any harness.

## 2. Goal

For each affected finding, make the isolated acceptance result traceable to one
reviewed commit and one authorized package digest. A successful or failed run
must leave only classified, leak-scanned, hashed evidence. No dirty, staged,
untracked, generated, secret, database, upload, or pre-existing evidence byte
may enter the tested source or retained evidence.

The result must let a reviewer answer all of the following from committed
documentation and retained sanitized evidence:

1. Which commit supplied the tested bytes?
2. Which exact files were authorized and transferred?
3. Did the received package and temporary test tree match those bytes?
4. Which behavioral gates ran, and how were their results classified?
5. Did the isolated run leave the three permitted production-container
   observations byte-identical before and after?
6. Did the evidence leak scan and evidence hash verification pass?

## 3. Scope

This design covers four existing harnesses:

| Finding | Harness | Source prefix | Fixed remote directory | Compose project |
| --- | --- | --- | --- | --- |
| F-04/F-13 | `license-file-privacy-smoke.sh` | `LICENSE_FILE_PRIVACY` | `/home/yu/services/secondhand-license-privacy-acceptance-20260726` | `secondhand-license-privacy-acceptance` |
| F-05 | `miniapp-auth-refresh-smoke.sh` | `MINIAPP_AUTH_REFRESH` | `/home/yu/services/secondhand-miniapp-auth-refresh-acceptance-20260726` | None |
| F-06 | `anonymous-upload-governance-smoke.sh` | `ANONYMOUS_UPLOAD_GOVERNANCE` | `/home/yu/services/secondhand-upload-governance-acceptance-20260726` | `secondhand-upload-governance-acceptance` |
| F-14 | `session-revocation-smoke.sh` | `SESSION_REVOCATION` | `/home/yu/services/secondhand-session-revocation-acceptance-20260727` | `secondhand-session-revocation-acceptance` |

F-13 shares the F-04 license-file privacy harness and evidence set. F-05 uses
Node `22.22.2` and npm `10.9.7` and uses no Docker, API, or database.

## 4. Non-Goals

- Do not alter the F-04/F-13, F-05, F-06, or F-14 application behavior,
  migrations, API contracts, test assertions, or pass criteria.
- Do not extract a shared shell library. Each harness owns a separate,
  protocol-compatible implementation so one finding can be reviewed and run
  without coupling its trust boundary to another harness.
- Do not refactor `idempotency-atomicity-smoke.sh` or its F-15 contract tests.
  F-15 is the reference protocol and retains its separately documented
  corrective-run path.
- Do not run a test server, transfer source, create remote directories, generate
  remote secrets, or inspect production as part of implementation. Those are
  later actions requiring an exact, consolidated authorization.
- Do not rewrite Git history, remove `backend/app.db` from historical commits,
  deploy a bundle, modify a production container, or execute production SQL.

## 5. Approaches Considered

### 5.1 Adopted: separate protocol-compatible implementation per harness

Each script implements the same source-package, verification, collision,
temporary-runtime, evidence-classification, and cleanup protocol using its own
prefix, allowlist, behavioral commands, and evidence schema.

This keeps the failure boundary local: a later edit to one harness cannot
silently change another harness. It also lets focused contract tests validate
each script as a standalone artifact. The limited duplication is intentional
because these scripts are security-sensitive review tools, not a general shell
framework.

### 5.2 Rejected: shared sourced shell library

A common library would reduce line count, but every acceptance package and
review would then depend on a fifth executable file and its sourcing rules. It
would broaden each change's review surface and make remote package authorization
less obvious.

### 5.3 Rejected: one orchestrator for all findings

A single runner would mix unrelated Node-only and Docker/MySQL lifecycles,
increase the authority of one script, and make partial reruns and evidence
ownership ambiguous. Consolidated authorization does not require consolidated
execution code.

## 6. Common Source Package Protocol

### 6.1 Public environment interface

Every harness adds exactly two source-mode variables and two normal-run
variables under its table prefix:

```text
<PREFIX>_SOURCE_LIST_ONLY=1
<PREFIX>_SOURCE_EXPORT_DIR=<absent absolute directory>
<PREFIX>_SOURCE_PACKAGE_DIR=<absolute directory>
<PREFIX>_SOURCE_PACKAGE_MANIFEST_SHA256=<64 lowercase hexadecimal characters>
```

List-only mode and export mode are mutually exclusive. Source modes execute
before confirmation, `.env`, Docker, Node, npm, network, evidence, and test
checks. They exit immediately after producing their output and must not create
or modify any repository file.

Normal mode requires `SOURCE_PACKAGE_DIR` to be an absolute, existing,
non-symlink directory and requires the out-of-band manifest digest. There is no
fallback to an implicit package directory because an implicit path could bind a
run to stale files.

### 6.2 Committed source authority

The current Git `HEAD` tree is the only source authority:

1. Enumerate candidate paths with `git ls-tree -r --name-only -z HEAD`.
2. Apply the harness-specific allowlist and the common forbidden-path rules.
3. Sort the NUL-delimited result uniquely with bytewise `C` ordering.
4. Obtain all file bytes with `git archive HEAD -- <approved paths>`.

The exporter must not read file content from the index or working tree. A
staged-only path, dirty replacement, untracked file, generated dependency, or
local build output cannot change either the list or archived bytes.

### 6.3 Package format

Every export directory contains exactly four regular, non-symlink files:

```text
source-files.z
source-sha256.txt
source.tar
package-sha256.txt
```

Their contract is:

- `source-files.z` is the sorted, unique, non-empty, NUL-delimited path list.
- `source-sha256.txt` contains one lowercase SHA-256 and relative path for each
  listed regular file, in source-list order, computed from a temporary
  extraction of `source.tar`.
- `source.tar` is produced by `git archive` from the same `HEAD` and exact path
  array. It contains no `.git` metadata.
- `package-sha256.txt` contains exactly three lines, in this order, for
  `source-files.z`, `source-sha256.txt`, and `source.tar`. It never hashes
  itself.

The exporter requires an absent absolute directory other than `/`, creates it
with mode `0700`, gives the four artifacts mode `0600`, validates the archive
against the list and per-file manifest, and removes all temporary extraction
state on success or failure. If export fails after creating the destination, it
also removes that incomplete destination so it cannot be mistaken for an
authorized package.

The SHA-256 of `package-sha256.txt` is the authorization digest. It is recorded
out of band with the exact final commit and supplied in normal mode through
`SOURCE_PACKAGE_MANIFEST_SHA256`.

### 6.4 Path policy

Every path must be relative, non-empty, free of `..` traversal, allowed by the
specific harness, and not forbidden by the common policy. Matching is
case-insensitive for forbidden path components. A path may contain only the
portable committed-source character set `[A-Za-z0-9._/-]`, may not begin with
`-`, and may not contain an empty component.

The common policy rejects:

- `.git`, `.tmp`, `.cache`, caches, `node_modules`, evidence, backups, secrets,
  databases, uploads, and any `.env` name;
- database-shaped files including `.db`, `.sqlite`, and `.sqlite3` variants;
- `backend/app.db`, `miniapp/project.private.config.json`, generated miniapp
  output, and every `docs/superpowers` path;
- the three protected review documents and any path outside the explicitly
  allowed source roots.

Harness-specific source sets are:

- F-04/F-13: `Makefile`, required backend Go/module/Docker/migration source,
  and required `deploy/acceptance` scripts, Compose/configuration, README, and
  SQL source.
- F-05: `Makefile`, `miniapp/.nvmrc`, `miniapp/babel.config.js`,
  `miniapp/config`, `miniapp/package.json`, `miniapp/package-lock.json`,
  `miniapp/project.config.json`, `miniapp/project.tt.json`, `miniapp/src`,
  `miniapp/tests`, `miniapp/tsconfig.json`, `miniapp/vitest.config.mjs`, and the
  F-05 acceptance script. `miniapp/project.private.config.json`, `.swc`,
  `node_modules`, and `dist` are never allowed.
- F-06: `Makefile`, required backend and frontend source/configuration, and
  required `deploy/acceptance` scripts, Compose/configuration, README, and SQL
  source. No frontend dependency or build-output directory is allowed.
- F-14: `Makefile`, required backend Go/module/Docker/migration source, and
  required `deploy/acceptance` scripts, Compose/configuration, README, and SQL
  source.

Each implementation defines a small explicit list of paths that must be
present in addition to the allowed patterns. An empty or incomplete allowlist
fails closed. F-05 must at minimum require `.nvmrc`, both package files, its
refresh implementation and focused test, both public project configs, the
acceptance script, and `Makefile`.

## 7. Normal-Mode Verification Order

Normal mode follows this order. A failure cannot skip forward to a later stage.

1. Validate the exact confirmation and fixed engine/project values that apply
   to the harness.
2. Validate only local provenance-tool availability and the four source
   environment variables.
3. Require all four package artifacts to be regular non-symlink files.
4. Compare the lowercase out-of-band digest with the actual SHA-256 of
   `package-sha256.txt`.
5. Parse `package-sha256.txt` strictly and verify all three package checksums.
6. Validate sorted uniqueness, path safety, allowlist membership, forbidden
   exclusions, and required paths in `source-files.z`.
7. Extract `source.tar` into a new mode-`0700` temporary directory without
   following or accepting links.
8. Require the extracted tree to contain exactly the listed regular files and
   no additional path, then recompute and compare every per-file SHA-256.
9. Require the package to have been unpacked into the dedicated remote source
   directory. Every received listed path must be a regular non-symlink file and
   its recomputed hash must match the same manifest. These received files are
   verified for transfer integrity but are never used as the build/test tree.
10. Require the retained evidence path to be absent. Docker harnesses then use
    Docker only to refuse any pre-existing container, volume, or network with
    the exact Compose project label.
11. Only after every prior check may the harness inspect the exact toolchain,
    snapshot the permitted production metadata, start Docker, invoke npm, use
    the network, or run a test.

Every build and test runs from the verified temporary extraction, never from
the transferred source tree. Compose receives a temporary override that points
its build contexts at that extraction. F-05 runs `npm ci`, tests, and both builds
inside the temporary extraction; removing the temporary directory removes
`node_modules`, build output, and raw logs on every exit.

The package, transferred committed source, remote-only `.env`/secrets, stopped
isolated project resources, and sanitized evidence may remain for review only
when the later authorization explicitly permits retention.

## 8. Harness Behavior Preservation

Provenance hardening wraps, but does not weaken or reorder, each approved
behavior matrix:

- F-04/F-13 retains dirty `0007` preflight rejection, clean `0007..0009`
  migration verification, private-license API behavior with both
  `AUTO_MIGRATE=false` and `AUTO_MIGRATE=true`, and historical-row invariants.
- F-05 retains the exact Node/npm lock, public npm registry restriction,
  `example.invalid` API base URL, focused refresh tests, full miniapp tests, and
  WeChat and Douyin builds. It starts no API and uses no Docker or database.
- F-06 retains dirty and clean `0008` migration checks, historical row/file
  fingerprints, MySQL concurrency and cleanup behavior, backend/frontend gates,
  and exact 10 MiB file and 11 MiB request boundaries through API and proxy.
- F-14 retains the complete `0001..0009` chain, both AutoMigrate modes, the
  ADMIN/MERCHANT/BUYER revocation matrix, full backend tests, and `go vet`.
  `backend/tests/session_revocation_acceptance_contract_test.go` remains the
  behavioral ordering authority.

No pass classification may be created merely from a command exit code when the
current harness already requires a specific migration marker, test name,
version, count, HTTP status, business code, hash, or before/after comparison.

## 9. Runtime And Failure Semantics

### 9.1 Temporary state

Raw migration, Docker, npm, build, test, vet, HTTP response, and diagnostic
output stays under one temporary runtime directory. The exit trap removes that
directory on success, controlled failure, interruption, or sanitization
failure. Raw output is never copied to retained evidence.

### 9.2 Isolated resource cleanup

Docker harnesses track whether their exact project was touched. On exit they
stop only containers belonging to that fixed Compose project. They do not run
global prune, broad label deletion, or remove any other container, network, or
volume. Project resources are retained stopped for review unless a later exact
authorization says otherwise.

F-05 removes only its temporary verified test copy and generated content. It
does not delete or change the received immutable package or committed source.

### 9.3 Production observation boundary

The three Docker harnesses may inspect only these production containers:

```text
secondhand-market-api
secondhand-market-web
secondhand-market-mysql
```

For each, the only allowed fields are name, container ID, state, and restart
count. Absence is represented by a fixed classified value. A before snapshot is
taken immediately before the isolated project is touched. Success and failure
paths both attempt an after snapshot, and the run cannot pass unless the two
validated snapshots are byte-identical.

No production SQL, logs, environment variables, mounts, configuration,
services, migrations, deployment, filesystem, uploads, secrets, or data may be
read or modified.

### 9.4 Sanitized retained evidence

The retained evidence directory must be absent before a run. It is created only
by publishing a completed sanitized set from temporary storage.

Retained evidence may contain only:

- strict `classification=<name>|result=<PASS|FAIL>|count=<integer>` records,
  optionally with one `stage=<fixed-lowercase-name>` field before `count` or
  one SHA-256 field after `count`;
- approved toolchain/version summaries that contain no connection or host
  details;
- approved historical-state or request-boundary fingerprints already required
  by the behavioral matrix and containing no raw identifiers;
- validated production before/after snapshots for Docker harnesses;
- `evidence-leak-scan.txt` and `evidence-sha256.txt`.

Success publishes only after all classifications and snapshots validate, the
production snapshots match where applicable, and a recursive forbidden-pattern
scan reports zero matches. `evidence-sha256.txt` hashes every other retained
regular file in deterministic path order.

After provenance preflight has completed, a runtime failure publishes only a
sanitized failure set: validated completed classifications, any complete
approved snapshots, one fixed failure-stage classification, a passing leak-scan
classification, and the evidence hash manifest. If any candidate classification
or snapshot is malformed, or sanitization cannot be proven, the harness
publishes only the hardcoded
`classification=evidence_sanitization|result=FAIL|stage=evidence_sanitization|count=1`
and `classification=evidence_scan|result=FAIL|count=1` records plus their hash
manifest. Preflight failures before an eligible run create no evidence
directory.

The leak patterns are tailored per harness but always cover credentials,
authorization headers, tokens, passwords, `.env` assignments, DSNs, database
connection fields, actor/contact/session/file identifiers, source and upload
paths, raw test fixture names that encode identifiers, and known synthetic
secret markers used by contract tests.

## 10. Contract Test Design

Each harness receives focused behavioral contract tests. Tests use temporary
repositories and command tripwires; they do not use real Docker, npm network,
MySQL, or production resources.

The common contract matrix proves:

1. List-only output contains only the sorted committed whitelist and required
   paths. Dirty, staged-only, untracked, generated, and forbidden paths do not
   affect it.
2. Export mode creates exactly four valid artifacts from immutable `HEAD`,
   rejects a relative, root, or pre-existing destination, and cannot reach
   Docker, npm, network, evidence, or tests.
3. A valid metadata-free package progresses to the first Docker or npm
   tripwire without `.git`.
4. A wrong authorization digest, changed package artifact, changed received
   file, missing or additional archive path, unsafe path, symlink, unsorted or
   duplicate list, incomplete required set, or mismatched per-file hash refuses
   before Docker/npm.
5. A pre-existing evidence directory refuses before Docker/npm. Docker
   harnesses also refuse each exact-project container, volume, or network
   collision before starting the project.
6. A controlled runtime failure retains only the permitted sanitized failure
   files, contains no injected secret marker or raw output, verifies under
   `sha256sum -c`, and removes temporary state.
7. The success evidence schema rejects malformed classifications and cannot be
   published before leak scan and hashing.
8. Existing behavioral command ordering and marker checks remain present.

F-05 additionally proves that `npm ci`, tests, and builds receive the temporary
verified `miniapp` directory as their working directory, that the received tree
does not gain `node_modules` or build output, and that all generated content is
removed after a controlled failure.

F-06 and F-14 tests retain their existing project-collision, source, migration,
and ordering regressions and extend them rather than replacing them. F-04/F-13
gains equivalent pre-Docker and evidence contracts.

Implementation follows RED/GREEN TDD for each harness: first add a focused test
that fails against the current script, then make the smallest script change
that satisfies it, then run the entire affected contract package. Shell syntax,
`gofmt`, `git diff --check`, full relevant Go/TypeScript suites, and an
independent code review are required before a harness is considered code-side
complete.

## 11. Test-Server Acceptance

Remote acceptance occurs only after all four implementations and documentation
are committed and a fresh package is exported from the final reviewed `HEAD`.
Earlier whitelists, packages, digests, transfers, and consumed one-run
authorizations cannot establish final-`HEAD` provenance.

One consolidated authorization request should enumerate, for every run:

- exact local commit and four-file package digest;
- exact fixed remote directory and source whitelist;
- whether an existing directory or exact Compose project must first be removed;
- allowed remote-only secret generation and toolchain/network use;
- the exact one-run Compose or Node-only command;
- allowed retention and read-only audit of newly generated sanitized evidence;
- the production observation boundary and all continuing prohibitions.

After transfer, local and remote hashes of all four package artifacts and the
out-of-band manifest digest must match before execution. Each Docker run uses
only its fixed project and isolated MySQL 8.4. F-05 uses only locked Node/npm
and the public configured npm registry. No run may be silently retried; a failed
one-run authorization is consumed and a corrective commit requires a fresh
package and authorization.

The retained evidence is reviewable only after its leak scan passes and every
entry verifies against `evidence-sha256.txt`. A tracked acceptance report then
records the commit, package digest, remote boundary, classifications, hashes,
production snapshot equality, and explicit non-actions without copying raw
logs, secrets, personal data, host addresses, or database contents.

## 12. Status And Traceability

Status documentation distinguishes three independent states for every finding:

1. **Code-side fixed:** implementation, focused/full local tests, and code review
   pass at a named commit.
2. **Test-server approved:** a package from that commit passes the authorized
   isolated run and its sanitized evidence audit.
3. **Production status:** migration/deployment/release state, which this design
   never changes.

An architecture or written-specification approval is not a fix. A local pass is
not test-server approval. An isolated test-server pass is not a production
deployment. Status documents are updated only when the corresponding evidence
exists and must link the design, implementation plan, review report, commit,
and evidence manifest.

F-15 remains pending its separately authorized corrective MySQL 8.4 run. This
design neither consumes nor expands that authorization. F-10 history cleanup
and F-12 identity-migration design remain separate blockers and are not changed
by acceptance-provenance hardening.

## 13. Acceptance Criteria

- Each of the four harnesses implements its own protocol-compatible source
  list, export, package import, temporary extraction, evidence, and cleanup
  flow with the exact documented prefix.
- The tested byte set is derived only from `git ls-tree` and `git archive` at a
  named committed `HEAD`.
- Each package contains exactly the four documented artifacts and binds to an
  out-of-band 64-character SHA-256 digest.
- Every package, path, file type, file list, and per-file hash is verified before
  Docker, npm, network, or test access.
- Every command builds or tests only the verified temporary extraction.
- Existing F-04/F-13, F-05, F-06, and F-14 behavior matrices and pass criteria
  remain intact.
- Pre-existing evidence and exact-project Docker resources fail closed.
- Raw logs and generated dependencies/builds remain temporary and are removed
  on every exit.
- Success and failure retain only schema-validated, leak-scanned, hashed
  evidence; sanitization uncertainty fails closed to hardcoded classifications.
- Docker harnesses inspect only the three named production containers and only
  name, ID, state, and restart count; validated before/after snapshots match.
- Cleanup stops only the exact isolated Compose project and performs no broad
  Docker deletion.
- Focused contract tests, affected full suites, shell syntax, formatting/diff
  checks, and independent review pass before remote execution is requested.
- Fresh final-`HEAD` packages pass their separately authorized isolated gates
  before any finding is marked test-server approved.
- No production SQL, log, environment, mount, configuration, service,
  migration, deployment, filesystem, upload, secret, or data is read or
  modified by this work.
