# F-04/F-13 Capability Publication Handoff

**Recorded:** 2026-07-29

**Status:** **Paused by user; unresolved; handoff to Claude.** This record is
about the isolated acceptance harness provenance hardening. The previously
implemented F-04/F-13 application behavior is not reclassified here. The
isolated MySQL 8.4 test-server gate remains **not approved** and production
remains unchanged.

## Exact Repository State

- Worktree: `.worktrees/f15-idempotency-atomicity`
- Branch: `codex/f15-idempotency-atomicity`
- Partial implementation diff base (HEAD before this handoff record):
  `a2ff25d735a37e7514b927c9b46785ad70ed118e`
- Latest implementation commit: `53fe674e7566e4315901eedac98aee6cf98d8ed8`
- F-04/F-13 partial work is uncommitted in:
  - `backend/tests/license_file_privacy_acceptance_contract_test.go`
  - `deploy/acceptance/license-file-privacy-smoke.sh`
- Six other harness/test files also contain uncommitted work. They must not be
  discarded or treated as F-04-only changes.

## Verified At Handoff

The latest focused publication matrix passed against the real script:

```text
$ cd backend
$ go test ./tests -run '^TestLicenseFilePrivacyAcceptance(RejectsFinalEvidenceTamper|RejectsPublicationCollision|EvidenceParentReplacementCannotRedirectPublication|PreservesLockCommitPublicationAmbiguity|PreservesPublicationLockReleaseFailure|PreservesFinalReplacementAtLockCommit|PublicationChildReplacementFailsClosed)$' -count=1
ok  second-hand-market-backend/backend/tests  19.133s
```

Additional local checks at the same working-tree state:

- `bash -n deploy/acceptance/license-file-privacy-smoke.sh`: passed.
- `git diff --check`: passed.
- `gofmt -d backend/tests/license_file_privacy_acceptance_contract_test.go`:
  not clean; one indentation-only diff remains at the `tar` fixture near line
  820.

The focused result is not a complete F-04/F-13 acceptance result. The complete
contract suite, integrated repository gates, independent fixed-range review,
package export, and isolated MySQL 8.4 server run were not performed after the
latest partial edit.

## Open Finding

Task 3 of the capability addendum is not implemented for F-04/F-13. At this
handoff, `deploy/acceptance/license-file-privacy-smoke.sh` still initializes
Compose with received mutable paths:

```bash
compose=(docker compose --project-name "$project_name" --env-file "$base_dir/.env" --file "$base_dir/docker-compose.yml")
```

The required design instead consumes the verified private build-context
`deploy/acceptance/docker-compose.yml` and a private mode-`0600` `.env`
snapshot produced and compared through two independently opened read-only file
descriptors. Required Compose/`.env` replacement and same-inode mutation RED
tests have not been added for this F-04/F-13 harness.

## Continuation Commands

Start by preserving the current dirty state and reviewing only the two F-04
files listed above. After implementing Task 3, run:

```bash
cd backend
go test ./tests -run 'TestLicenseFilePrivacyAcceptance.*(Compose|Env).*(Snapshot|Replacement|ABA)' -count=1
go test ./tests -run '^TestLicenseFilePrivacyAcceptance' -count=1
cd ..
bash -n deploy/acceptance/license-file-privacy-smoke.sh
gofmt -d backend/tests/license_file_privacy_acceptance_contract_test.go
git diff --check
```

Do not claim completion until the complete harness suite, integrated Go/race/
vet gates, fixed-range review, final committed-HEAD package verification, and
separately authorized isolated server run all pass.

## Safety Record

No SSH connection, source transfer, remote Docker action, server database
operation, production SQL, production log/config inspection, deployment,
migration, or production-data/file modification was performed while producing
this handoff.
