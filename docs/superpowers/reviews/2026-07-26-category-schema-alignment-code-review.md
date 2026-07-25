# F-16 Category Schema Alignment Code Review

**Date:** 2026-07-26

**Branch:** `codex/reconcile-code-reviews`

**Review basis:** `252d494` plus the current uncommitted F-09 and F-16 working tree

**Plan:** `docs/superpowers/plans/2026-07-26-category-schema-alignment.md`

**Verdict:** Ready for the final review commit; local review and isolated MySQL 8.4 acceptance passed.

## Scope and safety

This review covers only the F-16 model metadata, API startup seed path,
standalone seed command, and their focused tests. The existing F-09 file-table
work was preserved. No inventory, product, reservation, order, production
configuration, or historical migration file was changed for F-16.

Because the current execution policy did not permit dispatching a review
subagent, the required two-stage review was performed locally against the
approved design and plan: first for specification compliance, then for
implementation and test quality.

## Stage 1: Design and plan compliance

| Requirement | Result | Evidence |
| --- | --- | --- |
| GORM index is unique `(parent_id, name)` | Pass | `TestCategoryParentNameUniqueIndexMatchesMigration` |
| Historical `0001` remains unchanged | Pass | scope audit |
| Root lookup uses `parent_id IS NULL` | Pass | both `findOrCreateCategory` implementations |
| Child lookup uses `parent_id = ?` | Pass | both implementations |
| Seed never updates identity field `parent_id` | Pass | obsolete parent mutation removed |
| Mutable fields reconcile without changing row identity | Pass | both database-backed tests |
| API and standalone seed behavior stay aligned | Pass | side-by-side function comparison and package tests |
| F-09 `FileRecord.TableName()` test remains present | Pass | focused model test |

No specification deviations were found. The only addition beyond the written
test body is an assertion that the returned `Category` reflects a reconciled
`Level`; it exposed and closed a stale-return-value bug already anticipated by
the plan's reference implementation.

## Stage 2: Implementation quality

### Strengths

- The change corrects metadata at the model source rather than rewriting the
  historical migration or adding a redundant migration.
- Parent identity is expressed in the lookup predicate and is no longer
  treated as mutable seed metadata.
- Both shipped seed implementations have equivalent lookup, create, update,
  and error paths.
- Tests exercise real GORM schema parsing and database behavior without mocks.
- Test expectations use literal row counts, parent identities, index columns,
  and mutable-field values.

### Findings

- Critical: none.
- Important: none.
- Minor: none.

The deliberate duplication between API and standalone seed code remains. A
shared package could reduce duplication, but the approved repair explicitly
excludes that refactor and separate tests prevent silent divergence.

## TDD evidence

| Behavior | RED evidence | GREEN evidence |
| --- | --- | --- |
| Composite category index metadata | Parsed columns were `[name]`, expected `[parent_id name]` | Focused model tests pass |
| API seed scopes same name to parent | First shared-child count was 1, expected 2 | API focused and package tests pass |
| Standalone seed scopes same name to parent | First shared-child count was 1, expected 2 | Standalone focused and package tests pass |
| Reconciled return value is current | Both implementations returned `Level:9`, expected 2 | Both package tests pass |

## Verification observed

- Focused model/API/standalone tests: pass, uncached.
- Complete backend suite via `make backend-test`: pass.
- F-16 `git diff --check`: pass.
- Inventory/order business-path scope audit: no F-16 changes.

The complete isolated MySQL 8.4 F-09 matrix, including the final
`AUTO_MIGRATE=true` `app.NewServer` start, passed on the dedicated test server.
The archived pre-fix run failed with MySQL error 1061 for `uk_parent_name`; the
fresh run passed the same opt-in test with no duplicate-index error. Evidence
and hashes are recorded in
`docs/superpowers/reviews/2026-07-26-file-category-schema-isolated-acceptance.md`.

## Assessment

**Ready for isolated acceptance:** Completed.

**Ready for final review commit:** Yes, after the final repository-wide
verification commands are rerun.
