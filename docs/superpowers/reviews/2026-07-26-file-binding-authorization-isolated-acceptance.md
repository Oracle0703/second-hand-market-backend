# F-02 Isolated MySQL Acceptance

**Date:** 2026-07-26

**Branch:** `codex/reconcile-code-reviews`

**Environment:** Dedicated MySQL 8.4 acceptance project
`secondhand-file-binding-acceptance`

**Status:** F-02 is code-side closed on this branch and passed isolated
MySQL acceptance. The frontend/backend changes have not been deployed to
production, and production migration `0006` has not been executed.

## Scope and safety boundary

The run used only the isolated directory:

```text
/home/yu/services/secondhand-file-binding-acceptance-20260726
```

Before the run, the target whitelist and exclusions were verified, and no
container, volume, or network existed with Compose project label
`secondhand-file-binding-acceptance`. Fresh acceptance-only secrets were
generated in the isolated directory with mode `600`; no secret value was
displayed or included in evidence.

No `.git`, local database, upload, cache, `node_modules`, prior evidence, or
protected review document was transferred. No production container was
restarted or deployed, no production migration was run, and no production
data was read through SQL or written.

## Command and result

The complete matrix was run from a clean isolated Compose state:

```bash
FILE_BINDING_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_FILE_BINDING_DATA \
ACCEPTANCE_DB_ENGINE=mysql8.4 \
make acceptance-file-binding-smoke
```

Result: exit code 0 with `isolated file binding acceptance passed`.
MySQL `VERSION()` was `8.4.8`.

The matrix covered:

1. clean product-image and merchant-license references backfilled to the correct merchant;
2. orphan product image reference rejected before `0006` DDL with the schema unchanged;
3. wrong business type, non-`PASS` state, empty URL, cross-merchant reuse, and uploader-account mismatch each rejected independently before DDL;
4. unbound PUBLIC files remained ownerless while unbound MERCHANT files backfilled through `merchant_accounts`;
5. the full `0001..0006` chain with `AUTO_MIGRATE=false` completed anonymous upload, one-time registration claim, and owned product-image binding;
6. two synchronized claims of one capability produced exactly one successful merchant, with the losing merchant/account transaction rolled back and the capability cleared;
7. an `AUTO_MIGRATE=true` compatibility restart preserved one canonical table, one copy of each ownership/capability column, and one exact required index shape.

The migration-only API evidence includes:

```text
--- PASS: TestFileFlowWithMigrationOnlyMySQL (11.77s)
```

## Sanitized evidence

Evidence directory on the isolated host:

```text
/home/yu/services/secondhand-file-binding-acceptance-20260726/deploy/acceptance/evidence/file-binding-authorization/
```

The SHA-256 values below were independently recomputed after the successful
run:

```text
78fc39bd39b6e8bca5cb851482a31e2075659bec2a811eedd81265888cea8696  clean-backfill.txt
215311ebed91165dddbd78574e50cf2d46523acfc882cf2d70df6f03696d2862  cross-merchant.txt
e8df67a2049ca8c37ea82c8fccc9e5326bdcd7cf6912fd26ea57a345622f9265  empty-url.txt
2a571b6d528d9a1f2f27fb71ead9403773aedf24eaa8dfaef2ad1d9409afa4cd  file-flow.txt
78fc39bd39b6e8bca5cb851482a31e2075659bec2a811eedd81265888cea8696  full-chain.txt
e7a2e611502ea9bd55d8989d9fa4595943c7f6103252ff13d23841185e0aee2e  mysql-version.txt
20ac8b1f5d94d270ab13e567cbdf35994efcbd7fb8d986cb926788cb6c195938  non-pass.txt
c8f2c7ced31a2508e25a63e596c43c9783d66eb005c997af56e2fd4d41ee2e07  orphan.txt
84c708d395ddb9501489271ae1d968541e98e83ef7bfd7ac1bef9b4670a74f11  uploader-mismatch.txt
0d136f24d0dfc32f9b5ca32a5092dcc2bc5237161236ea666477473504d9e8ac  wrong-biz.txt
```

The clean/full-chain files contain the required ownership preflight and
postflight success markers. Every dirty fixture contains MySQL
`ERROR 1644 (45000)` and its expected reason-specific diagnostic. The evidence
scan found no MySQL/JWT secret assignment or private key.

## Production read-only comparison

Before and after the isolated run, the same production containers remained
running with restart count 0:

| Container | Before | After |
| --- | --- | --- |
| `secondhand-market-api` | running / 0 | running / 0 |
| `secondhand-market-web` | running / 0 | running / 0 |
| `secondhand-market-mysql` | running / 0 | running / 0 |

Production was not deployed, restarted, migrated, or used for acceptance
writes.

## Disposition

- F-02: **code-side closed and passed isolated MySQL 8.4 acceptance**.
- Production: **unchanged**. Frontend/backend deployment and migration `0006`
  still require separate production authorization.
- F-04, F-06, and F-13 remain separate open scope.

The isolated MySQL container, volume, network, and sanitized evidence remain
retained for review under Compose project
`secondhand-file-binding-acceptance`.
