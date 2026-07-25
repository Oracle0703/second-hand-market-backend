# F-09 / F-16 Isolated MySQL Acceptance

**Date:** 2026-07-26

**Branch:** `codex/reconcile-code-reviews`

**Environment:** Dedicated MySQL 8.4 acceptance project
`secondhand-file-schema-acceptance`

**Status:** F-09 and F-16 are locally fixed and passed isolated test-server
review. Neither change has been deployed to production, and production
migration `0005` has not been executed.

## Scope and safety boundary

The run used only the retained isolated directory:

```text
/home/yu/services/secondhand-file-schema-acceptance-20260726
```

Before the run, all existing resources with Compose project label
`secondhand-file-schema-acceptance` were removed with the exact project,
environment-file, and compose-file selectors. Only the disposable isolated
MySQL volume was deleted. Prior evidence was archived as
`file-record-schema-before-f16-20260725T185524Z`.

Only these reviewed F-16 files were synchronized, and local/remote SHA-256
values matched for each file:

- `backend/internal/model/models.go`
- `backend/internal/model/models_test.go`
- `backend/internal/app/server.go`
- `backend/internal/app/server_categories_test.go`
- `backend/scripts/seed_categories/main.go`
- `backend/scripts/seed_categories/main_test.go`

No `.env`, credentials, production configuration, database, upload, or
protected review document was synchronized. No production container was
restarted, deployed, or migrated, and no production data was written.

## Command and result

The complete matrix was run from a clean isolated Compose state:

```bash
FILE_SCHEMA_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_FILE_SCHEMA_DATA \
ACCEPTANCE_DB_ENGINE=mysql8.4 \
make acceptance-file-schema-smoke
```

Result: exit code 0 with `isolated file schema acceptance passed`.
MySQL `VERSION()` was `8.4.8`.

The matrix covered:

1. legacy `files`-only rename with row preservation;
2. canonical `file_records`-only verified no-op with stable row count;
3. both-table fail-closed behavior with both rows preserved;
4. neither-table fail-closed behavior without table creation;
5. legacy required-column drift rejected before rename;
6. canonical required-index drift rejected before no-op success;
7. the complete SQL chain and `AUTO_MIGRATE=false` file flow;
8. `AUTO_MIGRATE=true` `app.NewServer` compatibility after the SQL chain.

## F-16 MySQL RED to GREEN

The archived pre-fix run failed at the final AutoMigrate compatibility start:

```text
AutoMigrate compatibility start failed: Error 1061 (42000):
Duplicate key name 'uk_parent_name'
```

Archived RED evidence SHA-256:

```text
ee21e980eca7d68020700d675af18e63e13180f56373ddd1460a8e4c7b7da291  file-flow.txt
3e409a92a4dd5512dce675cf360dd688a60b6f06b02e91d573f73a5f259a67bd  matrix-run.txt
2e9779bc949d8f12fbd1babc45cbf97f68cac022dbfe482af9ba87d8574e03d7  result.txt
```

After the composite model index and parent-aware seed fixes, the same opt-in
test ran rather than skipped and passed:

```text
--- PASS: TestFileFlowWithMigrationOnlyMySQL (10.83s)
```

The test first starts the migration-only application with AutoMigrate disabled,
executes the file flow, then starts `app.NewServer` with AutoMigrate enabled and
asserts that `files=0` and `file_records=1`. The fresh evidence contains no
`Duplicate key name 'uk_parent_name'` text.

## Fresh evidence

Evidence directory:

```text
deploy/acceptance/evidence/file-record-schema/
```

SHA-256:

```text
89d9b3d03137ceee43eeca363981de05ce13b8da640a48ddacfdadd854e5f5b0  canonical.txt
4a5a12a6a602f50491c5074ffc9b2053e9ea5fceac8105ddc9184617924eb1c0  drift-canonical-index.txt
aafec6cd010656aaf512c9a0a156ffe38f23698721377438ec9c6e4bd0adaae3  drift-legacy-column.txt
4aa0631d2a3331b98b2285cf94cd173693822b86c6bcf5774d562e2a6e0f6a43  file-flow.txt
63d559aa4f82c739320bf34ceced3dfc9584f17f84ff83c7fadd4424c1d8a1c4  full-chain.txt
9185c0652bb3744ff8c18960f407bf372acdf48647cf38f01e8d06eb598524fc  legacy.txt
e7a2e611502ea9bd55d8989d9fa4595943c7f6103252ff13d23841185e0aee2e  mysql-version.txt
```

The legacy, canonical, and full-chain files contain their required preflight,
rename/no-op, and postflight markers. Both drift files contain the intended SQL
1644 errors and no migration success marker.

## Production read-only comparison

Before and after the isolated run, the same production containers remained
running with restart count 0:

| Container | ID prefix | Before | After |
| --- | --- | --- | --- |
| `secondhand-market-api` | `5cae6e645c73` | running / 0 | running / 0 |
| `secondhand-market-web` | `ce67597f4bdb` | running / 0 | running / 0 |
| `secondhand-market-mysql` | `cb7b25fa0738` | running / 0 | running / 0 |

`https://market.meaningful.ink/healthz` returned HTTP success with application
`code=0` before and after the run.

## Disposition

- F-09: **fixed locally and passed isolated MySQL 8.4 test-server review**.
- F-16: **fixed locally and passed isolated MySQL 8.4 test-server review**.
- Production: **unchanged**. Migration `0005`, application deployment,
  administrator rotation, production write checks, and the release observation
  window still require separate production authorization.

The isolated MySQL container, network, volume, and fresh evidence remain
retained for review under Compose project
`secondhand-file-schema-acceptance`.
