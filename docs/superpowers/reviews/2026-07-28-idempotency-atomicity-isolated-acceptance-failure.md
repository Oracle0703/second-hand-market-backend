# F-15 Isolated MySQL 8.4 Acceptance - First Run Failure

**Date:** 2026-07-28

**Source commit:** `ce8787abab9df7befb51948e387e0fdd0006135d`

**Remote directory:**
`/home/yu/services/secondhand-idempotency-acceptance-20260728`

**Compose project:** `secondhand-idempotency-acceptance`

**Result:** Failed at the acceptance harness `test_metadata` stage. This run
does not approve F-15 on the test server. Production was not modified.

## Authorized Source Boundary

The run used only the four-file immutable source package generated from the
committed whitelist at `ce8787a`:

- `source-files.z`
- `source-sha256.txt`
- `source.tar`
- `package-sha256.txt`

The out-of-band SHA-256 of `package-sha256.txt` was
`c42edc4d72210d5551d7261c93da9b90e6bf28509e7add0b88b17bd1fdfdcbe3`.
All three package checksum entries and all 127 committed source paths verified;
the forbidden-path count was zero. No `.git` directory was transferred.

## Sanitized Evidence

Only the newly generated classified evidence was read after its leak scan
passed. The retained classifications were:

```text
classification=source_manifest|result=PASS|count=127|sha256=edf6aee6915b7d7ccc0dceddeade301450c8bf26f810a79c783b78c9f1492855
classification=mysql_8_4|result=PASS|count=1
classification=acceptance_failure|result=FAIL|stage=test_metadata|count=1
classification=evidence_scan|result=PASS|count=0
```

The evidence checksum manifest verified every retained evidence file. The
authorized fixed-field production snapshot had SHA-256
`ee185083b43c7f2eef2dd462388010b966485ef355ccc6600b17bff309c33235`
both before and after the failed run. The snapshot contained only each of the
three fixed production containers' name, ID, state, and restart count.

No production SQL, logs, environment variables, mounts, configuration,
services, migrations, deployment, or data were read or modified.

## Failure Boundary And Root Cause

The MySQL 8.4 gate passed. The failure occurred before the application test
matrix when the metadata-only tools container initialized temporary Git
metadata in the host-owned mode-0700 build context. Compose ran that container
as root, so Git rejected the different-owner workspace and root-owned `.git`
content prevented host-side cleanup.

This is an acceptance-harness ownership defect, not evidence that the F-15
application assertions failed. It nevertheless invalidates the server gate:
the MySQL concurrency, AutoMigrate, full, race, and vet stages did not complete.

The stopped isolated MySQL container, project volume, and project network were
retained. Cleanup also left the root-owned remote runtime
`/tmp/tmp.AJaD782XNx`. It was not read, modified, or deleted because the first
authorization did not permit that deletion.

## Corrective Action

Commit `f46bb3c29a20209f1ae43df11df70a868db9900e` adds a behavioral RED/GREEN
contract and runs only the metadata-init tools container with the invoking host
UID:GID and `HOME=/tmp`. Independent scoped review found no Critical,
Important, or Minor issue. Fresh focused acceptance contracts, full backend
tests, Bash syntax, gofmt, and diff checks passed locally.

A second server run requires a fresh immutable package and separate exact
authorization to remove and recreate only this remote directory and Compose
project resources. F-15 remains `test-server review pending` until that run
completes and its sanitized evidence passes review.
