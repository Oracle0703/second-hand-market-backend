# File Record Schema Alignment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `file_records` the explicit GORM and SQL-migration table contract, safely rename legacy `files` tables, and prove the result in an isolated MySQL environment with `AUTO_MIGRATE=false`.

**Architecture:** Keep historical migration `0001` immutable and add a fail-closed `0005` preflight/up/postflight sequence. Pin the GORM model to `file_records`, add local contract tests, then exercise legacy, canonical, ambiguous, missing, clean-chain, and AutoMigrate-compatibility states in a dedicated acceptance Compose project that cannot target production.

**Tech Stack:** Go 1.22, GORM 1.30, MySQL 8.4, Gin test router, Bash, Docker Compose v2 for remote isolated acceptance only.

## Global Constraints

- `file_records` is the only canonical runtime and final migration-chain table name.
- Do not modify `backend/migrations/0001_init.up.sql` or `backend/migrations/0001_init.down.sql`.
- Do not deploy, execute a production migration, accept an external DSN, or connect the acceptance matrix to production.
- Local verification must run without Docker; executable MySQL verification runs only in the retained isolated acceptance environment.
- Do not change file ownership, license privacy, upload quotas, object storage, cleanup policy, inventory, or order behavior.
- Do not create a destructive `0005` down migration; application rollback retains `file_records`.
- Do not modify, stage, or commit `docs/architecture-evolution-plan-2026-07-24.md`, `docs/first-round-fix-review-2026-07-24.md`, or `docs/second-round-fix-review-2026-07-24.md`.
- Use repository-local Go caches and run `gofmt` on every Go file changed.

---

### Task 1: Pin the GORM table-name contract

**Files:**
- Create: `backend/internal/model/models_test.go`
- Modify: `backend/internal/model/models.go:203`

**Interfaces:**
- Consumes: GORM's `TableName() string` convention for `model.FileRecord`.
- Produces: `func (FileRecord) TableName() string`, returning exactly `file_records`; all existing GORM reads, writes, deletes, and AutoMigrate calls continue to target the production table.

- [ ] **Step 1: Write the failing model contract test**

Create `backend/internal/model/models_test.go`:

```go
package model

import "testing"

func TestFileRecordTableName(t *testing.T) {
	if got := (FileRecord{}).TableName(); got != "file_records" {
		t.Fatalf("FileRecord table name = %q, want %q", got, "file_records")
	}
}
```

- [ ] **Step 2: Run the focused test and verify it fails**

Run:

```bash
cd backend
mkdir -p .cache/go/mod .cache/go/build
GOMODCACHE=$(pwd)/.cache/go/mod GOCACHE=$(pwd)/.cache/go/build go test ./internal/model -run '^TestFileRecordTableName$' -count=1
```

Expected: FAIL to compile because `FileRecord.TableName` is undefined.

- [ ] **Step 3: Add the minimal explicit table-name method**

Add immediately after the `FileRecord` struct in `backend/internal/model/models.go`:

```go
func (FileRecord) TableName() string {
	return "file_records"
}
```

- [ ] **Step 4: Format and rerun the focused test**

Run:

```bash
gofmt -w backend/internal/model/models.go backend/internal/model/models_test.go
cd backend
GOMODCACHE=$(pwd)/.cache/go/mod GOCACHE=$(pwd)/.cache/go/build go test ./internal/model -run '^TestFileRecordTableName$' -count=1
```

Expected: PASS.

- [ ] **Step 5: Verify the task diff does not touch business paths or protected documents**

Run:

```bash
git diff --check
git diff --name-only
git status --short
```

Expected: only `backend/internal/model/models.go` and `backend/internal/model/models_test.go` are task changes; the three protected documents remain untracked.

- [ ] **Step 6: Commit the explicit model contract**

```bash
git add backend/internal/model/models.go backend/internal/model/models_test.go
git commit -m "fix(backend): pin file record table name"
```

### Task 2: Add fail-closed migration 0005 and local artifact tests

**Files:**
- Create: `backend/migrations/0005_file_records_table.preflight.sql`
- Create: `backend/migrations/0005_file_records_table.up.sql`
- Create: `backend/migrations/0005_file_records_table.postflight.sql`
- Create: `backend/migrations/file_records_migration_test.go`

**Interfaces:**
- Consumes: either exactly one legacy `files` table or exactly one canonical `file_records` table in the selected MySQL database.
- Produces: exactly one canonical `file_records` table; markers `file_records_preflight_passed`, `file_records_migration_renamed` or `file_records_migration_noop`, and `file_records_postflight_passed`.
- Produces no down migration: both the previous and corrected application code use `file_records`.

- [ ] **Step 1: Write the failing artifact contract test**

Create `backend/migrations/file_records_migration_test.go`:

```go
package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestFileRecordsMigrationArtifacts(t *testing.T) {
	tests := map[string][]string{
		"0005_file_records_table.preflight.sql": {
			"file_records_preflight",
			"table_name IN ('files', 'file_records')",
			"file_records_preflight_passed",
			"SIGNAL SQLSTATE '45000'",
		},
		"0005_file_records_table.up.sql": {
			"file_records_migration",
			"RENAME TABLE files TO file_records",
			"file_records_migration_renamed",
			"file_records_migration_noop",
			"SIGNAL SQLSTATE '45000'",
		},
		"0005_file_records_table.postflight.sql": {
			"file_records_postflight",
			"table_name = 'file_records'",
			"file_records_postflight_passed",
			"SIGNAL SQLSTATE '45000'",
		},
	}

	for name, required := range tests {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(name)
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			text := string(raw)
			for _, snippet := range required {
				if !strings.Contains(text, snippet) {
					t.Errorf("%s missing %q", name, snippet)
				}
			}
		})
	}
}

func TestFileRecordsMigrationHasNoDestructiveDownScript(t *testing.T) {
	_, err := os.Stat("0005_file_records_table.down.sql")
	if !os.IsNotExist(err) {
		t.Fatalf("0005 down migration must not exist; stat error = %v", err)
	}
}
```

- [ ] **Step 2: Run the artifact test and verify it fails**

Run:

```bash
cd backend
GOMODCACHE=$(pwd)/.cache/go/mod GOCACHE=$(pwd)/.cache/go/build go test ./migrations -run 'TestFileRecordsMigration' -count=1
```

Expected: FAIL because the three `0005` SQL files do not exist.

- [ ] **Step 3: Create the preflight gate**

Create `backend/migrations/0005_file_records_table.preflight.sql` with a stored procedure that:

```sql
DROP PROCEDURE IF EXISTS file_records_preflight;

DELIMITER //
CREATE PROCEDURE file_records_preflight()
BEGIN
  DECLARE files_count BIGINT DEFAULT 0;
  DECLARE file_records_count BIGINT DEFAULT 0;
  DECLARE required_columns BIGINT DEFAULT 0;
  DECLARE primary_keys BIGINT DEFAULT 0;
  DECLARE object_key_uniques BIGINT DEFAULT 0;
  DECLARE biz_created_indexes BIGINT DEFAULT 0;
  DECLARE candidate_table VARCHAR(64);

  SELECT SUM(table_name = 'files'), SUM(table_name = 'file_records')
    INTO files_count, file_records_count
  FROM information_schema.tables
  WHERE table_schema = DATABASE()
    AND table_name IN ('files', 'file_records');

  SET files_count = COALESCE(files_count, 0);
  SET file_records_count = COALESCE(file_records_count, 0);

  IF files_count + file_records_count <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file records preflight: exactly one of files or file_records must exist';
  END IF;

  SET candidate_table = IF(files_count = 1, 'files', 'file_records');

  SELECT COUNT(*) INTO required_columns
  FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = candidate_table
    AND column_name IN (
      'id', 'biz_type', 'object_key', 'url', 'mime_type',
      'size_bytes', 'uploader_type', 'uploader_id', 'scan_status', 'created_at'
    );
  IF required_columns <> 10 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file records preflight: required columns are missing';
  END IF;

  SELECT COUNT(*) INTO primary_keys
  FROM (
    SELECT index_name
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = candidate_table
      AND index_name = 'PRIMARY'
    GROUP BY index_name
    HAVING COUNT(*) = 1
      AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = 'id'
  ) expected_primary;
  IF primary_keys <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file records preflight: primary key on id is missing or drifted';
  END IF;

  SELECT COUNT(*) INTO object_key_uniques
  FROM (
    SELECT index_name
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = candidate_table
      AND non_unique = 0
    GROUP BY index_name
    HAVING COUNT(*) = 1
      AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = 'object_key'
  ) expected_object_key_unique;
  IF object_key_uniques < 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file records preflight: object_key uniqueness is missing';
  END IF;

  SELECT COUNT(DISTINCT first_column.index_name) INTO biz_created_indexes
  FROM information_schema.statistics AS first_column
  INNER JOIN information_schema.statistics AS second_column
    ON second_column.table_schema = first_column.table_schema
   AND second_column.table_name = first_column.table_name
   AND second_column.index_name = first_column.index_name
   AND second_column.seq_in_index = 2
   AND second_column.column_name = 'created_at'
  WHERE first_column.table_schema = DATABASE()
    AND first_column.table_name = candidate_table
    AND first_column.seq_in_index = 1
    AND first_column.column_name = 'biz_type';
  IF biz_created_indexes < 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file records preflight: biz_type,created_at index is missing';
  END IF;
END//
DELIMITER ;

CALL file_records_preflight();
DROP PROCEDURE file_records_preflight;

SELECT 'file_records_preflight_passed' AS migration_gate;
```

- [ ] **Step 4: Create the atomic rename/no-op migration**

Create `backend/migrations/0005_file_records_table.up.sql`:

```sql
DROP PROCEDURE IF EXISTS file_records_migration;

DELIMITER //
CREATE PROCEDURE file_records_migration()
BEGIN
  DECLARE files_count BIGINT DEFAULT 0;
  DECLARE file_records_count BIGINT DEFAULT 0;

  SELECT SUM(table_name = 'files'), SUM(table_name = 'file_records')
    INTO files_count, file_records_count
  FROM information_schema.tables
  WHERE table_schema = DATABASE()
    AND table_name IN ('files', 'file_records');

  SET files_count = COALESCE(files_count, 0);
  SET file_records_count = COALESCE(file_records_count, 0);

  IF files_count = 1 AND file_records_count = 0 THEN
    RENAME TABLE files TO file_records;
    SELECT 'file_records_migration_renamed' AS migration_gate;
  ELSEIF files_count = 0 AND file_records_count = 1 THEN
    SELECT 'file_records_migration_noop' AS migration_gate;
  ELSE
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file records migration: schema changed after preflight';
  END IF;
END//
DELIMITER ;

CALL file_records_migration();
DROP PROCEDURE file_records_migration;
```

- [ ] **Step 5: Create the postflight gate**

Create `backend/migrations/0005_file_records_table.postflight.sql`:

```sql
DROP PROCEDURE IF EXISTS file_records_postflight;

DELIMITER //
CREATE PROCEDURE file_records_postflight()
BEGIN
  DECLARE files_count BIGINT DEFAULT 0;
  DECLARE file_records_count BIGINT DEFAULT 0;
  DECLARE required_columns BIGINT DEFAULT 0;
  DECLARE primary_keys BIGINT DEFAULT 0;
  DECLARE object_key_uniques BIGINT DEFAULT 0;
  DECLARE biz_created_indexes BIGINT DEFAULT 0;

  SELECT SUM(table_name = 'files'), SUM(table_name = 'file_records')
    INTO files_count, file_records_count
  FROM information_schema.tables
  WHERE table_schema = DATABASE()
    AND table_name IN ('files', 'file_records');

  SET files_count = COALESCE(files_count, 0);
  SET file_records_count = COALESCE(file_records_count, 0);

  IF files_count <> 0 OR file_records_count <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file records postflight: canonical table state is absent or drifted';
  END IF;

  SELECT COUNT(*) INTO required_columns
  FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'file_records'
    AND column_name IN (
      'id', 'biz_type', 'object_key', 'url', 'mime_type',
      'size_bytes', 'uploader_type', 'uploader_id', 'scan_status', 'created_at'
    );
  IF required_columns <> 10 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file records postflight: required columns are missing';
  END IF;

  SELECT COUNT(*) INTO primary_keys
  FROM (
    SELECT index_name
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'file_records'
      AND index_name = 'PRIMARY'
    GROUP BY index_name
    HAVING COUNT(*) = 1
      AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = 'id'
  ) expected_primary;
  IF primary_keys <> 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file records postflight: primary key on id is missing or drifted';
  END IF;

  SELECT COUNT(*) INTO object_key_uniques
  FROM (
    SELECT index_name
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'file_records'
      AND non_unique = 0
    GROUP BY index_name
    HAVING COUNT(*) = 1
      AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = 'object_key'
  ) expected_object_key_unique;
  IF object_key_uniques < 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file records postflight: object_key uniqueness is missing';
  END IF;

  SELECT COUNT(DISTINCT first_column.index_name) INTO biz_created_indexes
  FROM information_schema.statistics AS first_column
  INNER JOIN information_schema.statistics AS second_column
    ON second_column.table_schema = first_column.table_schema
   AND second_column.table_name = first_column.table_name
   AND second_column.index_name = first_column.index_name
   AND second_column.seq_in_index = 2
   AND second_column.column_name = 'created_at'
  WHERE first_column.table_schema = DATABASE()
    AND first_column.table_name = 'file_records'
    AND first_column.seq_in_index = 1
    AND first_column.column_name = 'biz_type';
  IF biz_created_indexes < 1 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'file records postflight: biz_type,created_at index is missing';
  END IF;
END//
DELIMITER ;

CALL file_records_postflight();
DROP PROCEDURE file_records_postflight;

SELECT 'file_records_postflight_passed' AS migration_gate;
SELECT COUNT(*) AS file_records_rows FROM file_records;
```

- [ ] **Step 6: Run local migration contract tests**

Run:

```bash
cd backend
gofmt -w migrations/file_records_migration_test.go
GOMODCACHE=$(pwd)/.cache/go/mod GOCACHE=$(pwd)/.cache/go/build go test ./migrations -run 'TestFileRecordsMigration' -count=1
```

Expected: PASS. This is a local artifact contract, not executable MySQL evidence.

- [ ] **Step 7: Inspect SQL scope and commit**

Run:

```bash
git diff --check
git diff -- backend/migrations
git status --short
```

Expected: only the four Task 2 files are new; there is no `0005_file_records_table.down.sql` and no protected document is staged.

```bash
git add backend/migrations/0005_file_records_table.preflight.sql backend/migrations/0005_file_records_table.up.sql backend/migrations/0005_file_records_table.postflight.sql backend/migrations/file_records_migration_test.go
git commit -m "fix(migrations): align file record table name"
```

### Task 3: Add the migration-only MySQL file-flow regression test

**Files:**
- Create: `backend/tests/file_schema_mysql_test.go`

**Interfaces:**
- Consumes: `FILE_SCHEMA_MYSQL_TEST=1` and the acceptance container's existing `DB_DSN`; otherwise the test skips.
- Consumes: a schema already migrated through `0005`.
- Produces: executable evidence that `app.NewServer` works with `AutoMigrate=false`, the public file presign/upload flow writes `file_records`, and a subsequent `AutoMigrate=true` startup creates no competing `files` table.

- [ ] **Step 1: Add the opt-in MySQL integration test**

Create `backend/tests/file_schema_mysql_test.go`:

```go
package tests

import (
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"second-hand-market-backend/backend/internal/app"
)

func TestFileFlowWithMigrationOnlyMySQL(t *testing.T) {
	if os.Getenv("FILE_SCHEMA_MYSQL_TEST") != "1" {
		t.Skip("set FILE_SCHEMA_MYSQL_TEST=1 only in the isolated MySQL acceptance project")
	}
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		t.Fatal("DB_DSN is required for isolated file schema acceptance")
	}

	newConfig := func(autoMigrate bool) app.Config {
		return app.Config{
			AppEnv:                     "test",
			Addr:                       ":0",
			DBDriver:                   "mysql",
			DBDSN:                      dsn,
			JWTAccessSecret:            "file-schema-test-access",
			JWTRefreshSecret:           "file-schema-test-refresh",
			AccessTTL:                  time.Hour,
			RefreshTTL:                 24 * time.Hour,
			AutoMigrate:                autoMigrate,
			FileStorageProvider:        "local",
			FileUploadLocalDir:         t.TempDir(),
			FileUploadMaxBytes:         40 * 1024 * 1024,
			ImageCompressTargetBytes:   20 * 1024 * 1024,
			ImageProcessorDriver:       "passthrough",
			BuyerWechatLoginMode:       "mock",
			BuyerDouyinLoginMode:       "mock",
			BuyerWechatHTTPTimeout:     5 * time.Second,
			BuyerDouyinHTTPTimeout:     5 * time.Second,
		}
	}

	srv, err := app.NewServer(newConfig(false))
	if err != nil {
		t.Fatalf("start migration-only server: %v", err)
	}

	assertFileTableState(t, srv, 0, 1)

	jpeg := []byte{
		0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F',
		0x00, 0x01, 0x01, 0x01, 0x00, 0x48, 0x00, 0x48, 0x00, 0x00,
		0xFF, 0xD9,
	}
	presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type": "MERCHANT_LICENSE",
		"file_name": "migration-only-license.jpg",
		"file_size": len(jpeg),
		"mime_type": "image/jpeg",
	}, nil)
	if presign.Code != 0 {
		t.Fatalf("presign against migration-only schema failed: %+v", presign)
	}

	fileID := numToUint64(presign.Data["file_id"])
	objectKey := str(presign.Data["object_key"])
	upload := requestMultipart(t, srv.Router, http.MethodPost, "/api/v1/files/upload", map[string]string{
		"file_id": fmt.Sprintf("%d", fileID), "object_key": objectKey,
	}, "file", "migration-only-license.jpg", jpeg, nil)
	if upload.Code != 0 {
		t.Fatalf("upload against migration-only schema failed: %+v", upload)
	}
	confirm := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/confirm", map[string]interface{}{
		"file_id": fileID, "object_key": objectKey,
	}, nil)
	if confirm.Code != 0 {
		t.Fatalf("confirm against migration-only schema failed: %+v", confirm)
	}

	var rows int64
	if err := srv.DB.Table("file_records").Where("id = ?", fileID).Count(&rows).Error; err != nil || rows != 1 {
		t.Fatalf("file_records row check: rows=%d err=%v", rows, err)
	}

	autoSrv, err := app.NewServer(newConfig(true))
	if err != nil {
		t.Fatalf("AutoMigrate compatibility start failed: %v", err)
	}
	assertFileTableState(t, autoSrv, 0, 1)
}

func assertFileTableState(t *testing.T, srv *app.Server, wantFiles, wantFileRecords int64) {
	t.Helper()
	var files int64
	var fileRecords int64
	if err := srv.DB.Raw(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'files'`).Scan(&files).Error; err != nil {
		t.Fatalf("count files table: %v", err)
	}
	if err := srv.DB.Raw(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'file_records'`).Scan(&fileRecords).Error; err != nil {
		t.Fatalf("count file_records table: %v", err)
	}
	if files != wantFiles || fileRecords != wantFileRecords {
		t.Fatalf("file table state files=%d file_records=%d, want %d/%d", files, fileRecords, wantFiles, wantFileRecords)
	}
}
```

- [ ] **Step 2: Verify the test skips locally without Docker or a DSN**

Run:

```bash
gofmt -w backend/tests/file_schema_mysql_test.go
cd backend
GOMODCACHE=$(pwd)/.cache/go/mod GOCACHE=$(pwd)/.cache/go/build go test ./tests -run '^TestFileFlowWithMigrationOnlyMySQL$' -count=1 -v
```

Expected: PASS with one explicit SKIP message. A silent pass without the skip message is not acceptable.

- [ ] **Step 3: Run the complete local backend suite**

Run from the repository root:

```bash
make test
```

Expected: all backend packages pass; the opt-in MySQL test is skipped.

- [ ] **Step 4: Commit the opt-in integration test**

```bash
git add backend/tests/file_schema_mysql_test.go
git commit -m "test(backend): cover migration-only file schema"
```

### Task 4: Automate the isolated MySQL state matrix

**Files:**
- Create: `deploy/acceptance/file-record-schema-smoke.sh`
- Modify: `Makefile`
- Modify: `deploy/acceptance/README.md`

**Interfaces:**
- Consumes: existing `deploy/acceptance/.env` and acceptance-only secrets created by `deploy/acceptance/prepare.sh`.
- Consumes: exact confirmation `FILE_SCHEMA_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_FILE_SCHEMA_DATA` and exact engine marker `ACCEPTANCE_DB_ENGINE=mysql8.4`.
- Produces: evidence under ignored `deploy/acceptance/evidence/file-record-schema/` and a retained Compose project named exactly `secondhand-file-schema-acceptance`.
- Never consumes an external DSN, production database name, or production Compose project.

- [ ] **Step 1: Add the Makefile safety gate**

Add `acceptance-file-schema-smoke` to `.PHONY` and add:

```make
acceptance-file-schema-smoke:
	@test "$${FILE_SCHEMA_ACCEPTANCE_CONFIRM:-}" = "I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_FILE_SCHEMA_DATA" || { echo "set FILE_SCHEMA_ACCEPTANCE_CONFIRM for the isolated file schema smoke" >&2; exit 1; }
	@test "$${ACCEPTANCE_DB_ENGINE:-}" = "mysql8.4" || { echo "set ACCEPTANCE_DB_ENGINE=mysql8.4" >&2; exit 1; }
	./deploy/acceptance/file-record-schema-smoke.sh
```

- [ ] **Step 2: Prove the target fails closed without confirmation**

Run:

```bash
make acceptance-file-schema-smoke
```

Expected: FAIL before Docker is called, with the confirmation-variable message.

- [ ] **Step 3: Create the isolated matrix script**

Create executable `deploy/acceptance/file-record-schema-smoke.sh` with these exact contents:

```bash
#!/usr/bin/env bash

set -euo pipefail

base_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
project_name="secondhand-file-schema-acceptance"
evidence_dir="$base_dir/evidence/file-record-schema"
compose=(docker compose --project-name "$project_name" --env-file "$base_dir/.env" --file "$base_dir/docker-compose.yml")

[[ "${FILE_SCHEMA_ACCEPTANCE_CONFIRM:-}" == "I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_FILE_SCHEMA_DATA" ]] || {
  echo "isolated file schema confirmation is missing" >&2
  exit 1
}
[[ "${ACCEPTANCE_DB_ENGINE:-}" == "mysql8.4" ]] || {
  echo "ACCEPTANCE_DB_ENGINE must be mysql8.4" >&2
  exit 1
}
[[ -f "$base_dir/.env" ]] || {
  echo "run deploy/acceptance/prepare.sh first" >&2
  exit 1
}

existing_containers="$("${compose[@]}" ps -aq 2>/dev/null || true)"
existing_volumes="$(docker volume ls --filter "label=com.docker.compose.project=$project_name" -q)"
existing_networks="$(docker network ls --filter "label=com.docker.compose.project=$project_name" -q)"
[[ -z "$existing_containers" && -z "$existing_volumes" && -z "$existing_networks" ]] || {
  echo "refusing to reuse existing $project_name resources" >&2
  exit 1
}

mkdir -p "$evidence_dir"

mysql_sql() {
  local sql="$1"
  "${compose[@]}" exec -T mysql sh -ec '
    MYSQL_PWD="$MYSQL_PASSWORD" exec mysql --protocol=TCP -h 127.0.0.1 \
      -u"$MYSQL_USER" "$MYSQL_DATABASE" --batch --skip-column-names --execute="$1"
  ' sh "$sql"
}

mysql_file() {
  local container_path="$1"
  "${compose[@]}" exec -T mysql sh -ec '
    MYSQL_PWD="$MYSQL_PASSWORD" exec mysql --protocol=TCP -h 127.0.0.1 \
      -u"$MYSQL_USER" "$MYSQL_DATABASE" < "$1"
  ' sh "$container_path"
}

expect_gate_failure() {
  local path="$1"
  if mysql_file "$path"; then
    echo "expected migration gate failure for $path" >&2
    exit 1
  fi
}

reset_file_tables() {
  mysql_sql 'DROP TABLE IF EXISTS file_records; DROP TABLE IF EXISTS files;'
}

apply_0001() {
  mysql_file /acceptance/migrations/0001_init.up.sql
}

run_0005() {
  mysql_file /acceptance/migrations/0005_file_records_table.preflight.sql
  mysql_file /acceptance/migrations/0005_file_records_table.up.sql
  mysql_file /acceptance/migrations/0005_file_records_table.postflight.sql
}

"${compose[@]}" up -d --wait mysql
"${compose[@]}" ps mysql

# Legacy files-only state: preserve the exact sentinel row across rename.
reset_file_tables
apply_0001
mysql_sql "INSERT INTO files (id,biz_type,object_key,url,mime_type,size_bytes,uploader_type,uploader_id,scan_status) VALUES (900001,'MERCHANT_LICENSE','f09/legacy','/uploads/f09-legacy.jpg','image/jpeg',22,'PUBLIC',NULL,'PENDING')"
legacy_before="$(mysql_sql "SELECT SHA2(CONCAT_WS('|',id,biz_type,object_key,url,mime_type,size_bytes,uploader_type,COALESCE(uploader_id,'NULL'),scan_status,created_at),256) FROM files WHERE id=900001")"
run_0005 | tee "$evidence_dir/legacy.txt"
legacy_after="$(mysql_sql "SELECT SHA2(CONCAT_WS('|',id,biz_type,object_key,url,mime_type,size_bytes,uploader_type,COALESCE(uploader_id,'NULL'),scan_status,created_at),256) FROM file_records WHERE id=900001")"
[[ "$legacy_before" == "$legacy_after" ]] || { echo "legacy sentinel changed" >&2; exit 1; }

# Canonical file_records-only state: 0005 must be a no-op.
reset_file_tables
apply_0001
mysql_sql 'RENAME TABLE files TO file_records'
mysql_sql "INSERT INTO file_records (id,biz_type,object_key,url,mime_type,size_bytes,uploader_type,uploader_id,scan_status) VALUES (900002,'PRODUCT_IMAGE','f09/canonical','/uploads/f09-canonical.jpg','image/jpeg',22,'MERCHANT',7,'PASS')"
canonical_before="$(mysql_sql "SELECT SHA2(CONCAT_WS('|',id,biz_type,object_key,url,mime_type,size_bytes,uploader_type,uploader_id,scan_status,created_at),256) FROM file_records WHERE id=900002")"
run_0005 | tee "$evidence_dir/canonical.txt"
canonical_after="$(mysql_sql "SELECT SHA2(CONCAT_WS('|',id,biz_type,object_key,url,mime_type,size_bytes,uploader_type,uploader_id,scan_status,created_at),256) FROM file_records WHERE id=900002")"
[[ "$canonical_before" == "$canonical_after" ]] || { echo "canonical sentinel changed" >&2; exit 1; }
grep -q file_records_migration_noop "$evidence_dir/canonical.txt"

# Ambiguous state: preflight must fail and preserve both tables.
reset_file_tables
apply_0001
mysql_sql 'CREATE TABLE file_records LIKE files'
mysql_sql "INSERT INTO files (id,biz_type,object_key,url,mime_type,size_bytes,uploader_type,scan_status) VALUES (900003,'OTHER','f09/files','/uploads/f09-files.jpg','image/jpeg',22,'PUBLIC','PENDING')"
mysql_sql "INSERT INTO file_records (id,biz_type,object_key,url,mime_type,size_bytes,uploader_type,scan_status) VALUES (900004,'OTHER','f09/file-records','/uploads/f09-file-records.jpg','image/jpeg',22,'PUBLIC','PENDING')"
expect_gate_failure /acceptance/migrations/0005_file_records_table.preflight.sql
[[ "$(mysql_sql "SELECT COUNT(*) FROM files WHERE id=900003")" == "1" ]]
[[ "$(mysql_sql "SELECT COUNT(*) FROM file_records WHERE id=900004")" == "1" ]]

# Missing state: preflight must fail without creating either table.
reset_file_tables
expect_gate_failure /acceptance/migrations/0005_file_records_table.preflight.sql
[[ "$(mysql_sql "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ('files','file_records')")" == "0" ]]

# Clean full migration chain, migration-only API flow, then AutoMigrate compatibility.
mysql_file /acceptance/migrations/0001_init.down.sql
apply_0001
mysql_file /acceptance/migrations/0002_buyer_domain.up.sql
mysql_file /acceptance/migrations/0003_buyer_auth_provider.up.sql
mysql_file /acceptance/migrations/0004_merchant_multi_stock.preflight.sql
mysql_file /acceptance/migrations/0004_merchant_multi_stock.up.sql
mysql_file /acceptance/migrations/0004_merchant_multi_stock.postflight.sql
run_0005 | tee "$evidence_dir/full-chain.txt"
"${compose[@]}" --profile tools build bootstrap-admin
"${compose[@]}" --profile tools run --rm \
  -e FILE_SCHEMA_MYSQL_TEST=1 \
  bootstrap-admin go test ./tests -run '^TestFileFlowWithMigrationOnlyMySQL$' -count=1 -v \
  | tee "$evidence_dir/file-flow.txt"

grep -q file_records_preflight_passed "$evidence_dir/full-chain.txt"
grep -Eq 'file_records_migration_(renamed|noop)' "$evidence_dir/full-chain.txt"
grep -q file_records_postflight_passed "$evidence_dir/full-chain.txt"
grep -q -- '--- PASS: TestFileFlowWithMigrationOnlyMySQL' "$evidence_dir/file-flow.txt"

echo "isolated file schema acceptance passed"
echo "resources retained for inspection under Compose project: $project_name"
```

Do not add automatic `docker compose down -v`; failed and successful evidence must remain inspectable. Cleanup is a separately reviewed destructive action against the exact project name.

- [ ] **Step 4: Make the script executable and add operator documentation**

Run:

```bash
chmod +x deploy/acceptance/file-record-schema-smoke.sh
```

Add a `File record schema acceptance` section to `deploy/acceptance/README.md` containing:

````markdown
## File record schema acceptance

This matrix uses the separate Compose project
`secondhand-file-schema-acceptance`; it never accepts a DSN and never targets
production. Prepare acceptance-only secrets with `./prepare.sh`, then run from
the repository root:

```bash
FILE_SCHEMA_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_FILE_SCHEMA_DATA \
ACCEPTANCE_DB_ENGINE=mysql8.4 \
make acceptance-file-schema-smoke
```

The command covers files-only rename, file_records-only no-op, both-table and
neither-table failures, the full SQL migration chain, an `AUTO_MIGRATE=false`
file upload flow, and one `AUTO_MIGRATE=true` compatibility startup. It leaves
the isolated project and evidence in place for inspection. It does not deploy
or migrate production.
````

- [ ] **Step 5: Verify local safety behavior and shell syntax**

Run:

```bash
bash -n deploy/acceptance/file-record-schema-smoke.sh
make acceptance-file-schema-smoke
```

Expected: `bash -n` passes; the Make target fails before Docker because the confirmation variables are absent.

- [ ] **Step 6: Commit the locally verified acceptance harness**

```bash
git add Makefile deploy/acceptance/file-record-schema-smoke.sh deploy/acceptance/README.md
git commit -m "test(acceptance): cover file table migration states"
```

- [ ] **Step 7: Run the committed matrix only on the retained isolated acceptance host**

Synchronize the exact reviewed commit into the dedicated acceptance checkout. From that checkout, run:

```bash
FILE_SCHEMA_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_FILE_SCHEMA_DATA \
ACCEPTANCE_DB_ENGINE=mysql8.4 \
make acceptance-file-schema-smoke
```

Expected: all four state cases, the full chain, the migration-only file flow, and AutoMigrate compatibility pass. Stop if the fixed Compose project already exists; do not delete it automatically and do not substitute production credentials or a production checkout. If the remote matrix fails, keep the evidence, return to the responsible task, add a focused fix commit, and rerun the entire matrix before Task 5.

### Task 5: Close F-09 documentation and run final verification

**Files:**
- Modify: `docs/data-model.md:227`
- Modify: `README.md`
- Modify: `docs/release-readiness.md`
- Modify: `docs/isolated-acceptance-results-2026-07-24.md`
- Modify: `docs/deep-code-review-2026-07-24.md`
- Modify: `docs/full-project-code-review-2026-07-24.md`
- Modify: `docs/production-hardening-repair-plan-2026-07-24.md`
- Modify: `docs/superpowers/specs/2026-07-26-file-record-schema-alignment-design.md`

**Interfaces:**
- Consumes: reviewed commits and passing local plus isolated evidence from Tasks 1-4.
- Produces: tracked documentation that distinguishes code/migration completion from production deployment; F-09 is closed locally and in isolated acceptance but explicitly not described as run in production.

- [ ] **Step 1: Update the canonical data-model name**

Change the heading in `docs/data-model.md` from:

```markdown
### 2.10 files（文件元数据）
```

to:

```markdown
### 2.10 file_records（文件元数据）
```

Add below the index list:

```markdown
表名以完整 SQL migration 链和 `FileRecord.TableName()` 的
`file_records` 为准；历史 `0001` 中的 `files` 由 `0005` 兼容迁移收敛。
```

- [ ] **Step 2: Document the migration sequence and deployment boundary**

Add the three canonical `0005` files and their exact sequence to `README.md` and `docs/release-readiness.md`:

```text
0005_file_records_table.preflight.sql
-> 0005_file_records_table.up.sql
-> 0005_file_records_table.postflight.sql
```

Use this exact status block in both documents after isolated acceptance passes:

```markdown
The canonical file table is `file_records`. Migration `0005` renames the
legacy `files`-only state, treats the existing `file_records`-only state as a
verified no-op, and stops when both or neither table exists. Isolated MySQL
8.4 acceptance passed with `AUTO_MIGRATE=false` and a subsequent
`AUTO_MIGRATE=true` compatibility start. Migration `0005` has not been run in
production and still requires separate production authorization.
```

- [ ] **Step 3: Record isolated evidence**

Append a dated F-09 section to `docs/isolated-acceptance-results-2026-07-24.md` with the exact commit under test, MySQL version, all six matrix outcomes, local test command, remote acceptance command, and retained evidence paths. Do not include credentials, DSNs, file contents, or production identifiers.

- [ ] **Step 4: Close only F-09 in tracked review documents**

In the deep review, full review, repair plan, and design spec:

- mark F-09/R-09 as fixed in code and isolated acceptance;
- retain the original evidence as historical evidence;
- say production already uses `file_records` but `0005` was not executed there;
- do not close F-02, F-06, F-13, or F-16;
- do not alter inventory/order findings;
- set the design spec status to `Implemented and isolated-verified` only after the matrix passes.

- [ ] **Step 5: Run fresh local verification**

Run from the repository root:

```bash
git diff --check
bash -n deploy/acceptance/file-record-schema-smoke.sh
make test
```

Expected: diff check and shell syntax pass; all backend packages pass with the opt-in MySQL test explicitly skipped locally.

- [ ] **Step 6: Verify scope and protected files before staging**

Run:

```bash
git diff --name-only
git status --short
git diff -- backend/internal/app backend/internal/inventory backend/internal/order frontend miniapp
```

Expected: the business-path diff is empty; the three protected documents remain untracked and are absent from the staged set.

- [ ] **Step 7: Stage only the tracked F-09 documentation and commit**

```bash
git add docs/data-model.md README.md docs/release-readiness.md docs/isolated-acceptance-results-2026-07-24.md docs/deep-code-review-2026-07-24.md docs/full-project-code-review-2026-07-24.md docs/production-hardening-repair-plan-2026-07-24.md docs/superpowers/specs/2026-07-26-file-record-schema-alignment-design.md
git diff --cached --check
git diff --cached --name-status
git commit -m "docs(backend): close file table schema finding"
```

- [ ] **Step 8: Perform the final branch audit**

Run:

```bash
git log -7 --oneline
git status --short --branch
git diff 2be796d..HEAD --name-only
```

Expected: the implementation-plan commit plus at least five focused F-09 implementation commits follow design commit `2be796d`; only the three protected review documents remain untracked; no production deployment or migration has occurred.
