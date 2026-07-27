# 0008 MySQL 8.4 HAVING Compatibility Local Verification

**Date:** 2026-07-28

**Branch:** `codex/f11-buyer-intent-open-uniqueness`

**Status boundary:** local code-side verification and independent review are complete. A new isolated MySQL 8.4 rerun requires authorization. Test-server acceptance is not approved, and production `0008`/`0009` has not run.

## Reviewed Commit Boundary

- Approved design/spec commit: `50a991d321788ffe77eab9646f4888929b8f5e82`
- Approved plan commit: `e1ef92a`
- Implementation and reviewed correction commits:
  - `ee1a964` `fix(migrations): make 0008 index checks MySQL compatible`
  - `987c4f4` `test(migrations): bind 0008 alias projections`
  - `33121f1` `test(migrations): scope 0008 HAVING predicates`
- Whole-range review scope: `50a991d321788ffe77eab9646f4888929b8f5e82..33121f157dc671ad4d453d4052fc9a653f1eb804`

## Prior Authorized Isolated Attempt

- Attempt basis: `e55be6a`; 120 committed source files; manifest SHA-256 `b9b230c6706bfb399ad2679b92c4ca3a58d6f176ca18176dcd641c3a1cccc226`.
- Sanitized result: isolated MySQL `8.4.8` exited `2` in the `0008` preflight with `ERROR 1054 (42S22): Unknown column 'non_unique' in 'having clause'`; `0009` was not entered.
- Local isolated parser proof: the unprojected grouped query failed; the approved `non_unique AS is_non_unique` projection with the HAVING alias exited `0` and reported count `2`.
- Remote status: the retained failure was not read, removed, restarted, or rerun.

## RED Evidence

From `backend/`, before the SQL fix:

```bash
gofmt -w migrations/anonymous_upload_governance_migration_test.go
go test ./migrations -run '^TestAnonymousUploadGovernanceGroupedIndexHavingProjectsNonUnique$' -count=1
```

Exit code: `1`.

```text
anonymous_upload_governance_migration_test.go:121: projected grouped queries = 0, want 2
anonymous_upload_governance_migration_test.go:121: projected grouped queries = 0, want 1
anonymous_upload_governance_migration_test.go:121: projected grouped queries = 0, want 1
```

The first failure was the expected projected-query contract failure, not a compile or file-read failure. The two reviewed correction rounds also recorded expected mutation RED failures: `bound grouped projections = 0, want 2` and `HAVING alias predicates = 3, want 4`.

## GREEN Local Gates

Fresh gates ran on `33121f157dc671ad4d453d4052fc9a653f1eb804`; every command below exited `0`.

```bash
cd backend
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

Results:

```text
focused grouped-HAVING migration test: ok, 0.732s
related 0008/F-11 migration suite: ok, 5.882s
full backend suite: PASS; migrations 12.450s, tests 15.019s
race backend suite: PASS; migrations 15.508s, tests 210.615s
go vet ./...: exit 0, no output
both bash -n checks and git diff --check: exit 0, no output
```

The statement-scoped regression contract proved exactly four target queries: preflight `2`, up `1`, postflight `1`; projections/groupings/predicates were respectively `2/2/4`, `1/1/2`, and `1/1/2`.

## Independent Review

The read-only independent review of `50a991d..33121f1` returned spec compliance **PASS** with Critical `0`, Important `0`, and Minor `0`.

- It confirmed the exact `2/1/1` target-query distribution and the `4/2/2` statement-scoped HAVING alias predicates.
- It confirmed unchanged index names, column order, uniqueness counts, messages, DDL, DML, guard behavior, and all implementation paths outside the four-file boundary.
- It confirmed the regression contract fails when the approved alias correction is removed or displaced.
- It did not rerun local gates and did not access remote, production, credentials, uploads, or retained evidence.

## Status Boundaries

- **Local code-side:** reviewed. The SQL correction, RED/GREEN regression contract, fresh focused/full/race/vet/bash/diff gates, and independent review are complete.
- **Test-server acceptance:** pending authorization for a new isolated MySQL 8.4 rerun. The earlier authorized run stopped in `0008` before `0009`; no acceptance pass is claimed.
- **Production:** `0008` and `0009` were not executed. No deployment or production data, configuration, service, or upload change occurred.
- **F-12:** remains blocked pending the authorized isolated MySQL 8.4 acceptance record for the accepted F-11 range.
