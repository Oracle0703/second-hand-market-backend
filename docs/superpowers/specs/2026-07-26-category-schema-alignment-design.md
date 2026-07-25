# Category Schema Alignment Design

**Date:** 2026-07-26
**Finding:** F-16 - category uniqueness semantics differ between SQL, GORM, and seed queries
**Status:** 本地修复并通过隔离 MySQL 8.4 测试服务器审核；生产未部署
**Scope:** Category composite uniqueness, default-category seeding, and AutoMigrate compatibility

## Context

Before this change, the repository expressed three different uniqueness contracts for
categories:

- `backend/migrations/0001_init.up.sql` defines
  `uk_parent_name(parent_id, name)`.
- `backend/internal/model/models.go` assigned `uk_parent_name` only to `Name`,
  so GORM interprets it as a unique index on `name` alone.
- Both the API startup seed path and `backend/scripts/seed_categories` queried by
  global `name` and may move an existing same-named category to another
  parent.

On a schema created by the SQL migration, an `AUTO_MIGRATE=true` application
start therefore asks MySQL to create a conflicting index with the same name
and fails with error 1061, `Duplicate key name 'uk_parent_name'`. The same
drift also prevents different parents from safely containing children with
the same name.

## Goals

- Make `(parent_id, name)` the single category uniqueness contract used by
  SQL, GORM, the API seed path, and the standalone seed command.
- Prevent a seed lookup from finding or modifying a same-named category under
  a different parent.
- Keep repeated default-category seeding idempotent.
- Restore `AUTO_MIGRATE=true` compatibility after the full SQL migration
  chain so the F-09 isolated acceptance matrix can complete.
- Prove the alignment with focused unit and database-backed regression tests.

## Non-goals

- No production deployment or production migration execution.
- No rewrite of historical migration `0001` and no new category migration.
- No generated column or functional index to enforce uniqueness for root
  categories whose `parent_id` is `NULL`.
- No category administration feature or taxonomy redesign.
- No changes to inventory, product stock, reservation, or order business
  paths.
- No modification, staging, or commit of the three protected untracked review
  documents or the F-02 design and plan.

## Decision

The canonical uniqueness key is `(parent_id, name)`, matching the historical
SQL migration and the documented data model. `Category.ParentID` will join
GORM's named unique index at priority 1 while `Category.Name` remains priority
2. GORM will then recognize the existing MySQL index rather than attempting to
create a different single-column index with the same name.

Category lookup will use parent-aware predicates:

- Root category: `parent_id IS NULL AND name = ?`
- Child category: `parent_id = ? AND name = ?`

After a row is selected by that identity, seeding may still reconcile its
level, enabled status, and sort order. It will not update `parent_id`, because
the parent is part of the row's identity rather than mutable seed metadata.
The API startup path and standalone seed command will use the same predicate
and update rules.

MySQL permits multiple `NULL` values in a composite unique index, so the
database alone does not prevent duplicate same-named root categories. This
change deliberately keeps root-category idempotency at the seed lookup layer.
Adding a generated column solely to close that edge case would require a new
migration and broader production-schema planning, which is outside this fix.

## Components

### GORM model contract

Update `Category.ParentID` to declare both the existing sort index and the
first column of `uk_parent_name`. `Name` continues to declare the second
column. The index name, column order, and uniqueness must exactly match
`0001_init.up.sql`.

No SQL migration file changes are needed. Existing migration-built databases
already have the intended index, and newly AutoMigrated databases will create
the same shape after the model correction.

### Parent-aware seed lookup

Change `findOrCreateCategory` in both seed implementations to build its lookup
from `parentID`:

- nil parent IDs use `parent_id IS NULL`;
- non-nil parent IDs use `parent_id = ?` with the dereferenced ID;
- both branches also filter by `name`.

If no row matches that composite identity, create one with the supplied
parent, level, name, enabled status, and sort order. If a row matches, update
only level, status, and sort when they differ. Database errors other than
record-not-found are returned unchanged.

### Regression tests

Add focused tests that exercise real GORM behavior:

1. Parse the `Category` schema and assert `uk_parent_name` is unique and its
   ordered database columns are exactly `parent_id, name`. This fails against
   the current model because only `name` is present.
2. Seed two children with the same name under different parents and assert two
   distinct rows remain attached to their original parents. This fails
   against the current global-name lookup because the first row is moved.
3. Run default-category seeding twice and assert row identities and total
   count remain unchanged while mutable seed fields are reconciled.
4. Cover both the API startup seed implementation and the standalone seed
   command so their duplicated logic cannot silently diverge again.

SQLite is sufficient for the focused lookup and idempotency tests. The
existing isolated MySQL 8.4.8 acceptance environment remains authoritative
for SQL migration and AutoMigrate compatibility.

## Failure Handling

- Query failures other than `gorm.ErrRecordNotFound` stop seeding and return
  the original error.
- Create and update failures propagate to the caller; startup or the seed
  command must not report success after a partial operation.
- A MySQL AutoMigrate failure stops the isolated acceptance run and preserves
  its evidence for inspection.
- No failed isolated test authorizes production migration or deployment.

## Verification

Local verification runs without Docker:

- focused model schema test;
- focused API seed tests;
- focused standalone seed tests;
- complete backend Go test suite using repository-local caches.

After local tests and code review pass, synchronize only the approved working
tree changes to the retained server acceptance directory and rerun the entire
F-09 isolated matrix from its first state. The matrix must cover legacy rename,
canonical no-op, both-table rejection, neither-table rejection, both schema
drift cases, the clean full SQL chain with `AUTO_MIGRATE=false`, the file flow,
and the final `AUTO_MIGRATE=true` compatibility start.

Production API, web, and MySQL services must remain untouched. Their health
and restart counts may be checked read-only before and after acceptance.

## Acceptance Criteria

- GORM resolves `uk_parent_name` as unique `(parent_id, name)`.
- SQL migration and AutoMigrate no longer disagree about the category index.
- Same-named children under different parents coexist without reassignment.
- Repeated default-category seeding is idempotent.
- API startup and standalone seed behavior remain aligned.
- All local backend tests pass without requiring Docker.
- The full isolated MySQL 8.4.8 F-09 matrix passes from the beginning,
  including the final `AUTO_MIGRATE=true` start.
- No inventory or order business-path files are changed.
- No production deployment or migration is performed.
- Protected review documents and F-02 documents remain unmodified, unstaged,
  and uncommitted.
