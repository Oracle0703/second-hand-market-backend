# Category Schema Alignment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Align GORM and both category seed paths with the existing `(parent_id, name)` SQL contract so MySQL AutoMigrate succeeds and same-named children remain scoped to their parents.

**Architecture:** Keep historical migration `0001` unchanged and correct the GORM model metadata to describe its existing composite unique index. Make both duplicated seed implementations select categories by the same composite identity, then prove the behavior locally before rerunning the retained F-09 MySQL 8.4.8 acceptance matrix from a clean isolated Compose state.

**Tech Stack:** Go 1.22, GORM 1.30, glebarez SQLite, MySQL 8.4.8, Bash, Docker Compose on the dedicated acceptance server

## Global Constraints

- Do not modify inventory, product stock, reservation, or order business paths.
- Do not deploy to production and do not run a production migration.
- Do not rewrite `backend/migrations/0001_init.up.sql` and do not add a category migration.
- Do not modify, stage, or commit `docs/architecture-evolution-plan-2026-07-24.md`, `docs/first-round-fix-review-2026-07-24.md`, or `docs/second-round-fix-review-2026-07-24.md`.
- Do not modify, stage, or commit the F-02 files `docs/superpowers/specs/2026-07-26-file-binding-authorization-design.md` and `docs/superpowers/plans/2026-07-26-file-binding-authorization.md`.
- Preserve all existing uncommitted F-09 work and the existing `TestFileRecordTableName` test.
- Do not create a commit during this plan. Stop after implementation, review, local verification, and isolated server acceptance so the user can authorize the final commit separately.
- Follow strict TDD: every production edit must be preceded by a focused test that is observed failing for the expected reason.
- The MySQL composite unique index intentionally retains standard `NULL` semantics; root-category idempotency remains enforced by the seed lookup rather than a generated-column migration.

## File Map

- Modify `backend/internal/model/models.go`: describe `uk_parent_name` as `(parent_id, name)` to GORM.
- Modify `backend/internal/model/models_test.go`: preserve the F-09 table-name test and add the category index contract test.
- Create `backend/internal/app/server_categories_test.go`: exercise API startup seed identity and idempotency.
- Modify `backend/internal/app/server.go`: use parent-aware category lookup and stop moving rows between parents.
- Create `backend/scripts/seed_categories/main_test.go`: exercise standalone seed identity and idempotency.
- Modify `backend/scripts/seed_categories/main.go`: mirror the API seed lookup rules.
- Use, but do not edit for F-16, `deploy/acceptance/file-record-schema-smoke.sh`: rerun the full F-09 MySQL matrix.

---

### Task 1: Align the GORM category index contract

**Files:**
- Modify: `backend/internal/model/models_test.go`
- Modify: `backend/internal/model/models.go:119`

**Interfaces:**
- Consumes: GORM `schema.Parse` and `(*Schema).ParseIndexes()`.
- Produces: `Category.ParentID` and `Category.Name` jointly describe the unique index `uk_parent_name` in database-column order `parent_id, name`.

- [x] **Step 1: Add the failing model metadata test**

Preserve `TestFileRecordTableName` and expand `backend/internal/model/models_test.go` to include these imports and test:

```go
package model

import (
	"reflect"
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

func TestFileRecordTableName(t *testing.T) {
	if got := (FileRecord{}).TableName(); got != "file_records" {
		t.Fatalf("FileRecord table name = %q, want %q", got, "file_records")
	}
}

func TestCategoryParentNameUniqueIndexMatchesMigration(t *testing.T) {
	parsed, err := schema.Parse(&Category{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse Category schema: %v", err)
	}

	var parentNameIndex *schema.Index
	for _, index := range parsed.ParseIndexes() {
		if index.Name == "uk_parent_name" {
			parentNameIndex = index
			break
		}
	}
	if parentNameIndex == nil {
		t.Fatal("uk_parent_name index is missing")
	}
	if parentNameIndex.Class != "UNIQUE" {
		t.Fatalf("uk_parent_name class = %q, want UNIQUE", parentNameIndex.Class)
	}

	columns := make([]string, 0, len(parentNameIndex.Fields))
	for _, field := range parentNameIndex.Fields {
		columns = append(columns, field.Field.DBName)
	}
	if want := []string{"parent_id", "name"}; !reflect.DeepEqual(columns, want) {
		t.Fatalf("uk_parent_name columns = %v, want %v", columns, want)
	}
}
```

- [x] **Step 2: Run the focused test and verify RED**

Run:

```bash
cd backend
mkdir -p .cache/go/mod .cache/go/build
GOMODCACHE=$(pwd)/.cache/go/mod GOCACHE=$(pwd)/.cache/go/build go test ./internal/model -run '^TestCategoryParentNameUniqueIndexMatchesMigration$' -count=1 -v
```

Expected: FAIL because the parsed columns are `[name]`, proving the test catches the current model drift.

- [x] **Step 3: Add the missing composite-index tag**

Change only `Category.ParentID` in `backend/internal/model/models.go`:

```go
ParentID *uint64 `gorm:"uniqueIndex:uk_parent_name,priority:1;index:idx_parent_sort,priority:1" json:"parent_id"`
```

Keep the existing `Name` tag as priority 2:

```go
Name string `gorm:"size:64;uniqueIndex:uk_parent_name,priority:2" json:"name"`
```

- [x] **Step 4: Verify GREEN and preserve the F-09 model test**

Run:

```bash
cd backend
GOMODCACHE=$(pwd)/.cache/go/mod GOCACHE=$(pwd)/.cache/go/build go test ./internal/model -run '^(TestCategoryParentNameUniqueIndexMatchesMigration|TestFileRecordTableName)$' -count=1 -v
```

Expected: both tests PASS.

- [x] **Step 5: Review checkpoint without committing**

Run `git diff --check -- backend/internal/model/models.go backend/internal/model/models_test.go` and confirm no other model field changed. Do not stage or commit.

---

### Task 2: Make API startup category seeding parent-aware

**Files:**
- Create: `backend/internal/app/server_categories_test.go`
- Modify: `backend/internal/app/server.go:205`

**Interfaces:**
- Consumes: `ensureDefaultCategories(*gorm.DB) error`, `defaultCategorySeeds`, and `model.Category`.
- Produces: `findOrCreateCategory` treats `(parentID, name)` as identity and updates only `level`, `status`, and `sort` for an existing row.

- [x] **Step 1: Add a failing database-backed seed test**

Create `backend/internal/app/server_categories_test.go`:

```go
package app

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/model"
)

func openCategorySeedTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open category test database: %v", err)
	}
	if err := db.AutoMigrate(&model.Category{}); err != nil {
		t.Fatalf("migrate category test database: %v", err)
	}
	return db
}

func TestEnsureDefaultCategoriesScopesSameNameToParentAndIsIdempotent(t *testing.T) {
	originalSeeds := defaultCategorySeeds
	defaultCategorySeeds = []categorySeed{
		{Name: "Root A", Children: []string{"Shared"}},
		{Name: "Root B", Children: []string{"Shared"}},
	}
	t.Cleanup(func() { defaultCategorySeeds = originalSeeds })

	db := openCategorySeedTestDB(t)
	if err := ensureDefaultCategories(db); err != nil {
		t.Fatalf("first seed: %v", err)
	}

	var firstChildren []model.Category
	if err := db.Where("level = ? AND name = ?", 2, "Shared").Order("parent_id").Find(&firstChildren).Error; err != nil {
		t.Fatalf("load first children: %v", err)
	}
	if len(firstChildren) != 2 {
		t.Fatalf("first shared child count = %d, want 2", len(firstChildren))
	}
	if firstChildren[0].ParentID == nil || firstChildren[1].ParentID == nil || *firstChildren[0].ParentID == *firstChildren[1].ParentID {
		t.Fatalf("shared children have invalid parents: %+v", firstChildren)
	}
	firstIDs := map[uint64]uint64{
		*firstChildren[0].ParentID: firstChildren[0].ID,
		*firstChildren[1].ParentID: firstChildren[1].ID,
	}
	if err := db.Model(&model.Category{}).Where("id = ?", firstChildren[0].ID).Updates(map[string]interface{}{
		"level":  9,
		"status": model.CategoryDisabled,
		"sort":   99,
	}).Error; err != nil {
		t.Fatalf("drift first shared child: %v", err)
	}

	if err := ensureDefaultCategories(db); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	var secondChildren []model.Category
	if err := db.Where("level = ? AND name = ?", 2, "Shared").Order("parent_id").Find(&secondChildren).Error; err != nil {
		t.Fatalf("load second children: %v", err)
	}
	if len(secondChildren) != 2 {
		t.Fatalf("second shared child count = %d, want 2", len(secondChildren))
	}
	for _, child := range secondChildren {
		if child.ParentID == nil || firstIDs[*child.ParentID] != child.ID {
			t.Fatalf("seed changed shared child identity: %+v", child)
		}
		if child.Level != 2 || child.Status != model.CategoryEnabled || child.Sort != 1 {
			t.Fatalf("seed did not reconcile shared child: %+v", child)
		}
	}

	var total int64
	if err := db.Model(&model.Category{}).Count(&total).Error; err != nil {
		t.Fatalf("count categories: %v", err)
	}
	if total != 4 {
		t.Fatalf("category count after repeated seed = %d, want 4", total)
	}
}
```

The production change that this test catches is a lookup that omits the parent predicate and moves or reuses the first `Shared` row.

- [x] **Step 2: Run the focused test and verify RED**

Run:

```bash
cd backend
GOMODCACHE=$(pwd)/.cache/go/mod GOCACHE=$(pwd)/.cache/go/build go test ./internal/app -run '^TestEnsureDefaultCategoriesScopesSameNameToParentAndIsIdempotent$' -count=1 -v
```

Expected: FAIL with `first shared child count = 1, want 2` against the global-name lookup.

- [x] **Step 3: Implement the minimal parent-aware query**

In `backend/internal/app/server.go`, replace `findOrCreateCategory` and remove
the now-unused `sameParentID` helper:

```go
func findOrCreateCategory(db *gorm.DB, parentID *uint64, level int8, name string, sort int) (model.Category, error) {
	var cat model.Category
	query := db.Model(&model.Category{}).Where("name = ?", name)
	if parentID == nil {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *parentID)
	}
	if err := query.First(&cat).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			cat = model.Category{
				ParentID: parentID,
				Level:    level,
				Name:     name,
				Status:   model.CategoryEnabled,
				Sort:     sort,
			}
			return cat, db.Create(&cat).Error
		}
		return model.Category{}, err
	}
	updates := map[string]interface{}{}
	if cat.Level != level {
		updates["level"] = level
	}
	if cat.Status != model.CategoryEnabled {
		updates["status"] = model.CategoryEnabled
	}
	if cat.Sort != sort {
		updates["sort"] = sort
	}
	if len(updates) > 0 {
		if err := db.Model(&model.Category{}).Where("id = ?", cat.ID).Updates(updates).Error; err != nil {
			return model.Category{}, err
		}
		cat.Level = level
		cat.Status = model.CategoryEnabled
		cat.Sort = sort
	}
	return cat, nil
}
```

- [x] **Step 4: Verify GREEN**

Run the focused test from Step 2 again.

Expected: PASS with two stable `Shared` child rows and four total category rows after the second seed.

- [x] **Step 5: Run the API package regression tests**

Run:

```bash
cd backend
GOMODCACHE=$(pwd)/.cache/go/mod GOCACHE=$(pwd)/.cache/go/build go test ./internal/app -count=1
```

Expected: PASS.

- [x] **Step 6: Review checkpoint without committing**

Confirm `git diff -- backend/internal/app/server.go` contains only the parent-aware query and removal of the obsolete parent-mutation helper. Do not stage or commit.

---

### Task 3: Keep the standalone seed command aligned

**Files:**
- Create: `backend/scripts/seed_categories/main_test.go`
- Modify: `backend/scripts/seed_categories/main.go:80`

**Interfaces:**
- Consumes: the standalone command's `ensureDefaultCategories`, `defaultCategorySeeds`, and `model.Category`.
- Produces: the standalone command applies the same `(parentID, name)` identity and mutable-field reconciliation as API startup.

- [x] **Step 1: Add the equivalent failing standalone test**

Create `backend/scripts/seed_categories/main_test.go`:

```go
package main

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/model"
)

func openCategorySeedTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open category test database: %v", err)
	}
	if err := db.AutoMigrate(&model.Category{}); err != nil {
		t.Fatalf("migrate category test database: %v", err)
	}
	return db
}

func TestEnsureDefaultCategoriesScopesSameNameToParentAndIsIdempotent(t *testing.T) {
	originalSeeds := defaultCategorySeeds
	defaultCategorySeeds = []categorySeed{
		{Name: "Root A", Children: []string{"Shared"}},
		{Name: "Root B", Children: []string{"Shared"}},
	}
	t.Cleanup(func() { defaultCategorySeeds = originalSeeds })

	db := openCategorySeedTestDB(t)
	if err := ensureDefaultCategories(db); err != nil {
		t.Fatalf("first seed: %v", err)
	}

	var firstChildren []model.Category
	if err := db.Where("level = ? AND name = ?", 2, "Shared").Order("parent_id").Find(&firstChildren).Error; err != nil {
		t.Fatalf("load first children: %v", err)
	}
	if len(firstChildren) != 2 {
		t.Fatalf("first shared child count = %d, want 2", len(firstChildren))
	}
	if firstChildren[0].ParentID == nil || firstChildren[1].ParentID == nil || *firstChildren[0].ParentID == *firstChildren[1].ParentID {
		t.Fatalf("shared children have invalid parents: %+v", firstChildren)
	}
	firstIDs := map[uint64]uint64{
		*firstChildren[0].ParentID: firstChildren[0].ID,
		*firstChildren[1].ParentID: firstChildren[1].ID,
	}
	if err := db.Model(&model.Category{}).Where("id = ?", firstChildren[0].ID).Updates(map[string]interface{}{
		"level":  9,
		"status": model.CategoryDisabled,
		"sort":   99,
	}).Error; err != nil {
		t.Fatalf("drift first shared child: %v", err)
	}

	if err := ensureDefaultCategories(db); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	var secondChildren []model.Category
	if err := db.Where("level = ? AND name = ?", 2, "Shared").Order("parent_id").Find(&secondChildren).Error; err != nil {
		t.Fatalf("load second children: %v", err)
	}
	if len(secondChildren) != 2 {
		t.Fatalf("second shared child count = %d, want 2", len(secondChildren))
	}
	for _, child := range secondChildren {
		if child.ParentID == nil || firstIDs[*child.ParentID] != child.ID {
			t.Fatalf("seed changed shared child identity: %+v", child)
		}
		if child.Level != 2 || child.Status != model.CategoryEnabled || child.Sort != 1 {
			t.Fatalf("seed did not reconcile shared child: %+v", child)
		}
	}

	var total int64
	if err := db.Model(&model.Category{}).Count(&total).Error; err != nil {
		t.Fatalf("count categories: %v", err)
	}
	if total != 4 {
		t.Fatalf("category count after repeated seed = %d, want 4", total)
	}
}
```

This duplication is deliberate: the repository currently ships two independently compiled seed implementations, and each test must exercise its real package rather than a mock or source-text assertion.

- [x] **Step 2: Run the focused test and verify RED**

Run:

```bash
cd backend
GOMODCACHE=$(pwd)/.cache/go/mod GOCACHE=$(pwd)/.cache/go/build go test ./scripts/seed_categories -run '^TestEnsureDefaultCategoriesScopesSameNameToParentAndIsIdempotent$' -count=1 -v
```

Expected: FAIL with `first shared child count = 1, want 2` because the standalone command still queries by global name.

- [x] **Step 3: Mirror the minimal production fix**

In `backend/scripts/seed_categories/main.go`, replace
`findOrCreateCategory` with this implementation and remove `sameParentID`:

```go
func findOrCreateCategory(db *gorm.DB, parentID *uint64, level int8, name string, sort int) (model.Category, error) {
	var cat model.Category
	query := db.Model(&model.Category{}).Where("name = ?", name)
	if parentID == nil {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *parentID)
	}
	if err := query.First(&cat).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			cat = model.Category{
				ParentID: parentID,
				Level:    level,
				Name:     name,
				Status:   model.CategoryEnabled,
				Sort:     sort,
			}
			return cat, db.Create(&cat).Error
		}
		return model.Category{}, err
	}
	updates := map[string]interface{}{}
	if cat.Level != level {
		updates["level"] = level
	}
	if cat.Status != model.CategoryEnabled {
		updates["status"] = model.CategoryEnabled
	}
	if cat.Sort != sort {
		updates["sort"] = sort
	}
	if len(updates) > 0 {
		if err := db.Model(&model.Category{}).Where("id = ?", cat.ID).Updates(updates).Error; err != nil {
			return model.Category{}, err
		}
		cat.Level = level
		cat.Status = model.CategoryEnabled
		cat.Sort = sort
	}
	return cat, nil
}
```

Do not refactor the two seed implementations into a shared package in this repair.

- [x] **Step 4: Verify GREEN**

Run the focused standalone test from Step 2 again.

Expected: PASS.

- [x] **Step 5: Compare both implementations**

Read both `findOrCreateCategory` functions side by side and confirm their lookup, create, update, and error paths are behaviorally identical. Run:

```bash
cd backend
GOMODCACHE=$(pwd)/.cache/go/mod GOCACHE=$(pwd)/.cache/go/build go test ./internal/app ./scripts/seed_categories -count=1
```

Expected: both packages PASS.

- [x] **Step 6: Review checkpoint without committing**

Run `git diff --check -- backend/scripts/seed_categories/main.go backend/scripts/seed_categories/main_test.go`. Do not stage or commit.

---

### Task 4: Format, review, and run the complete local suite

**Files:**
- Verify all files from Tasks 1-3.
- Do not edit protected review documents, F-02 documents, inventory files, or order files.

**Interfaces:**
- Consumes: the corrected model and both seed implementations.
- Produces: a formatted, reviewed, locally green working tree ready for isolated MySQL replay.

- [x] **Step 1: Format only the F-16 Go files**

Run:

```bash
gofmt -w backend/internal/model/models.go backend/internal/model/models_test.go backend/internal/app/server.go backend/internal/app/server_categories_test.go backend/scripts/seed_categories/main.go backend/scripts/seed_categories/main_test.go
```

- [x] **Step 2: Rerun all focused F-16 tests uncached**

Run:

```bash
cd backend
GOMODCACHE=$(pwd)/.cache/go/mod GOCACHE=$(pwd)/.cache/go/build go test ./internal/model ./internal/app ./scripts/seed_categories -run 'Category|Categories|FileRecordTableName' -count=1 -v
```

Expected: all selected tests PASS.

- [x] **Step 3: Run the complete backend suite**

Run from repository root:

```bash
make backend-test
```

Expected: all backend packages PASS.

- [x] **Step 4: Request two-stage code review**

Use `superpowers:requesting-code-review` to review first for design/spec compliance and then for implementation quality. Findings must be fixed with a new RED/GREEN cycle when they affect behavior. Rerun Steps 2-3 after any correction.

- [x] **Step 5: Audit scope and protected files**

Run:

```bash
git diff --check
git diff --name-only
git status --short
```

Confirm the F-16 change set contains only the six files listed in the File Map plus this design and plan. Existing F-09 changes may remain present but must not be rewritten accidentally. Confirm protected review documents and F-02 documents remain untracked and unstaged. Do not stage or commit.

---

### Task 5: Replay the complete F-09 matrix on the isolated server

**Files:**
- Synchronize only the approved F-16 source and test files into `/home/yu/services/secondhand-file-schema-acceptance-20260726`.
- Execute the existing `deploy/acceptance/file-record-schema-smoke.sh` there.
- Preserve evidence under `deploy/acceptance/evidence/file-record-schema/`.

**Interfaces:**
- Consumes: the previously authorized acceptance SSH target, the retained isolated directory, Docker Compose project `secondhand-file-schema-acceptance`, and exact confirmation variables required by the smoke harness.
- Produces: fresh evidence for all eight F-09 states, including a successful final `AUTO_MIGRATE=true` application start with the corrected category index metadata.

- [x] **Step 1: Verify the remote target before any cleanup**

On the previously authorized acceptance host, confirm the working directory is exactly:

```text
/home/yu/services/secondhand-file-schema-acceptance-20260726
```

Inspect Docker labels and names. Abort unless the test resources belong to Compose project `secondhand-file-schema-acceptance`. Separately record the production API, web, and MySQL container IDs, states, and restart counts. Do not run commands against the production Compose project or production database.

- [x] **Step 2: Archive prior F-09 evidence and remove only retained isolated resources**

Inside the exact isolated directory, archive the existing evidence with:

```bash
f16_archive_suffix="$(date -u +%Y%m%dT%H%M%SZ)"
if [[ -d deploy/acceptance/evidence/file-record-schema ]]; then
  mv deploy/acceptance/evidence/file-record-schema "deploy/acceptance/evidence/file-record-schema-before-f16-$f16_archive_suffix"
fi
```

Then run `docker compose down --volumes` with all three exact selectors:

```bash
docker compose --project-name secondhand-file-schema-acceptance --env-file deploy/acceptance/.env --file deploy/acceptance/docker-compose.yml down --volumes
```

Verify no container, volume, or network remains with label `com.docker.compose.project=secondhand-file-schema-acceptance`. Retain built images. This cleanup deletes only disposable isolated MySQL data; the archived evidence remains recoverable.

- [x] **Step 3: Synchronize the reviewed F-16 files**

Using the previously authorized SSH target, copy only:

```text
backend/internal/model/models.go
backend/internal/model/models_test.go
backend/internal/app/server.go
backend/internal/app/server_categories_test.go
backend/scripts/seed_categories/main.go
backend/scripts/seed_categories/main_test.go
```

to the same relative paths under `/home/yu/services/secondhand-file-schema-acceptance-20260726`. Do not synchronize `.env`, production configuration, databases, uploads, the protected documents, or unrelated working-tree files.

- [x] **Step 4: Run the full isolated matrix from the first state**

From the isolated directory, run:

```bash
FILE_SCHEMA_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_FILE_SCHEMA_DATA ACCEPTANCE_DB_ENGINE=mysql8.4 make acceptance-file-schema-smoke
```

Expected: legacy rename, canonical no-op, both-table rejection, neither-table rejection, legacy column drift rejection, canonical index drift rejection, full SQL chain, migration-only file flow, and final AutoMigrate compatibility all pass. The script ends with `isolated file schema acceptance passed` and retains the isolated Compose resources for inspection.

- [x] **Step 5: Verify evidence and the fixed AutoMigrate gate**

Confirm:

- `mysql-version.txt` reports MySQL 8.4.8.
- `legacy.txt` includes `file_records_migration_renamed`.
- `canonical.txt` includes `file_records_migration_noop`.
- both drift evidence files contain failures without success markers.
- `full-chain.txt` contains preflight, migration, and postflight success markers.
- `file-flow.txt` contains `PASS: TestFileFlowWithMigrationOnlyMySQL` and no `Duplicate key name 'uk_parent_name'`.
- the matrix result reports success.

Calculate SHA-256 hashes for the fresh evidence files and record them for the final report.

- [x] **Step 6: Recheck production read-only and stop**

Re-read the production container states and restart counts and compare them with Step 1. Verify `https://market.meaningful.ink/healthz` returns HTTP 200 with application `code=0`. Do not deploy, migrate, restart, or write production data.

Stop with the isolated test resources and fresh evidence retained. Report local tests, review results, remote matrix results, evidence hashes, production read-only checks, and the exact uncommitted F-16 file list. Wait for explicit user authorization before staging or committing.
