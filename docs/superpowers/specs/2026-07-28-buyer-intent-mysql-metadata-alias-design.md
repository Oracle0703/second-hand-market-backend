# F-11 MySQL Metadata Alias Compatibility Design

**Date:** 2026-07-28

**Branch:** `codex/f11-buyer-intent-open-uniqueness`

**Status:** Root cause confirmed in the isolated MySQL 8.4 project; implementation pending

**Scope:** F-11 startup verification in `inspectMySQLBuyerIntentSchema`

## 1. Problem And Evidence

The current committed F-11 source reaches migration `0009` successfully on
MySQL 8.4.8, but the opt-in `TestBuyerIntentMySQLAcceptance` fails while
constructing `NewServer` with the stable error:

```text
buyer intent columns are missing or drifted
```

Read-only inspection of the isolated schema proves that the five relevant
columns and final unique index are correct. A controlled diagnostic using the
same GORM/MySQL connection then proves the mapping failure:

- the metadata query returns 20 rows;
- every directly selected `information_schema.columns` field scans as an empty
  Go string;
- the computed field explicitly aliased as `is_generated` scans correctly;
- `marker_present` and `marker_exact` therefore remain false.

MySQL 8.4 exposes direct information-schema result labels using the source
column casing. GORM raw scanning matches the labels against the lowercase
`gorm:"column:..."` names. The query relies on implicit labels for
`column_name`, `data_type`, `column_type`, `is_nullable`,
`generation_expression`, and `extra`, so those fields are not populated.
The statistics query has the same latent problem for `index_name`,
`non_unique`, `seq_in_index`, and `column_name`.

This is a runtime metadata-mapping defect, not migration drift. Production was
not queried or changed while diagnosing it.

## 2. Goals

1. Make MySQL metadata labels stable and independent of driver/source-column
   casing.
2. Preserve the existing strict column, generated-marker, index, and row-state
   validation.
3. Keep SQLite behavior and all migration SQL unchanged.
4. Prove the fix with the already failing real MySQL 8.4 acceptance test, then
   rerun local focused and full Go gates.
5. Preserve all existing isolated-project, source-whitelist, evidence, and
   production prohibitions.

## 3. Non-Goals

- Do not loosen accepted MySQL column or index layouts.
- Do not change `0008`, `0009`, GORM model tags, or business behavior.
- Do not add a migration, dependency, reflection helper, or driver-specific
  global naming rule.
- Do not read or operate production SQL, data, logs, configuration, services,
  or containers beyond the existing fixed metadata snapshots.

## 4. Approaches Considered

### A. Explicit Stable Aliases (Selected)

Alias every selected information-schema field to the exact lowercase name used
by the destination struct tag. Apply the same rule to the columns and
statistics queries.

This is the smallest fix at the component boundary where the mismatch occurs.
It keeps the validator and its accepted layouts unchanged.

### B. Uppercase Struct Tags

Change tags to the labels currently returned by MySQL 8.4. This couples the Go
types to one driver/server presentation and can regress when aliases or another
supported MySQL version use lowercase names.

### C. Positional `Rows.Scan`

Bypass GORM mapping and scan each result position manually. This is reliable but
adds lower-level resource management and duplicates mapping mechanics for two
queries without a broader need.

Approach A is selected because it fixes the boundary contract explicitly with
the least code and no semantic expansion.

## 5. Detailed Design

In `inspectMySQLBuyerIntentSchema`, the column query will select:

```sql
column_name AS column_name,
data_type AS data_type,
column_type AS column_type,
is_nullable AS is_nullable,
generation_expression AS generation_expression,
extra AS extra,
... AS is_generated
```

The statistics query will select:

```sql
index_name AS index_name,
non_unique AS non_unique,
seq_in_index AS seq_in_index,
column_name AS column_name
```

The aliases are part of the query-to-struct interface. No validator condition,
accepted layout, error message, or migration operation changes.

## 6. Error Semantics

Query errors retain their existing wrapped diagnostic categories. Correct
schemas pass. Missing, duplicate, lookalike, malformed, or non-final schemas
continue to fail closed with the existing stable drift errors. The fix must not
turn an empty scan into an accepted schema.

## 7. Verification

The existing isolated MySQL test is the behavioral regression test. It already
produced RED against the committed source before implementation. GREEN requires:

1. `TestBuyerIntentMySQLAcceptance` passes with `AUTO_MIGRATE=false` and true in
   the fixed Compose project;
2. the complete F-11 acceptance harness exits zero;
3. local buyer-intent schema, app, migration, and full Go tests pass;
4. `go test -race ./...`, `go vet ./...`, and `git diff --check` pass;
5. source manifests match before the accepted remote run;
6. evidence leak/hash gates and production snapshot equality pass.

The regression would fail again if any required alias were removed because the
real MySQL/GORM boundary would once more produce empty metadata fields and stop
`NewServer`.

## 8. Rollback

Revert the focused alias commit. No database or data rollback is required
because this change performs read-only metadata inspection and adds no schema
mutation. Reverting reopens the MySQL 8.4 startup failure and must not be used in
an F-11 release candidate.

## 9. Traceability

- RED source: `TestBuyerIntentMySQLAcceptance` on the isolated MySQL 8.4.8
  project.
- Root-cause probe: correct CLI metadata plus empty GORM direct-field scans and
  a populated explicit `is_generated` alias.
- Implementation owner: the companion plan
  `docs/superpowers/plans/2026-07-28-buyer-intent-mysql-metadata-alias.md`.
- Governing authorization: the approved F-11 direct iterative debugging plan,
  which requires RED -> GREEN and a focused tracked-source fix when a
  reproducible source defect is found.
