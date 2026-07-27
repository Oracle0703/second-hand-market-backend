# F-11 Buyer Intent Open Uniqueness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Permit unlimited closed buyer-intent history while enforcing at most
one open intent per buyer/product across MySQL 8.4, SQLite, GORM startup, and
concurrent API requests.

**Architecture:** MySQL owns a stored nullable generated marker and a unique
key on buyer, product, and marker; SQLite owns an equivalent partial unique
index. A focused dialect helper converges only recognized development states,
an always-on verifier rejects non-final startup state, and centralized intent
state/error helpers keep every API path fail-closed. Formal migration 0009 and
an isolated MySQL 8.4 matrix provide the production-facing evidence chain
without changing production.

**Tech Stack:** Go 1.22, Gin 1.10, GORM 1.30 with TranslateError, glebarez
SQLite, MySQL 8.4, SQL stored procedures, Bash, Docker Compose, Go testing.

## Global Constraints

- Implement only F-11. Do not implement F-10, F-12, F-15, or another finding
  in these commits.
- Keep migration 0002 immutable. Add migration
  0009_buyer_intent_open_uniqueness as preflight/up/postflight only.
- MySQL final schema is a stored nullable TINYINT generated column whose
  expression is CASE WHEN is_open = 1 THEN 1 ELSE NULL END, plus exactly one
  uk_buyer_intent_open(buyer_id, product_id, open_marker).
- SQLite final schema has no open_marker column and has exactly one unique
  partial index on (buyer_id, product_id) WHERE is_open = 1.
- Application code and GORM must never create or update open_marker.
- NEW/CONTACTED plus is_open=true and CLOSED plus is_open=false are the only
  valid persisted state combinations.
- An existing valid open row is HTTP 409/code 10010. Unknown duplicate-key
  causes, re-query failures, and state drift are HTTP 500/code 20001.
- AUTO_MIGRATE=true may converge only approved states; AUTO_MIGRATE=false
  performs no DDL and accepts only the final dialect schema.
- Migration preflight/up may not contain business-table INSERT, UPDATE, DELETE,
  REPLACE, or TRUNCATE. Do not add a 0009 down migration.
- Use RED -> GREEN tests and gofmt all Go edits. Add no dependency.
- Never read, modify, stage, commit, or transfer .tmp/ or the three protected
  untracked review documents.
- Never read or modify backend/app.db, production data, credentials, uploads,
  production configuration, or production services.
- The F-11 remote directory is
  /home/yu/services/secondhand-buyer-intent-acceptance-20260727 and the Compose
  project is secondhand-buyer-intent-acceptance.
- This plan does not authorize source transfer, SSH execution, Docker actions,
  production inspection, or production migration. Obtain a new exact approval
  before Task 9.
- F-12 implementation remains blocked until Task 9 records the accepted F-11
  commit range and isolated MySQL 8.4 evidence.

---

## File Map

| Path | Responsibility |
| --- | --- |
| backend/internal/model/models.go | Remove legacy GORM index ownership; expose read-only migration-ignored OpenMarker |
| backend/internal/app/server.go | Enable GORM error translation and invoke convergence/verification at the correct startup points |
| backend/internal/app/buyer_intent_schema.go | Inspect, converge, and verify MySQL/SQLite F-11 schema plus persisted state |
| backend/internal/app/buyer_intent_schema_test.go | Focused dialect-helper, startup, drift, and write-protection tests |
| backend/internal/app/buyer_intent_state.go | Canonical state validator and duplicate-key reconciliation |
| backend/internal/app/buyer_intent_state_test.go | Exhaustive state/error unit tests |
| backend/internal/app/buyer_handlers.go | Apply verified open-row creation and buyer read-path validation |
| backend/internal/app/merchant_intent_handlers.go | Validate merchant list/detail/contact/close paths before success |
| backend/tests/integration_flow_test.go | Preserve HTTP status in API test responses |
| backend/tests/buyer_flow_test.go | Three close cycles, concurrency, response, and state-drift regression coverage |
| backend/migrations/0009_buyer_intent_open_uniqueness.preflight.sql | Read-only MySQL schema/data gate |
| backend/migrations/0009_buyer_intent_open_uniqueness.up.sql | Resumable additive generated-column/index migration |
| backend/migrations/0009_buyer_intent_open_uniqueness.postflight.sql | Exact final MySQL shape/data verification |
| backend/migrations/buyer_intent_open_uniqueness_migration_test.go | Static migration and acceptance-harness contracts |
| deploy/acceptance/file-record-schema-smoke.sh | Keep F-09 migration-only API replay on the current full chain through 0009 |
| deploy/acceptance/file-binding-authorization-smoke.sh | Keep F-02 API replay on the current full chain through 0009 |
| deploy/acceptance/license-file-privacy-smoke.sh | Keep F-13 API replay on the current full chain through 0009 |
| deploy/acceptance/anonymous-upload-governance-smoke.sh | Keep F-06 API replay on the current full chain through 0009 |
| deploy/acceptance/session-revocation-smoke.sh | Keep F-14 API replay on the current full chain through 0009 |
| backend/tests/buyer_intent_mysql_test.go | Opt-in MySQL API/schema/runtime acceptance |
| deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh | Isolated state matrix, hashes, sanitized evidence, and production snapshot guard |
| deploy/acceptance/README.md | Exact F-11 isolated execution and retention contract |
| Makefile | Guarded acceptance-buyer-intent-smoke target |
| docs/miniapp-buyer-data-model.md | Replace the obsolete three-column index contract |
| docs/full-project-code-review-2026-07-24.md | Append F-11 code/server/production status without rewriting history |
| docs/release-readiness.md | Track F-11 code, test-server, and production states independently |
| docs/superpowers/reviews/2026-07-27-buyer-intent-open-uniqueness-isolated-acceptance.md | Sanitized accepted server evidence after Task 9 |
| docs/superpowers/specs/2026-07-27-buyer-intent-open-uniqueness-design.md | Advance status only when evidence exists |

## Shared Interfaces

The plan fixes these unexported app-package interfaces. Later tasks must use the
same names and signatures:

~~~go
func migrateBuyerIntentOpenUniqueness(db *gorm.DB) error
func verifyBuyerIntentOpenUniqueness(db *gorm.DB) error

func validateBuyerIntentStatus(status string, isOpen bool) error
func validateBuyerIntentState(intent model.BuyerIntent) error
func findOpenBuyerIntent(db *gorm.DB, buyerID, productID uint64) (bool, error)
func classifyBuyerIntentCreateError(
	db *gorm.DB,
	createErr error,
	buyerID uint64,
	productID uint64,
) error
~~~

The schema helper owns DDL. The state helper owns runtime semantics. Handlers
must not duplicate either decision table.

---

### Task 1: Lock the model and translated database-error contract

**Files:**
- Modify: backend/internal/model/models.go:330
- Modify: backend/internal/app/server.go:118-129
- Create: backend/internal/app/buyer_intent_schema_test.go

**Interfaces:**
- Consumes: current BuyerIntent model and openDB(Config).
- Produces: BuyerIntent.OpenMarker and translated gorm.ErrDuplicatedKey for
  Task 3; it does not yet add dialect DDL.

- [ ] **Step 1: Write the failing model and translation tests**

Add package-app tests that parse the model and exercise the application-owned
database opener:

~~~go
func TestBuyerIntentModelDoesNotOwnLegacyUniqueIndex(t *testing.T) {
	db, err := openDB(Config{
		DBDriver: "sqlite",
		DBDSN:    "file:model_contract?mode=memory&cache=shared",
	})
	if err != nil {
		t.Fatal(err)
	}
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(&model.BuyerIntent{}); err != nil {
		t.Fatal(err)
	}
	for _, fieldName := range []string{"BuyerID", "ProductID", "IsOpen"} {
		field := stmt.Schema.LookUpField(fieldName)
		if strings.Contains(field.Tag.Get("gorm"), "uk_buyer_product_open") {
			t.Fatalf("%s still owns legacy unique index", fieldName)
		}
	}
	marker := stmt.Schema.LookUpField("OpenMarker")
	if marker == nil || marker.Creatable || marker.Updatable || !marker.IgnoreMigration {
		t.Fatalf("OpenMarker permissions = %#v", marker)
	}
}

func TestOpenDBTranslatesDuplicateKeys(t *testing.T) {
	db, err := openDB(Config{
		DBDriver: "sqlite",
		DBDSN:    "file:" + strings.ReplaceAll(t.Name(), "/", "_") +
			"?mode=memory&cache=shared",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(
		"CREATE TABLE duplicate_probe (id INTEGER PRIMARY KEY, value TEXT UNIQUE)",
	).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO duplicate_probe(value) VALUES (?)", "same").Error; err != nil {
		t.Fatal(err)
	}
	err = db.Exec("INSERT INTO duplicate_probe(value) VALUES (?)", "same").Error
	if !errors.Is(err, gorm.ErrDuplicatedKey) {
		t.Fatalf("duplicate error = %v, want gorm.ErrDuplicatedKey", err)
	}
}
~~~

- [ ] **Step 2: Run the focused tests and confirm RED**

Run:

~~~bash
cd backend
go test ./internal/app -run 'TestBuyerIntentModelDoesNotOwnLegacyUniqueIndex|TestOpenDBTranslatesDuplicateKeys' -count=1
~~~

Expected: FAIL because OpenMarker does not exist, legacy index tags remain, and
openDB does not enable TranslateError.

- [ ] **Step 3: Make OpenMarker read-only and enable translation**

Change BuyerIntent fields to this exact contract:

~~~go
BuyerID        uint64  `gorm:"index:idx_buyer_intent_buyer_created,priority:1"`
SourceDeviceID *string `gorm:"size:64;index:idx_buyer_intent_source_device_created,priority:1"`
ProductID      uint64  `gorm:"index:idx_buyer_intent_product_open,priority:1"`
MerchantID     uint64  `gorm:"index:idx_buyer_intent_merchant_status_created,priority:1"`
Status         string  `gorm:"size:16;index:idx_buyer_intent_merchant_status_created,priority:2"`
IsOpen         bool    `gorm:"index:idx_buyer_intent_product_open,priority:2"`
OpenMarker     *uint8  `gorm:"->;-:migration"`
~~~

Change openDB to:

~~~go
return gorm.Open(dial, &gorm.Config{
	Logger:         logger.Default.LogMode(logger.Silent),
	TranslateError: true,
})
~~~

Do not change the script-only GORM openers under backend/scripts; F-11 runtime
classification is owned by the API server.

- [ ] **Step 4: Run focused and package tests**

Run:

~~~bash
cd backend
gofmt -w internal/model/models.go internal/app/server.go internal/app/buyer_intent_schema_test.go
go test ./internal/app -run 'TestBuyerIntentModelDoesNotOwnLegacyUniqueIndex|TestOpenDBTranslatesDuplicateKeys' -count=1
go test ./internal/app ./internal/model -count=1
~~~

Expected: PASS. No new F-11 index exists yet; Task 2 owns it.

- [ ] **Step 5: Commit the model/error boundary**

~~~bash
git add backend/internal/model/models.go backend/internal/app/server.go backend/internal/app/buyer_intent_schema_test.go
git commit -m "fix(buyer): define open intent model contract"
~~~

---

### Task 2: Add dialect convergence and fail-closed startup verification

**Files:**
- Create: backend/internal/app/buyer_intent_schema.go
- Modify: backend/internal/app/buyer_intent_schema_test.go
- Modify: backend/internal/app/server.go:36-58,131-158

**Interfaces:**
- Consumes: the Task 1 migration-ignored OpenMarker model.
- Produces: migrateBuyerIntentOpenUniqueness and
  verifyBuyerIntentOpenUniqueness for NewServer and Tasks 5-7.

- [ ] **Step 1: Add RED tests for the approved SQLite state machine**

Build raw fixtures with a canonical BuyerIntent table, the exact legacy index,
the exact new partial index, both indexes, or no index. Cover these named tests:

~~~go
func TestMigrateBuyerIntentOpenUniquenessSQLiteFreshEmpty(t *testing.T)
func TestMigrateBuyerIntentOpenUniquenessSQLiteLegacy(t *testing.T)
func TestMigrateBuyerIntentOpenUniquenessSQLiteBothIndexes(t *testing.T)
func TestMigrateBuyerIntentOpenUniquenessSQLiteFinalIsIdempotent(t *testing.T)
func TestMigrateBuyerIntentOpenUniquenessSQLiteRejectsDrift(t *testing.T)
func TestVerifyBuyerIntentOpenUniquenessSQLiteRequiresFinalState(t *testing.T)
func TestBuyerIntentSQLiteConstraintAllowsHistoryAndRejectsSecondOpen(t *testing.T)
func TestNewServerVerifiesBuyerIntentSchemaWithAutoMigrateDisabled(t *testing.T)
~~~

The legacy fixture must use:

~~~sql
CREATE UNIQUE INDEX uk_buyer_product_open
  ON buyer_intents (buyer_id, product_id, is_open);
~~~

The final fixture must use:

~~~sql
CREATE UNIQUE INDEX uk_buyer_intent_open
  ON buyer_intents (buyer_id, product_id)
  WHERE is_open = 1;
~~~

For every successful convergence, capture rows before and after and assert exact
equality. Drift subtests must include:

~~~text
non-empty table with neither relevant index
physical open_marker column
legacy index with wrong order or non-unique definition
new index with wrong order, non-unique definition, or WHERE is_open = 0
same open-only shape under another index name
unknown NEW/false, CONTACTED/false, CLOSED/true, and BOGUS rows
two open rows after removing all relevant constraints
~~~

For `TestNewServerVerifiesBuyerIntentSchemaWithAutoMigrateDisabled`, create a
complete reusable SQLite database by starting `NewServer` once with
`AUTO_MIGRATE=true`, then close its SQL pool. Reopen the same DSN with
`AUTO_MIGRATE=false`: the exact final state must start successfully. In
separate databases, replace only the F-11 partial index with the exact legacy
index, a drifted lookalike, or no F-11 index; each startup must fail. Snapshot
the `buyer_intents` row set and all `sqlite_master` table/index SQL before and
after every disabled-migration attempt and require byte-for-byte equality, so
the test proves rejection did not execute DDL or mutate rows. Do not use an
incomplete one-table fixture whose later category seeding could mask the F-11
verifier result.

- [ ] **Step 2: Run the schema tests and confirm RED**

Run:

~~~bash
cd backend
go test ./internal/app -run 'Test(MigrateBuyerIntent|VerifyBuyerIntent|BuyerIntentSQLiteConstraint|NewServerVerifiesBuyerIntent)' -count=1
~~~

Expected: FAIL because the convergence/verifier functions do not exist.

- [ ] **Step 3: Implement one focused schema owner**

Create buyer_intent_schema.go with these state types:

~~~go
const (
	buyerIntentLegacyIndex = "uk_buyer_product_open"
	buyerIntentOpenIndex   = "uk_buyer_intent_open"
)

type buyerIntentIndexState struct {
	Present bool
	Exact   bool
}

type buyerIntentSchemaState struct {
	Rows             int64
	MarkerPresent    bool
	MarkerExact      bool
	Legacy           buyerIntentIndexState
	Open             buyerIntentIndexState
	RelevantLookalike bool
}
~~~

Implement the shared data gate exactly once:

~~~go
func verifyBuyerIntentRows(db *gorm.DB) error {
	var invalid int64
	err := db.Table("buyer_intents").Where(`
		CASE
			WHEN status IN ? AND is_open = 1 THEN 0
			WHEN status = ? AND is_open = 0 THEN 0
			ELSE 1
		END = 1`,
		[]string{model.IntentNew, model.IntentContacted},
		model.IntentClosed,
	).Count(&invalid).Error
	if err != nil {
		return fmt.Errorf("verify buyer intent state: %w", err)
	}
	if invalid != 0 {
		return fmt.Errorf("buyer intent state is invalid")
	}

	var duplicateGroups int64
	err = db.Raw(`
		SELECT COUNT(*) FROM (
			SELECT buyer_id, product_id
			FROM buyer_intents
			WHERE is_open = ?
			GROUP BY buyer_id, product_id
			HAVING COUNT(*) > 1
		) AS duplicate_open_intents`, true).Scan(&duplicateGroups).Error
	if err != nil {
		return fmt.Errorf("verify buyer intent open groups: %w", err)
	}
	if duplicateGroups != 0 {
		return fmt.Errorf("buyer intent open uniqueness is violated")
	}
	return nil
}
~~~

Before the data query, use PRAGMA table_xinfo('buyer_intents') on SQLite and
information_schema.columns on MySQL to require buyer_id, product_id, status,
and is_open with their canonical non-null semantics. SQLite table_xinfo must
also prove open_marker is absent; it catches generated/hidden columns that
table_info can omit.

Use PRAGMA index_list('buyer_intents'), PRAGMA index_info for each named index,
and sqlite_master.sql to require exact order, uniqueness, and partial predicate.
Ignore `PRIMARY`/rowid ownership and the independent single-column `intent_no`
unique index. Treat every other unique index containing both `buyer_id` and
`product_id` as F-11-relevant: only the exact approved legacy index during
convergence and the exact approved partial index are allowed. This rejects a
lookalike under another name, extra `is_open`/`open_marker` members, wrong
order, and an additional duplicate key without guessing from the index name.

The SQLite migration branch must execute only this sequence after inspecting an
approved state:

~~~go
if !state.Open.Present {
	if err := db.Exec(`
		CREATE UNIQUE INDEX uk_buyer_intent_open
		ON buyer_intents (buyer_id, product_id)
		WHERE is_open = 1`).Error; err != nil {
		return fmt.Errorf("create buyer intent open index: %w", err)
	}
}
if err := verifySQLiteBuyerIntentOpenIndex(db); err != nil {
	return err
}
if state.Legacy.Present {
	if err := db.Exec("DROP INDEX uk_buyer_product_open").Error; err != nil {
		return fmt.Errorf("drop legacy buyer intent index: %w", err)
	}
}
return verifyBuyerIntentOpenUniqueness(db)
~~~

The MySQL branch inspects information_schema.columns and
information_schema.statistics. Normalize generation_expression by removing
ASCII whitespace, backticks, and parentheses and lowercasing; require exactly:

~~~text
casewhenis_open=1then1elsenullend
~~~

Also require data_type=tinyint, column_type=tinyint, is_nullable=YES,
is_generated=ALWAYS, and extra containing STORED GENERATED. Use this resumable
DDL:

~~~sql
ALTER TABLE buyer_intents
  ADD COLUMN open_marker TINYINT
    GENERATED ALWAYS AS (
      CASE WHEN is_open = 1 THEN 1 ELSE NULL END
    ) STORED AFTER is_open;

ALTER TABLE buyer_intents
  ADD UNIQUE KEY uk_buyer_intent_open
    (buyer_id, product_id, open_marker);

ALTER TABLE buyer_intents
  DROP INDEX uk_buyer_product_open;
~~~

Allow MySQL states only as specified:

~~~go
switch {
case state.Rows == 0 && !state.MarkerPresent &&
	!state.Legacy.Present && !state.Open.Present:
	// Empty GORM-created development table.
case !state.MarkerPresent && state.Legacy.Exact && !state.Open.Present:
	// Legacy formal schema.
case state.MarkerExact && state.Legacy.Exact && !state.Open.Present:
	// Interrupted after generated column.
case state.MarkerExact && state.Legacy.Exact && state.Open.Exact:
	// Interrupted after new key.
case state.MarkerExact && !state.Legacy.Present && state.Open.Exact:
	return verifyBuyerIntentOpenUniqueness(db)
default:
	return fmt.Errorf("buyer intent uniqueness schema is missing or drifted")
}
~~~

Do not use the empty-table exception in verifyBuyerIntentOpenUniqueness. The
verifier accepts only MySQL marker+new/no-legacy or SQLite partial-new/no-marker/
no-legacy.

- [ ] **Step 4: Wire convergence and verification into startup**

At the end of migrate, after migrateFileQuotaGuard:

~~~go
if err := migrateBuyerIntentOpenUniqueness(db); err != nil {
	return err
}
return nil
~~~

In NewServer, keep convergence conditional but verify unconditionally before
seedDefaults:

~~~go
if cfg.AutoMigrate {
	if err := migrate(db); err != nil {
		return nil, err
	}
}
if err := verifyBuyerIntentOpenUniqueness(db); err != nil {
	return nil, err
}
if err := seedDefaults(db); err != nil {
	return nil, err
}
~~~

AUTO_MIGRATE=false must not call migrate or any DDL helper.

- [ ] **Step 5: Run RED/GREEN schema and startup tests**

Run:

~~~bash
cd backend
gofmt -w internal/app/buyer_intent_schema.go internal/app/buyer_intent_schema_test.go internal/app/server.go
go test ./internal/app -run 'Test(MigrateBuyerIntent|VerifyBuyerIntent|BuyerIntentSQLiteConstraint|NewServerVerifiesBuyerIntent)' -count=1
go test ./internal/app -count=1
~~~

Expected: PASS. The fixture row snapshots are unchanged; final SQLite has one
partial index and no physical marker.

- [ ] **Step 6: Commit the schema owner**

~~~bash
git add backend/internal/app/buyer_intent_schema.go backend/internal/app/buyer_intent_schema_test.go backend/internal/app/server.go
git commit -m "fix(buyer): enforce open intent schema"
~~~

---

### Task 3: Centralize runtime state and duplicate-key semantics

**Files:**
- Create: backend/internal/app/buyer_intent_state.go
- Create: backend/internal/app/buyer_intent_state_test.go
- Modify: backend/internal/app/buyer_handlers.go:943-1146
- Modify: backend/internal/app/merchant_intent_handlers.go:15-255
- Modify: backend/tests/integration_flow_test.go:21-24,106-124
- Modify: backend/tests/buyer_flow_test.go:333-446

**Interfaces:**
- Consumes: Task 1 translated gorm.ErrDuplicatedKey and Task 2 final index.
- Produces: all Shared Interfaces state functions and fixed intent API
  semantics consumed by Task 6.

- [ ] **Step 1: Write exhaustive state and duplicate classification tests**

Add a table-driven validator test:

~~~go
func TestValidateBuyerIntentStatus(t *testing.T) {
	tests := []struct {
		name   string
		status string
		open   bool
		valid  bool
	}{
		{"new open", model.IntentNew, true, true},
		{"contacted open", model.IntentContacted, true, true},
		{"closed closed", model.IntentClosed, false, true},
		{"new closed", model.IntentNew, false, false},
		{"contacted closed", model.IntentContacted, false, false},
		{"closed open", model.IntentClosed, true, false},
		{"unknown open", "BOGUS", true, false},
		{"unknown closed", "BOGUS", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBuyerIntentStatus(tt.status, tt.open)
			if tt.valid && err != nil {
				t.Fatalf("valid state error = %v", err)
			}
			if !tt.valid && !errors.Is(err, common.ErrInternal) {
				t.Fatalf("invalid state error = %v", err)
			}
		})
	}
}
~~~

Add duplicate reconciliation subtests using an isolated final SQLite table:

~~~text
non-duplicate create error -> 20001
duplicated key plus valid NEW/open winner -> 10010
duplicated key plus valid CONTACTED/open winner -> 10010
duplicated key plus no open row -> 20001
duplicated key plus malformed open row -> 20001
duplicated key plus malformed closed-flag history -> 20001
duplicated key plus closed history only -> 20001
duplicated key plus multiple open rows after dropping the constraint -> 20001
duplicated key plus closed database/re-query failure -> 20001
~~~

Test `findOpenBuyerIntent` separately with zero rows, multiple valid closed
histories, multiple valid closed histories plus one valid open row, each of the
five illegal/unknown persisted combinations, and two open rows after dropping
the test-only partial index. It must select only `status,is_open`, validate
every row for that buyer/product, and reject more than one open row even if
runtime schema drift has removed the constraint.

- [ ] **Step 2: Extend API tests to expose the existing failures**

Give apiResp a non-JSON HTTPStatus field and set it from the recorder:

~~~go
type apiResp struct {
	HTTPStatus int                    `json:"-"`
	Code       int                    `json:"code"`
	Data       map[string]interface{} `json:"data"`
}

// after ServeHTTP and successful JSON decoding
resp.HTTPStatus = w.Code
~~~

Extend TestBuyerIntentCreateConflictAndMerchantStatusFlow so it:

~~~text
creates intent 1
runs the existing sequential conflict request while intent 1 is open and
  asserts HTTP 409/code 10010
closes intent 1
creates and closes intent 2
creates and closes intent 3
asserts exactly 3 rows, all CLOSED/false, for the buyer/product
creates intent 4 and leaves it open
asserts exactly 3 CLOSED/false rows and 1 NEW/true row at the end
~~~

Add TestBuyerIntentConcurrentCreateHasOneWinner with two goroutines released by
one start channel. Require exactly one HTTP 200/code 0 and one HTTP 409/code
10010, then query exactly one open row. Create this test's server from a
dedicated SQLite DSN ending in
&_pragma=busy_timeout(5000), set MaxOpenConns(4) and MaxIdleConns(2), and use no
Idempotency-Key so both requests reach the database constraint.

Add TestBuyerIntentStateDriftFailsClosed. Use separate fixtures for NEW/false,
CONTACTED/false, CLOSED/true, and BOGUS. Start each server against a verified
final schema first, then inject exactly one malformed synthetic row so the test
models runtime drift after startup rather than bypassing the startup verifier.
Exercise:

~~~text
buyer list and buyer detail
merchant list and merchant detail
merchant contacted and close transitions
buyer duplicate pre-check
~~~

Each affected call must return HTTP 500/code 20001 and leave the row plus
operation-log count unchanged.

- [ ] **Step 3: Run focused tests and confirm RED**

Run:

~~~bash
cd backend
go test ./internal/app -run 'TestValidateBuyerIntent|TestClassifyBuyerIntent' -count=1
go test ./tests -run 'TestBuyerIntent' -count=1
~~~

Expected: FAIL because the state helper is absent, malformed rows can return
success, and duplicate classification still scans error text.

- [ ] **Step 4: Implement the canonical runtime helpers**

Create buyer_intent_state.go:

~~~go
func validateBuyerIntentStatus(status string, isOpen bool) error {
	switch status {
	case model.IntentNew, model.IntentContacted:
		if isOpen {
			return nil
		}
	case model.IntentClosed:
		if !isOpen {
			return nil
		}
	}
	return common.ErrInternal
}

func validateBuyerIntentState(intent model.BuyerIntent) error {
	return validateBuyerIntentStatus(intent.Status, intent.IsOpen)
}

func findOpenBuyerIntent(
	db *gorm.DB,
	buyerID uint64,
	productID uint64,
) (bool, error) {
	var intents []model.BuyerIntent
	if err := db.Select("status", "is_open").Where(
		"buyer_id = ? AND product_id = ?",
		buyerID, productID,
	).Find(&intents).Error; err != nil {
		return false, common.ErrInternal
	}
	found := false
	for _, intent := range intents {
		if err := validateBuyerIntentState(intent); err != nil {
			return false, common.ErrInternal
		}
		if intent.IsOpen {
			if found {
				return false, common.ErrInternal
			}
			found = true
		}
	}
	return found, nil
}

func classifyBuyerIntentCreateError(
	db *gorm.DB,
	createErr error,
	buyerID uint64,
	productID uint64,
) error {
	if !errors.Is(createErr, gorm.ErrDuplicatedKey) {
		return common.ErrInternal
	}
	found, err := findOpenBuyerIntent(db, buyerID, productID)
	if err != nil || !found {
		return common.ErrInternal
	}
	return common.ErrConflict
}
~~~

- [ ] **Step 5: Apply helpers to create and read paths**

Replace the Count pre-check with:

~~~go
found, err := findOpenBuyerIntent(s.DB, actor.UserID, req.ProductID)
if err != nil {
	return nil, err
}
if found {
	return nil, common.ErrConflict
}
~~~

Replace the broad string match after Create with:

~~~go
if err := s.DB.Create(&intent).Error; err != nil {
	return nil, classifyBuyerIntentCreateError(
		s.DB, err, actor.UserID, req.ProductID,
	)
}
~~~

Delete only the now-unused broad unique-error logic; keep strings imports still
needed for request and query normalization.

For buyer list, validate every loaded model before product lookups. For buyer
detail, check BuyerID authorization first, then validate before returning data.

For loadOwnedIntent, check MerchantID authorization first, then validate. For
merchant list, add IsOpen bool with json:"-" to the local item, select
i.is_open, and validate every item before common.Success.

The contacted/close handlers must call the validator through loadOwnedIntent
before idempotency checks. Keep transition outputs unchanged.

- [ ] **Step 6: Run focused tests and inspect SQL write ownership**

Run:

~~~bash
cd backend
gofmt -w internal/app/buyer_intent_state.go internal/app/buyer_intent_state_test.go internal/app/buyer_handlers.go internal/app/merchant_intent_handlers.go tests/integration_flow_test.go tests/buyer_flow_test.go
go test ./internal/app -run 'TestValidateBuyerIntent|TestClassifyBuyerIntent' -count=1
go test ./tests -run 'TestBuyerIntent' -count=1
rg -n '"open_marker"|open_marker[[:space:]]*:' internal --glob '*.go'
~~~

Expected: focused tests PASS. The final search finds only schema inspection or
the read-only model field, never a create/update map or DTO.

- [ ] **Step 7: Commit runtime behavior**

~~~bash
git add backend/internal/app/buyer_intent_state.go backend/internal/app/buyer_intent_state_test.go backend/internal/app/buyer_handlers.go backend/internal/app/merchant_intent_handlers.go backend/tests/integration_flow_test.go backend/tests/buyer_flow_test.go
git commit -m "fix(buyer): validate intent lifecycle"
~~~

---

### Task 4: Add the formal resumable MySQL 0009 migration

**Files:**
- Create: backend/migrations/0009_buyer_intent_open_uniqueness.preflight.sql
- Create: backend/migrations/0009_buyer_intent_open_uniqueness.up.sql
- Create: backend/migrations/0009_buyer_intent_open_uniqueness.postflight.sql
- Create: backend/migrations/buyer_intent_open_uniqueness_migration_test.go

**Interfaces:**
- Consumes: the exact Task 2 MySQL schema contract.
- Produces: the final 0009 postflight shape required by Tasks 5, 7, and 9, and
  F-12 migration 0010.

- [ ] **Step 1: Write RED artifact tests**

Require these stable markers:

~~~text
buyer_intent_open_uniqueness_preflight_passed
buyer_intent_open_uniqueness_migration_applied
buyer_intent_open_uniqueness_postflight_passed
~~~

Require all files to mention buyer_intents, open_marker,
uk_buyer_product_open, uk_buyer_intent_open, ordered key columns,
generation_expression, is_generated, STORED GENERATED, status/is_open checks,
duplicate-open GROUP BY, and SIGNAL SQLSTATE '45000'.

Add a whole-file DML prohibition for preflight, up, and postflight. It must
catch statements even when a label or `IF ... THEN` appears earlier on the
line; do not rely on line starts:

~~~go
for _, name := range []string{
	"0009_buyer_intent_open_uniqueness.preflight.sql",
	"0009_buyer_intent_open_uniqueness.up.sql",
	"0009_buyer_intent_open_uniqueness.postflight.sql",
} {
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	forbidden := regexp.MustCompile(
		"(?is)\\b(?:INSERT\\s+INTO|UPDATE\\s+[`a-z0-9_]+|DELETE\\s+FROM|" +
			"REPLACE\\s+INTO|TRUNCATE(?:\\s+TABLE)?)\\b",
	)
	if match := forbidden.Find(raw); match != nil {
		t.Fatalf("%s contains business DML %q", name, match)
	}
}
~~~

Keep comments and `MESSAGE_TEXT` values free of those statement phrases so the
raw-file guard remains deterministic. Also reject `PREPARE`, `EXECUTE`,
`CREATE TABLE ... SELECT`, and `RENAME TABLE`; 0009 needs no dynamic SQL,
table copy, or hidden row-mutation path.

Assert filepath.Glob("0009*.down.sql") returns zero files.

- [ ] **Step 2: Run artifact tests and confirm RED**

Run:

~~~bash
cd backend
go test ./migrations -run 'TestBuyerIntentOpenUniquenessMigration' -count=1
~~~

Expected: FAIL because all 0009 artifacts are absent.

- [ ] **Step 3: Implement the read-only preflight procedure**

The procedure must first validate table/engine, baseline columns/indexes,
exact present F-11 definitions, row states, and duplicate-open groups. Require
exactly one `buyer_intents` table with `ENGINE=InnoDB`; non-null `id BIGINT`,
`intent_no VARCHAR(32)`, `buyer_id BIGINT`, `product_id BIGINT`,
`status VARCHAR(16)`, and `is_open TINYINT(1)` columns; `PRIMARY(id)`; and one
single-column unique key on `intent_no` regardless of its non-F-11 key name.
Reject missing, duplicated, nullable, reordered, or drifted baseline members
before evaluating a resume state. Use this formal-state decision:

~~~sql
IF v_marker = 0 AND v_legacy_exact = 1 AND v_open_key = 0 THEN
  SET v_state = 'legacy';
ELSEIF v_marker_exact = 1 AND v_legacy_exact = 1 AND v_open_key = 0 THEN
  SET v_state = 'marker_only';
ELSEIF v_marker_exact = 1 AND v_legacy_exact = 1 AND v_open_exact = 1 THEN
  SET v_state = 'both_keys';
ELSEIF v_marker_exact = 1 AND v_legacy_key = 0 AND v_open_exact = 1 THEN
  SET v_state = 'final';
ELSE
  SIGNAL SQLSTATE '45000'
    SET MESSAGE_TEXT = 'buyer intent preflight: schema is partial or drifted';
END IF;
~~~

Compute v_legacy_exact and v_open_exact by grouping
information_schema.statistics and requiring non_unique=0 plus exact ordered
GROUP_CONCAT(column_name ORDER BY seq_in_index). Reject relevant lookalike
unique keys under another name. The SQL definition of relevant must match the
application helper: excluding `PRIMARY` and the independent one-column
`intent_no` key, any unique index containing both `buyer_id` and `product_id`
is F-11-relevant and must be one of the exact named definitions allowed by the
current resume state.

Compute v_marker_exact from one information_schema.columns row with
data_type='tinyint', column_type='tinyint', is_nullable='YES',
is_generated='ALWAYS', and extra containing 'STORED GENERATED'. Normalize the
expression with nested LOWER/REPLACE calls that remove backticks, spaces,
CHAR(9), CHAR(10), CHAR(13), '(' and ')', then require exactly:

~~~text
casewhenis_open=1then1elsenullend
~~~

Run data checks before the formal-state decision so a duplicate-open fixture is
reported as unsafe data even when its old key was removed to create that
fixture:

~~~sql
SELECT COUNT(*) INTO v_invalid_rows
FROM buyer_intents
WHERE CASE
  WHEN status IN ('NEW', 'CONTACTED') AND is_open = 1 THEN 0
  WHEN status = 'CLOSED' AND is_open = 0 THEN 0
  ELSE 1
END = 1;

SELECT COUNT(*) INTO v_duplicate_groups
FROM (
  SELECT buyer_id, product_id
  FROM buyer_intents
  WHERE is_open = 1
  GROUP BY buyer_id, product_id
  HAVING COUNT(*) > 1
) AS duplicate_open_intents;
~~~

Signal 45000 when either count is nonzero. Emit the preflight marker only after
all checks pass.

- [ ] **Step 4: Implement resumable up in the approved order**

Repeat all preflight data and state checks in the up procedure, then:

~~~sql
IF v_marker = 0 THEN
  ALTER TABLE buyer_intents
    ADD COLUMN open_marker TINYINT
      GENERATED ALWAYS AS (
        CASE WHEN is_open = 1 THEN 1 ELSE NULL END
      ) STORED AFTER is_open;
END IF;
~~~

Re-read information_schema.columns and require the exact marker before:

~~~sql
IF v_open_key = 0 THEN
  ALTER TABLE buyer_intents
    ADD UNIQUE KEY uk_buyer_intent_open
      (buyer_id, product_id, open_marker);
END IF;
~~~

Re-read statistics and require the exact new unique key before:

~~~sql
IF v_legacy_key = 1 THEN
  ALTER TABLE buyer_intents
    DROP INDEX uk_buyer_product_open;
END IF;
~~~

Re-read the final column/key state, require no legacy/lookalike key, and only
then emit buyer_intent_open_uniqueness_migration_applied. Do not use dynamic
row repair or a table copy.

- [ ] **Step 5: Implement exact read-only postflight**

Postflight requires:

~~~text
one InnoDB buyer_intents table
one exact stored generated open_marker
one exact uk_buyer_intent_open(buyer_id,product_id,open_marker)
zero uk_buyer_product_open
zero relevant lookalike unique keys
zero invalid state rows
zero duplicate open groups
~~~

It emits only buyer_intent_open_uniqueness_postflight_passed on success.

- [ ] **Step 6: Run migration contracts**

Run:

~~~bash
cd backend
gofmt -w migrations/buyer_intent_open_uniqueness_migration_test.go
go test ./migrations -run 'TestBuyerIntentOpenUniquenessMigration' -count=1
go test ./migrations -count=1
~~~

Expected: PASS, with no 0009 down file and no prohibited DML.

- [ ] **Step 7: Commit formal migration artifacts**

~~~bash
git add backend/migrations/0009_buyer_intent_open_uniqueness.preflight.sql backend/migrations/0009_buyer_intent_open_uniqueness.up.sql backend/migrations/0009_buyer_intent_open_uniqueness.postflight.sql backend/migrations/buyer_intent_open_uniqueness_migration_test.go
git commit -m "fix(migrations): add buyer intent open key"
~~~

---

### Task 5: Advance every existing migration-chain consumer through 0009

**Files:**
- Modify: deploy/acceptance/file-record-schema-smoke.sh
- Modify: deploy/acceptance/file-binding-authorization-smoke.sh
- Modify: deploy/acceptance/license-file-privacy-smoke.sh
- Modify: deploy/acceptance/anonymous-upload-governance-smoke.sh
- Modify: deploy/acceptance/session-revocation-smoke.sh
- Modify: backend/migrations/file_records_migration_test.go
- Modify: backend/migrations/file_binding_migration_test.go
- Modify: backend/migrations/license_file_privacy_migration_test.go
- Modify: backend/migrations/anonymous_upload_governance_migration_test.go
- Modify: backend/tests/session_revocation_acceptance_contract_test.go
- Modify: deploy/acceptance/README.md

**Interfaces:**
- Consumes: Task 4 formal 0009 gates and the Task 2
  AUTO_MIGRATE=false final-schema requirement.
- Produces: all previously accepted migration-only API harnesses remain
  replayable against the current schema; their original dirty-state matrices
  remain scoped to their own migrations.

- [ ] **Step 1: Add RED compatibility contracts**

Add or extend these exact contract tests and require all three current-chain
files:

~~~text
TestFileSchemaSmokeUsesCurrentMigrationChain
TestFileBindingAcceptanceUsesCurrentMigrationChain
TestLicenseFilePrivacyAcceptanceUsesCurrentMigrationChain
TestAnonymousUploadGovernanceAcceptanceUsesCurrentMigrationChain
TestSessionRevocationAcceptanceUsesCurrentMigrationChain

0009_buyer_intent_open_uniqueness.preflight.sql
0009_buyer_intent_open_uniqueness.up.sql
0009_buyer_intent_open_uniqueness.postflight.sql
~~~

Also require that the 0009 postflight runs before the first
AUTO_MIGRATE=false focused API test. Do not change the expected guards,
dedicated Compose project names, or original failure fixtures for F-09, F-02,
F-13, F-06, or F-14.

- [ ] **Step 2: Run the compatibility contracts and confirm RED**

Run:

~~~bash
cd backend
go test ./migrations -run 'Test(FileSchemaSmoke|FileBindingAcceptance|LicenseFilePrivacyAcceptance|AnonymousUploadGovernanceAcceptance)UsesCurrentMigrationChain' -count=1
go test ./tests -run '^TestSessionRevocationAcceptanceUsesCurrentMigrationChain$' -count=1
~~~

Expected: FAIL because the five scripts currently stop at migration 0005,
0006, 0007, or 0008 before starting NewServer.

- [ ] **Step 3: Append only the required clean-chain suffixes**

Preserve every feature-specific dirty fixture and gate. Add later migrations
only after that script has completed its own clean migration and immediately
before it starts a migration-only API server:

~~~text
file-record-schema-smoke.sh:
  after clean 0005, run 0006, 0007, 0008, then 0009

file-binding-authorization-smoke.sh:
  after clean 0006, run 0007, 0008, then 0009

license-file-privacy-smoke.sh:
  after clean 0007, run 0008, then 0009

anonymous-upload-governance-smoke.sh:
  after clean 0008, run 0009

session-revocation-smoke.sh:
  extend apply_migration_chain from 0001..0008 to 0001..0009
~~~

Use the same exact triplet everywhere:

~~~bash
mysql_file /acceptance/migrations/0009_buyer_intent_open_uniqueness.preflight.sql
mysql_file /acceptance/migrations/0009_buyer_intent_open_uniqueness.up.sql
mysql_file /acceptance/migrations/0009_buyer_intent_open_uniqueness.postflight.sql
~~~

For suffixes beginning before 0008, execute every intervening migration's
preflight/up/postflight in numeric order. Do not run 0009 inside an earlier
migration's intentional rejection fixture; those fixtures must still prove
their original failure before later schema is applied.

- [ ] **Step 4: Update replay documentation**

In each existing acceptance README section, change only the clean API replay
chain to say it applies the current chain through 0009. Preserve the historical
finding's own migration number, evidence, project, authorization, and
production status.

- [ ] **Step 5: Run all script and contract checks**

Run:

~~~bash
bash -n deploy/acceptance/file-record-schema-smoke.sh
bash -n deploy/acceptance/file-binding-authorization-smoke.sh
bash -n deploy/acceptance/license-file-privacy-smoke.sh
bash -n deploy/acceptance/anonymous-upload-governance-smoke.sh
bash -n deploy/acceptance/session-revocation-smoke.sh
cd backend
go test ./migrations -run 'Test(FileSchemaSmoke|FileBindingAcceptance|LicenseFilePrivacyAcceptance|AnonymousUploadGovernanceAcceptance)UsesCurrentMigrationChain' -count=1
go test ./tests -run '^TestSessionRevocationAcceptanceUsesCurrentMigrationChain$' -count=1
~~~

Expected: PASS. This is a static/local compatibility gate; do not rerun any
older remote Compose project under its previous authorization.

- [ ] **Step 6: Commit current-chain compatibility**

~~~bash
git add deploy/acceptance/file-record-schema-smoke.sh deploy/acceptance/file-binding-authorization-smoke.sh deploy/acceptance/license-file-privacy-smoke.sh deploy/acceptance/anonymous-upload-governance-smoke.sh deploy/acceptance/session-revocation-smoke.sh backend/migrations/file_records_migration_test.go backend/migrations/file_binding_migration_test.go backend/migrations/license_file_privacy_migration_test.go backend/migrations/anonymous_upload_governance_migration_test.go backend/tests/session_revocation_acceptance_contract_test.go deploy/acceptance/README.md
git commit -m "test(acceptance): advance schema chain to 0009"
~~~

---

### Task 6: Add opt-in MySQL schema and API acceptance

**Files:**
- Create: backend/tests/buyer_intent_mysql_test.go

**Interfaces:**
- Consumes: final 0009 schema, Task 2 startup verifier, and Task 3 API behavior.
- Produces: TestBuyerIntentMySQLAcceptance for the Task 7 harness.

- [ ] **Step 1: Add the fail-closed environment and DSN guard**

Start the test exactly this way:

~~~go
func TestBuyerIntentMySQLAcceptance(t *testing.T) {
	if os.Getenv("BUYER_INTENT_MYSQL_TEST") != "1" {
		t.Skip("set BUYER_INTENT_MYSQL_TEST=1 only in the isolated buyer intent project")
	}
	dsn := strings.TrimSpace(os.Getenv("DB_DSN"))
	parsed, err := mysqlcfg.ParseDSN(dsn)
	if err != nil ||
		parsed.Net != "tcp" ||
		parsed.Addr != "mysql:3306" ||
		parsed.DBName != "second_hand_market_acceptance" {
		t.Fatal("DB_DSN must target isolated mysql:3306/second_hand_market_acceptance")
	}
	cfg := app.Config{
		AppEnv:                   "test",
		Addr:                     ":0",
		DBDriver:                 "mysql",
		DBDSN:                    dsn,
		JWTAccessSecret:          "buyer-intent-test-access",
		JWTRefreshSecret:         "buyer-intent-test-refresh",
		AccessTTL:                time.Hour,
		RefreshTTL:               24 * time.Hour,
		AutoMigrate:              strings.EqualFold(os.Getenv("AUTO_MIGRATE"), "true"),
		FileStorageProvider:      "local",
		FileUploadLocalDir:       t.TempDir(),
		ImageCompressTargetBytes: 10 * 1024 * 1024,
		ImageProcessorDriver:     "passthrough",
		BuyerWechatLoginMode:     "mock",
		BuyerDouyinLoginMode:     "mock",
		BuyerWechatHTTPTimeout:   5 * time.Second,
		BuyerDouyinHTTPTimeout:   5 * time.Second,
	}
	configureTestUploadGovernance(&cfg)
	srv, err := app.NewServer(cfg)
	if err != nil {
		t.Fatal("start isolated buyer intent server")
	}
	sqlDB, err := srv.DB.DB()
	if err != nil {
		t.Fatal("open isolated buyer intent pool")
	}
	sqlDB.SetMaxOpenConns(8)
	sqlDB.SetMaxIdleConns(4)
	t.Cleanup(func() { _ = sqlDB.Close() })
}
~~~

- [ ] **Step 2: Assert exact schema before business behavior**

Query information_schema and require:

~~~text
open_marker: tinyint, nullable YES, generated ALWAYS, STORED GENERATED
normalized expression: casewhenis_open=1then1elsenullend
uk_buyer_intent_open: non_unique=0, buyer_id,product_id,open_marker
uk_buyer_product_open count: 0
relevant lookalike key count: 0
~~~

Also use a dry-run GORM Create and assert the generated INSERT SQL does not
contain open_marker.

- [ ] **Step 3: Exercise three histories and concurrent creation through Gin**

Create synthetic merchant/account/product and buyer/session fixtures. Use only
test-only contact values. Through the real router:

~~~text
cycle 1: buyer create -> merchant contacted -> merchant close
cycle 2: buyer create -> merchant close
cycle 3: buyer create -> merchant close
assert three unchanged CLOSED/false histories
launch two buyer creates together for the same buyer/product
assert one HTTP 200/code 0 and one HTTP 409/code 10010
assert exactly one NEW/true winner and three CLOSED/false histories
with a second buyer, create once for the original product and once for a
  second product; both succeed and prove buyer/product independence without
  exceeding the original buyer's five-per-minute rate limit
~~~

Log only sanitized status/code and aggregate counts:

~~~go
t.Logf("concurrent create status/codes = %v", statusCodes)
t.Logf("intent history/open counts = %d/%d", closedCount, openCount)
~~~

Never log access tokens, buyer IDs, merchant IDs, contact values, intent
numbers, DSN, or row contents.

- [ ] **Step 4: Prove MySQL runtime drift fails closed**

For a separate synthetic row, use direct isolated SQL to create CLOSED/true and
BOGUS/false states one at a time. Assert buyer/merchant read and transition
requests return HTTP 500/code 20001 and the row digest plus operation-log count
remain unchanged. Restore/delete only synthetic isolated fixtures inside the
test cleanup.

- [ ] **Step 5: Compile locally and record the intentional skip**

Run:

~~~bash
cd backend
gofmt -w tests/buyer_intent_mysql_test.go
go test ./tests -run '^TestBuyerIntentMySQLAcceptance$' -count=1 -v
~~~

Expected locally without the opt-in variable: PASS with an explicit SKIP. This
is compile/guard evidence only; Task 9 must run both AUTO_MIGRATE modes on
MySQL 8.4.

- [ ] **Step 6: Commit the opt-in test**

~~~bash
git add backend/tests/buyer_intent_mysql_test.go
git commit -m "test(buyer): add MySQL intent acceptance"
~~~

---

### Task 7: Build the guarded isolated MySQL 8.4 matrix

**Files:**
- Create: deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh
- Modify: backend/migrations/buyer_intent_open_uniqueness_migration_test.go
- Modify: deploy/acceptance/README.md
- Modify: Makefile

**Interfaces:**
- Consumes: all 0001..0009 migrations and
  TestBuyerIntentMySQLAcceptance.
- Produces: acceptance-buyer-intent-smoke and sanitized retained evidence for
  Task 9.

- [ ] **Step 1: Write RED static harness contracts**

Extend the migration contract test to require:

~~~text
BUYER_INTENT_ACCEPTANCE_CONFIRM
I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_BUYER_INTENT_DATA
BUYER_INTENT_SOURCE_LIST_ONLY
secondhand-buyer-intent-acceptance
evidence/buyer-intent-open-uniqueness
ACCEPTANCE_DB_ENGINE=mysql8.4
MySQL 8.4 version check
0009 preflight/up/postflight
legacy, marker-only, both-key, and final-rerun labels
invalid-state, duplicate-open, drifted-marker, drifted-key labels
ERROR 1644 (45000)
before/after row-summary comparisons
BUYER_INTENT_MYSQL_TEST=1
AUTO_MIGRATE=false and AUTO_MIGRATE=true
go test ./..., go test -race ./..., and go vet ./...
source-sha256.txt and evidence-sha256.txt
production-before.txt and production-after.txt
resource retention marker
~~~

Require the Makefile target and the exact confirmation value.

Add executable-order tests beside the static contract. Stub `docker` with a
script that writes `$DOCKER_CALLED`, then require every unsafe case to exit
nonzero without creating that marker:

~~~go
const buyerIntentAcceptanceConfirmation =
	"I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_BUYER_INTENT_DATA"

func TestBuyerIntentOpenUniquenessAcceptanceRejectsUnsafeEnvironmentBeforeDocker(t *testing.T) {
	script := "../../deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh"
	stubDir := t.TempDir()
	dockerCalled := filepath.Join(stubDir, "docker-called")
	dockerStub := filepath.Join(stubDir, "docker")
	stub := "#!/bin/sh\n: >\"$DOCKER_CALLED\"\nexit 99\n"
	if err := os.WriteFile(dockerStub, []byte(stub), 0o700); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, confirm, engine, project string
	}{
		{name: "missing confirmation", engine: "mysql8.4"},
		{name: "wrong confirmation", confirm: "unsafe", engine: "mysql8.4"},
		{name: "wrong engine", confirm: buyerIntentAcceptanceConfirmation, engine: "mysql8.0"},
		{name: "wrong project", confirm: buyerIntentAcceptanceConfirmation, engine: "mysql8.4", project: "secondhand-market"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.Remove(dockerCalled); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
			cmd := exec.Command("/bin/bash", script)
			cmd.Env = []string{
				"PATH=" + stubDir + ":/usr/bin:/bin",
				"DOCKER_CALLED=" + dockerCalled,
				"BUYER_INTENT_ACCEPTANCE_CONFIRM=" + tc.confirm,
				"ACCEPTANCE_DB_ENGINE=" + tc.engine,
				"COMPOSE_PROJECT_NAME=" + tc.project,
			}
			if err := cmd.Run(); err == nil {
				t.Fatal("unsafe acceptance environment succeeded")
			}
			if _, err := os.Stat(dockerCalled); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("unsafe environment reached Docker")
			}
		})
	}
}

func TestBuyerIntentOpenUniquenessAcceptanceManifestModeIsReadOnly(t *testing.T) {
	script := "../../deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh"
	stubDir := t.TempDir()
	dockerCalled := filepath.Join(stubDir, "docker-called")
	dockerStub := filepath.Join(stubDir, "docker")
	if err := os.WriteFile(dockerStub, []byte(
		"#!/bin/sh\n: >\"$DOCKER_CALLED\"\nexit 99\n",
	), 0o700); err != nil {
		t.Fatal(err)
	}
	sha256sum, err := exec.LookPath("sha256sum")
	if err != nil {
		t.Fatal("sha256sum is required for the manifest contract")
	}
	utilityPath := stubDir + ":" + filepath.Dir(sha256sum) +
		":/usr/bin:/bin:/usr/sbin:/sbin"
	cmd := exec.Command("/bin/bash", script)
	cmd.Env = []string{
		"PATH=" + utilityPath,
		"DOCKER_CALLED=" + dockerCalled,
		"BUYER_INTENT_SOURCE_MANIFEST_ONLY=1",
	}
	raw, err := cmd.Output()
	if err != nil {
		t.Fatalf("manifest-only mode: %v", err)
	}
	if _, err := os.Stat(dockerCalled); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("manifest-only mode reached Docker")
	}
	text := string(raw)
	for _, required := range []string{
		"  Makefile\n",
		"  backend/internal/app/server.go\n",
		"  backend/migrations/0009_buyer_intent_open_uniqueness.up.sql\n",
		"  deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh\n",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("manifest missing %q", required)
		}
	}
	for _, forbidden := range []string{
		".env", "secrets/", "evidence/", "uploads/", "app.db", ".tmp/",
		"architecture-evolution-plan-2026-07-24.md",
		"first-round-fix-review-2026-07-24.md",
		"second-round-fix-review-2026-07-24.md",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("manifest exposed forbidden path %q", forbidden)
		}
	}
	var paths []string
	linePattern := regexp.MustCompile(`^[0-9a-f]{64}  (.+)$`)
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		match := linePattern.FindStringSubmatch(line)
		if match == nil {
			t.Fatalf("invalid manifest line %q", line)
		}
		paths = append(paths, match[1])
	}
	want := append([]string(nil), paths...)
	sort.Strings(want)
	if !slices.Equal(paths, want) {
		t.Fatal("source manifest paths are not sorted")
	}
	listCmd := exec.Command("/bin/bash", script)
	listCmd.Env = []string{
		"PATH=" + utilityPath,
		"DOCKER_CALLED=" + dockerCalled,
		"BUYER_INTENT_SOURCE_LIST_ONLY=1",
	}
	listRaw, err := listCmd.Output()
	if err != nil {
		t.Fatalf("source-list mode: %v", err)
	}
	listText := strings.TrimSuffix(string(listRaw), "\x00")
	listPaths := strings.Split(listText, "\x00")
	if !slices.Equal(listPaths, paths) {
		t.Fatal("transfer list and hash manifest select different paths")
	}
	if _, err := os.Stat(dockerCalled); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("source-list mode reached Docker")
	}
}
~~~

Define `buyerIntentAcceptanceConfirmation` in the test as the full approved
literal. Add the shown `errors`, `os`, `os/exec`, `path/filepath`, `regexp`,
`slices`, `sort`, and `strings` imports. Use `os/exec`, not a shell command
string, so test input cannot alter the command line.

- [ ] **Step 2: Run the static test and confirm RED**

Run:

~~~bash
cd backend
go test ./migrations -run 'TestBuyerIntentOpenUniquenessAcceptance' -count=1
~~~

Expected: FAIL because the script, target, and README contract are absent.

- [ ] **Step 3: Add fail-closed script guards and source isolation**

The script header must fix:

~~~bash
set -euo pipefail

project_name="secondhand-buyer-intent-acceptance"
evidence_dir="$base_dir/evidence/buyer-intent-open-uniqueness"
compose=(docker compose --project-name "$project_name" --env-file "$base_dir/.env" --file "$base_dir/docker-compose.yml")
production_containers=(
  secondhand-market-api
  secondhand-market-web
  secondhand-market-mysql
)
~~~

Create the script with mode `0755`, and have the static contract assert it is a
regular executable file.

After the read-only source-list/manifest-mode early exits, require exact confirmation,
ACCEPTANCE_DB_ENGINE=mysql8.4, an absent or exact COMPOSE_PROJECT_NAME,
remote-generated .env, required commands, zero existing project containers/
volumes/networks, and an absent evidence directory. Task 9 separately proves
the exact authorized remote directory before calling the script.

Create a temporary Docker build context from only:

~~~text
Makefile
backend/Dockerfile
backend/go.mod
backend/go.sum
backend/**/*.go excluding .cache
backend/migrations/*.sql
non-sensitive deploy/acceptance scripts/yaml/conf/md/sql
~~~

Exclude .env, secrets, databases, app.db, uploads, evidence, backups, .git,
caches, node_modules, .tmp, and protected review documents. Hash the same sorted
NUL-delimited list into source-sha256.txt.

Define `write_source_file_list` and `write_source_manifest` before any guard
that calls them, using this one source-of-truth list:

~~~bash
write_source_file_list() {
  (
    cd "$repo_dir"
    {
      printf '%s\0' Makefile backend/Dockerfile backend/go.mod backend/go.sum
      find backend -type f -name '*.go' \
        ! -path '*/.cache/*' ! -path '*/uploads/*' ! -name 'app.db' -print0
      find backend/migrations -maxdepth 1 -type f -name '*.sql' -print0
      find deploy/acceptance -maxdepth 2 -type f \
        \( -name '*.sh' -o -name '*.yml' -o -name '*.yaml' \
          -o -name '*.conf' -o -name '*.md' -o -name '*.Dockerfile' \
          -o -path 'deploy/acceptance/sql/*.sql' \) \
        ! -name '.env' ! -name '.env.*' ! -path '*/secrets/*' \
        ! -path '*/backups/*' ! -path '*/evidence/*' -print0
    } | LC_ALL=C sort -zu
  )
}

write_source_manifest() {
  local output="$1"
  write_source_file_list | (
    cd "$repo_dir"
    xargs -0 sha256sum
  ) >"$output"
}
~~~

This list contains no config examples, compiled artifacts, uploaded media, or
files selected merely because they happen to exist below `backend/`. Add
read-only source-list and manifest modes before any .env, confirmation,
evidence-directory creation, production snapshot, or Docker check:

~~~bash
if [[ "${BUYER_INTENT_SOURCE_LIST_ONLY:-0}" == "1" &&
      "${BUYER_INTENT_SOURCE_MANIFEST_ONLY:-0}" == "1" ]]; then
  echo "choose one read-only source mode" >&2
  exit 1
fi
if [[ "${BUYER_INTENT_SOURCE_LIST_ONLY:-0}" == "1" ]]; then
  write_source_file_list
  exit 0
fi
if [[ "${BUYER_INTENT_SOURCE_MANIFEST_ONLY:-0}" == "1" ]]; then
  write_source_manifest /dev/stdout
  exit 0
fi
~~~

Both conditions use `${...:-0}` so unset variables are safe under `set -u`.
`write_source_manifest` accepts exactly one output path and uses the same
whitelist for read-only modes, retained evidence, exact Task 9 transfer, and
the temporary Docker context.

Stage that exact NUL-delimited file list into
`$runtime_dir/build-context` with `tar --null --files-from`, preserving paths.
Create `$runtime_dir/buyer-intent-compose.yml` and append it to `compose` so
only the test image uses the staged context. Run tests from a read-only mount
of that same context at repository layout, because migration contract tests
resolve `../../deploy/acceptance` and `../../Makefile` from package working
directories:

~~~bash
compose_override="$runtime_dir/buyer-intent-compose.yml"
printf 'services:\n  bootstrap-admin:\n    build:\n      context: "%s"\n      dockerfile: backend/Dockerfile\n    working_dir: /workspace/backend\n    volumes:\n      - "%s:/workspace:ro"\n' \
  "$build_context" "$build_context" >"$compose_override"
compose+=(--file "$compose_override")
~~~

The MySQL migration bind mount and remote-only secret bind remain sourced from
the original acceptance directory and merge with the source mount. Assert via
`docker compose config --no-interpolate --format json` plus `jq -e` that
`/workspace` is read-only, its source equals the mktemp build context, and the
effective working directory is `/workspace/backend`. Save that structural JSON
only under `$runtime_dir`, never print or retain the full Compose configuration,
and fail if an interpolated credential value appears. Do not build or run
`bootstrap-admin` directly from the transferred repository tree after `.env`,
secrets, or evidence have been created.

Use a trap that stops only this Compose project's services, deletes only the
mktemp runtime directory, and retains project resources and evidence.

- [ ] **Step 4: Implement migration-state and rejection fixtures**

Add functions with these exact responsibilities:

~~~bash
reset_schema                 # drop only isolated acceptance tables
apply_chain_0001_0008        # execute every approved gate in order
run_0009                     # preflight -> up -> postflight
capture_intent_summary       # count and SHA-256 aggregate of synthetic rows
require_45000_unchanged      # run expected failure, grep 1644/45000, cmp summary
~~~

`capture_intent_summary` must emit only `row_count=<n>` and one SHA-256 digest.
Build the digest from primary-key-ordered per-row `JSON_ARRAY` values covering
`id,intent_no,buyer_id,source_device_id,product_id,merchant_id,status,is_open,`
`contact_name,contact_phone,contact_wechat,message,handled_by,handled_at,`
`closed_at,close_reason,merchant_note,created_at,updated_at`, excluding only
derived `open_marker`; hash each row before the ordered aggregate so no row
value appears in evidence. Set a
session-local `group_concat_max_len` large enough for these small synthetic
fixtures and fail when the digest query is empty or malformed. Never retain the
source JSON, SQL result rows, IDs, intent numbers, or contacts.

Run these independent resets:

~~~text
legacy:
  apply 0001..0008; seed one NEW/open and one CLOSED/false on distinct keys;
  capture; run 0009; compare; require final shape

marker-only:
  apply 0001..0008; add exact open_marker; capture; run 0009; compare

both-keys:
  apply 0001..0008; add marker and exact new key; capture; run 0009; compare

final-rerun:
  apply 0001..0008; run 0009 twice; require both passes and unchanged rows

invalid-state:
  for NEW/false, CONTACTED/false, CLOSED/true, BOGUS/false, and BOGUS/true,
  independently apply 0001..0008 and seed exactly that one fixture;
  each preflight must be 45000 and its before/after summary identical

duplicate-open:
  apply 0001..0008; drop legacy key; seed two open rows for one key;
  preflight must be 45000 and summaries identical

drifted-marker:
  apply 0001..0008; add ordinary nullable TINYINT open_marker;
  preflight must be 45000 and summaries identical

drifted-key:
  apply 0001..0008; add exact marker and wrong-order new unique key;
  preflight must be 45000 and summaries identical

unknown-partial:
  apply 0001..0008; add marker then drop legacy without adding new key;
  preflight must be 45000 and summaries identical
~~~

The script must never run a rejection fixture against production or a shared
schema.

- [ ] **Step 5: Run API/startup and quality matrices**

After a clean 0001..0009 chain, build the test image and run:

~~~bash
"${compose[@]}" --profile tools run --rm -e BUYER_INTENT_MYSQL_TEST=1 -e AUTO_MIGRATE=false bootstrap-admin go test ./tests -run '^TestBuyerIntentMySQLAcceptance$' -count=1 -v

"${compose[@]}" --profile tools run --rm -e BUYER_INTENT_MYSQL_TEST=1 -e AUTO_MIGRATE=true bootstrap-admin go test ./tests -run '^TestBuyerIntentMySQLAcceptance$' -count=1 -v
~~~

Reset/apply the full chain before each mode. Save only PASS markers,
status/codes, aggregate counts, and schema summaries.

Then run with BUYER_INTENT_MYSQL_TEST=0:

~~~bash
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
~~~

Store package PASS summaries and categorical pass markers, not raw request
payloads.

- [ ] **Step 6: Add production snapshot and evidence leak gates**

Before any isolated project startup and after all tests, capture only:

~~~text
container name | container ID | state | restart count
~~~

for the three fixed production container names. Require byte-identical
snapshots. Never inspect production environment variables, mounts, logs,
networks, database, or uploads.

Reject evidence containing:

~~~text
Authorization
access_token or refresh_token
DB_DSN, MYSQL_PASSWORD, MYSQL_ROOT_PASSWORD
JWT_ACCESS_SECRET or JWT_REFRESH_SECRET
openid followed by a value delimiter
buyer_id, merchant_id, actor_id, or session_id followed by a numeric value
intent_no followed by a value delimiter
contact_phone, contact_wechat, or contact_name followed by a value delimiter
JWT-shaped eyJ tokens
~~~

The scan must allow schema column names such as buyer_id and open_marker when
they have no row value; otherwise the required index-shape evidence would be
impossible to retain. Use a value-sensitive extended expression:

~~~text
Authorization|access_token|refresh_token|DB_DSN=|MYSQL_PASSWORD=|
MYSQL_ROOT_PASSWORD=|JWT_ACCESS_SECRET=|JWT_REFRESH_SECRET=|
openid["=:]|buyer_id["=:][[:space:]]*[0-9]|
merchant_id["=:][[:space:]]*[0-9]|actor_id["=:][[:space:]]*[0-9]|
session_id["=:][[:space:]]*[0-9]|intent_no["=:]|
contact_(phone|wechat|name)["=:]|eyJ[A-Za-z0-9_-]+\.
~~~

Keep it as one shell regex line in the script. Write evidence-sha256.txt from
all other retained text files.

- [ ] **Step 7: Add the Makefile and README contracts**

Add:

~~~make
acceptance-buyer-intent-smoke:
	@test "$$BUYER_INTENT_ACCEPTANCE_CONFIRM" = "I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_BUYER_INTENT_DATA" || { echo "set BUYER_INTENT_ACCEPTANCE_CONFIRM for isolated buyer intent tests" >&2; exit 1; }
	@test "$$ACCEPTANCE_DB_ENGINE" = "mysql8.4" || { echo "set ACCEPTANCE_DB_ENGINE=mysql8.4" >&2; exit 1; }
	./deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh
~~~

Add the target to .PHONY.

Document the exact remote path, project, command, transfer exclusions, evidence
path, read-only production snapshot scope, retained resources, and the fact that
passing does not execute production 0009.

- [ ] **Step 8: Run local harness checks**

Run:

~~~bash
bash -n deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh
cd backend
go test ./migrations -run 'TestBuyerIntentOpenUniqueness' -count=1
cd ..
git diff --check
~~~

Expected: PASS. Do not run Docker or SSH in this task.

- [ ] **Step 9: Commit the acceptance harness**

~~~bash
git add Makefile deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh deploy/acceptance/README.md backend/migrations/buyer_intent_open_uniqueness_migration_test.go
git commit -m "test(acceptance): add buyer intent matrix"
~~~

---

### Task 8: Complete local gates, independent review, and code-side status

**Files:**
- Modify: docs/miniapp-buyer-data-model.md:139-205
- Modify: docs/full-project-code-review-2026-07-24.md:54,304-324,489
- Modify: docs/release-readiness.md:5-59
- Modify: docs/superpowers/specs/2026-07-27-buyer-intent-open-uniqueness-design.md

**Interfaces:**
- Consumes: Tasks 1-7 committed code and local tests.
- Produces: a reviewed code-side F-11 commit range whose status still says
  isolated test-server review pending.

- [ ] **Step 1: Run focused verification from a clean index**

Run:

~~~bash
git status --short --untracked-files=no
cd backend
go test ./internal/app -run 'Test.*BuyerIntent' -count=1
go test ./tests -run 'TestBuyerIntent' -count=1
go test ./migrations -run 'TestBuyerIntentOpenUniqueness' -count=1
cd ..
bash -n deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh
~~~

Expected: all commands PASS and no tracked change is left unstaged. Do not
enumerate or inspect unrelated protected untracked paths.

- [ ] **Step 2: Run full backend quality gates**

Use repository-local caches:

~~~bash
cd backend
mkdir -p .cache/go/mod .cache/go/build
env GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" go test ./... -count=1
env GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" go test -race ./... -count=1
env GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" go vet ./...
cd ..
git diff --check
~~~

Expected: PASS. The opt-in MySQL test may SKIP; do not call that server
acceptance.

- [ ] **Step 3: Request independent code and spec review**

Invoke `superpowers:requesting-code-review` and give the independent reviewer:

~~~text
Scope: F-11 commits after 4e8ea92 through current HEAD
Spec: docs/superpowers/specs/2026-07-27-buyer-intent-open-uniqueness-design.md
Review priorities:
  MySQL/SQLite/GORM final-contract agreement
  DDL interruption recovery
  no migration business DML/down script
  duplicate-key re-query semantics
  state-drift handling on every read/transition
  AUTO_MIGRATE=false no-DDL behavior
  concurrency authority and test strength
  acceptance isolation/evidence leakage
  protected/production scope
Output: Critical, Important, Minor findings with file:line evidence
~~~

Resolve every finding through RED -> GREEN tests and rerun the affected focused
plus full gates. Invoke `superpowers:receiving-code-review` before applying
review feedback. Do not dismiss a finding solely because existing tests pass.

- [ ] **Step 4: Update canonical data-model documentation**

Replace the obsolete index statement with both dialect contracts:

~~~text
MySQL 8.4:
  stored generated open_marker =
    CASE WHEN is_open = 1 THEN 1 ELSE NULL END
  uk_buyer_intent_open(buyer_id,product_id,open_marker)

SQLite development/test:
  unique (buyer_id,product_id) WHERE is_open = 1

Application code never writes open_marker. Any number of CLOSED histories are
retained; only one NEW/CONTACTED open row may exist.
~~~

- [ ] **Step 5: Record code-side status without overstating server review**

Append follow-up text; preserve historical finding evidence. Use exactly:

~~~text
F-11 code-side fixed; isolated MySQL 8.4 test-server review pending;
production 0009 not executed.
~~~

In release-readiness, add an F-11 row with:

~~~text
修复状态: 代码侧已修复；生成列/部分索引、状态校验、重复键复查和三轮关闭已通过本地门禁
测试服务器审核: 未审核；专用 Compose 项目尚未获本问题的精确授权运行
生产状态: 未执行 0009、未部署、未修改生产数据
~~~

Advance the design status to:

~~~text
Design and written specification approved; code-side fixed; isolated
test-server review pending; production 0009 not executed
~~~

Record exact commit range and verification commands/results. Do not edit the
three protected review documents.

- [ ] **Step 6: Commit code-side documentation**

~~~bash
git add docs/miniapp-buyer-data-model.md docs/full-project-code-review-2026-07-24.md docs/release-readiness.md docs/superpowers/specs/2026-07-27-buyer-intent-open-uniqueness-design.md
git commit -m "docs(buyer): record F-11 code closure"
~~~

- [ ] **Step 7: Re-run post-commit proof**

Run:

~~~bash
git show --check --stat HEAD
git status --short --branch --untracked-files=no
git log --oneline 4e8ea92..HEAD
~~~

Expected: the commit range is reviewable and the tracked worktree is clean.
Keep F-11 test-server status pending and do not enumerate protected untracked
paths.

---

### Task 9: Obtain exact authorization and run isolated MySQL 8.4 acceptance

**Files:**
- Create after passing evidence:
  docs/superpowers/reviews/2026-07-27-buyer-intent-open-uniqueness-isolated-acceptance.md
- Modify after passing evidence: docs/full-project-code-review-2026-07-24.md
- Modify after passing evidence: docs/release-readiness.md
- Modify after passing evidence:
  docs/superpowers/specs/2026-07-27-buyer-intent-open-uniqueness-design.md

**Interfaces:**
- Consumes: the exact reviewed Task 8 commit range and Task 7 harness.
- Produces: accepted F-11 MySQL evidence and the prerequisite commit range for
  F-12.

- [ ] **Step 1: Stop and request exact transfer/execution authorization**

Request authorization that names:

~~~text
Host alias: aliyun-server
Directory: /home/yu/services/secondhand-buyer-intent-acceptance-20260727
Compose project: secondhand-buyer-intent-acceptance
Actions: create new directory, transfer whitelist, generate remote-only
  acceptance secrets, build/start/stop only the dedicated Compose project,
  run synthetic MySQL 8.4 tests, inspect only production container
  name/ID/state/restart count before and after, retain isolated resources
Whitelist: exactly the NUL-delimited paths emitted by
  BUYER_INTENT_SOURCE_LIST_ONLY=1: Makefile; backend/Dockerfile, go.mod,
  go.sum, Go source/tests, and migrations/*.sql; non-sensitive
  deploy/acceptance scripts/yaml/conf/md/sql/Dockerfile files
Forbidden: .env, secrets, databases, uploads, evidence, backups, .git, caches,
  node_modules, backend/app.db, .tmp, protected review documents,
  production SQL/logs/env/mounts/uploads/configuration/service changes
~~~

Do not reuse any F-02, F-06, F-14, or other prior authorization. Wait for an
explicit approval before the next step.

- [ ] **Step 2: Verify exact local source and transfer only the whitelist**

After approval, first require a clean tracked worktree and record:

~~~bash
git diff --quiet
git diff --cached --quiet
git status --short --branch --untracked-files=no
git rev-parse HEAD
~~~

Create the remote directory only if absent. Generate the exact NUL-delimited
source list from the reviewed script, then use it as rsync's `--files-from`
input for both dry-run and real transfer. Do not use include/exclude patterns as
a second whitelist, a deletion option, or a repository-root archive.

Run these commands from the repository root after approval:

~~~bash
f11_transfer_tmp="$(mktemp -d)"
f11_source_files="$f11_transfer_tmp/source-files.z"
f11_local_manifest="$f11_transfer_tmp/local-source-sha256.txt"
f11_remote_manifest="$f11_transfer_tmp/remote-source-sha256.txt"
BUYER_INTENT_SOURCE_LIST_ONLY=1 ./deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh >"$f11_source_files"
BUYER_INTENT_SOURCE_MANIFEST_ONLY=1 ./deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh >"$f11_local_manifest"
while IFS= read -r -d '' f11_source_path; do
  git ls-files --error-unmatch -- "$f11_source_path" >/dev/null
done <"$f11_source_files"
ssh aliyun-server "test ! -e /home/yu/services/secondhand-buyer-intent-acceptance-20260727 && install -d -m 700 /home/yu/services/secondhand-buyer-intent-acceptance-20260727"
rsync -anv --from0 --files-from="$f11_source_files" --relative ./ aliyun-server:/home/yu/services/secondhand-buyer-intent-acceptance-20260727/
rsync -av --from0 --files-from="$f11_source_files" --relative ./ aliyun-server:/home/yu/services/secondhand-buyer-intent-acceptance-20260727/
ssh aliyun-server "cd /home/yu/services/secondhand-buyer-intent-acceptance-20260727 && BUYER_INTENT_SOURCE_MANIFEST_ONLY=1 ./deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh" >"$f11_remote_manifest"
cmp -s "$f11_local_manifest" "$f11_remote_manifest"
~~~

Inspect the rsync dry-run list before the real transfer and stop if any
non-whitelisted path appears. The two sorted SHA-256 manifests must be
byte-identical before running prepare.sh or Docker. Every selected path must be
tracked by the clean recorded HEAD; do not inspect or enumerate unrelated
untracked paths. Record only hashes and relative paths. Retain the local `mktemp` directory through evidence audit,
then remove only that exact directory after the report records the manifest
hash; never reuse an unresolved variable as a deletion target.

- [ ] **Step 3: Generate remote-only secrets and run the guarded target**

From the exact remote directory:

~~~bash
./deploy/acceptance/prepare.sh
env COMPOSE_PROJECT_NAME=secondhand-buyer-intent-acceptance BUYER_INTENT_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_BUYER_INTENT_DATA ACCEPTANCE_DB_ENGINE=mysql8.4 make acceptance-buyer-intent-smoke
~~~

Expected: exit 0, MySQL version 8.4.x, every schema/rejection/API/quality gate
passes, evidence leak count is zero, and production snapshots match. The trap
stops dedicated services but retains its volume/network/containers and evidence
for inspection.

Do not tear down the project, run production 0009, restart production, deploy,
or alter production data.

- [ ] **Step 4: Audit retained evidence before making a status claim**

Read only sanitized evidence and prove:

~~~text
remote commit/source hashes equal reviewed local source
MySQL reports 8.4.x
legacy, marker-only, both-key, and final rerun gates pass
all five rejection groups fail with 1644/45000 and unchanged row summaries
AUTO_MIGRATE=false and true focused tests pass
three closed histories plus one open and one-winner concurrency pass
full, race, and vet gates pass
evidence leak scan is zero and its manifest verifies
production before/after snapshots are identical
Compose project/path are exact and distinct from production
~~~

Any missing or ambiguous artifact means test-server review is not approved.
Keep resources and evidence unchanged for inspection. Because the harness
refuses to reuse an existing project or evidence directory, do not remove,
recreate, overwrite, or rerun after a failure under the original authorization.
First obtain a new exact cleanup-and-rerun authorization naming only
`/home/yu/services/secondhand-buyer-intent-acceptance-20260727`, Compose project
`secondhand-buyer-intent-acceptance`, the exact retained containers/volumes/
networks/evidence allowed to be removed, and the repeated transfer/execution
actions. A code fix may be prepared and locally verified while evidence remains
retained, but it is not server acceptance.

- [ ] **Step 5: Write the sanitized acceptance report**

The report must contain:

~~~text
date, remote path, Compose project, MySQL/tool versions
accepted local commit range and HEAD
local/remote source-manifest SHA-256
matrix case table with exit/pass state
AUTO_MIGRATE false/true results
focused/full/race/vet package counts
evidence file SHA-256 manifest
production snapshot equality result
retained resource/evidence location
explicit statements: production 0009 not run, no deployment, no production
  data/config/service/upload/session changes
~~~

Do not include DSNs, credentials, tokens, row identifiers, contacts, production
rows, or raw logs.

- [ ] **Step 6: Advance only the test-server status**

Update tracked status wording to:

~~~text
F-11 fixed and passed isolated MySQL 8.4 test-server review;
production 0009 not executed and production not deployed.
~~~

Update the design status consistently. Record the accepted F-11 commit range so
F-12 Task 1 can verify it. Do not mark production F-11 closed.

- [ ] **Step 7: Commit the accepted evidence and status**

~~~bash
git add docs/superpowers/reviews/2026-07-27-buyer-intent-open-uniqueness-isolated-acceptance.md docs/full-project-code-review-2026-07-24.md docs/release-readiness.md docs/superpowers/specs/2026-07-27-buyer-intent-open-uniqueness-design.md
git commit -m "docs(buyer): record F-11 server acceptance"
~~~

- [ ] **Step 8: Final F-11 handoff and F-12 prerequisite proof**

Run:

~~~bash
git show --check --stat HEAD
git status --short --branch --untracked-files=no
git log --oneline 4e8ea92..HEAD
test -f backend/migrations/0009_buyer_intent_open_uniqueness.preflight.sql
test -f backend/migrations/0009_buyer_intent_open_uniqueness.up.sql
test -f backend/migrations/0009_buyer_intent_open_uniqueness.postflight.sql
test ! -e backend/migrations/0009_buyer_intent_open_uniqueness.down.sql
~~~

Expected: all checks pass, the tracked worktree is clean, F-11 is code-side and
test-server complete, production remains untouched, and F-12 may begin from the
recorded accepted commit range. Do not enumerate protected untracked paths.

Do not finish or merge the overall branch here. The active first-round goal
continues through F-12, F-15, and the remaining production-independent gates.

---

## Plan Self-Review Map

| Specification requirement | Implemented/verified by |
| --- | --- |
| Nullable MySQL marker and exact unique key | Tasks 2, 4, 6, 7 |
| SQLite partial unique index and no marker | Tasks 1-2 |
| GORM never writes/migrates marker | Tasks 1, 3, 6 |
| Recognized convergence/resume states | Tasks 2, 4, 7 |
| Existing full-chain harness compatibility | Task 5 |
| AUTO_MIGRATE true/false split | Tasks 2, 6, 7 |
| No DML and no down migration | Tasks 4, 7, 9 |
| Unlimited closed history and one open row | Tasks 2, 3, 6, 7 |
| Three complete create/close cycles | Tasks 3, 6, 7 |
| Concurrent one-winner creation | Tasks 3, 6, 7 |
| Duplicate-key re-query and 409/500 split | Tasks 1, 3, 6 |
| State-drift 500 semantics on all paths | Tasks 2, 3, 6, 7 |
| Migration rollback/recovery | Tasks 2, 4, 7 |
| Local focused/full/race/vet evidence | Task 8 |
| Isolated MySQL 8.4 evidence and hashes | Tasks 7, 9 |
| Production container snapshot equality | Tasks 7, 9 |
| Failed acceptance retains evidence; cleanup/rerun needs new authorization | Task 9 |
| Separate code/server/production statuses | Tasks 8-9 |
| Protected files and production untouched | Global constraints, Tasks 7-9 |
| F-12 remains gated on accepted F-11 | Tasks 8-9 |
