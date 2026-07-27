package app

import (
	"database/sql"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/model"
)

const (
	buyerIntentLegacyIndex = "uk_buyer_product_open"
	buyerIntentOpenIndex   = "uk_buyer_intent_open"
)

type buyerIntentIndexState struct {
	Present bool
	Exact   bool
}

type buyerIntentSchemaState struct {
	Rows              int64
	MarkerPresent     bool
	MarkerExact       bool
	Legacy            buyerIntentIndexState
	Open              buyerIntentIndexState
	RelevantLookalike bool
}

type sqliteBuyerIntentColumn struct {
	Name    string
	NotNull int `gorm:"column:notnull"`
	Hidden  int
}

type sqliteBuyerIntentIndex struct {
	Name    string
	Unique  int `gorm:"column:unique"`
	Origin  string
	Partial int
}

type mysqlBuyerIntentColumn struct {
	Name                 string `gorm:"column:column_name"`
	DataType             string `gorm:"column:data_type"`
	ColumnType           string `gorm:"column:column_type"`
	IsNullable           string `gorm:"column:is_nullable"`
	GenerationExpression string `gorm:"column:generation_expression"`
	Extra                string `gorm:"column:extra"`
	IsGenerated          string `gorm:"column:is_generated"`
}

type mysqlBuyerIntentIndexColumn struct {
	IndexName  string         `gorm:"column:index_name"`
	NonUnique  int            `gorm:"column:non_unique"`
	Sequence   int            `gorm:"column:seq_in_index"`
	ColumnName sql.NullString `gorm:"column:column_name"`
}

func migrateBuyerIntentOpenUniqueness(db *gorm.DB) error {
	switch db.Dialector.Name() {
	case "sqlite":
		return migrateSQLiteBuyerIntentOpenUniqueness(db)
	case "mysql":
		return migrateMySQLBuyerIntentOpenUniqueness(db)
	default:
		return fmt.Errorf("unsupported buyer intent schema dialect: %s", db.Dialector.Name())
	}
}

func verifyBuyerIntentOpenUniqueness(db *gorm.DB) error {
	switch db.Dialector.Name() {
	case "sqlite":
		state, err := inspectSQLiteBuyerIntentSchema(db)
		if err != nil {
			return err
		}
		if state.MarkerPresent || state.Legacy.Present || !state.Open.Exact || state.RelevantLookalike {
			return fmt.Errorf("buyer intent uniqueness schema is missing or drifted")
		}
	case "mysql":
		state, err := inspectMySQLBuyerIntentSchema(db)
		if err != nil {
			return err
		}
		if !state.MarkerExact || state.Legacy.Present || !state.Open.Exact || state.RelevantLookalike {
			return fmt.Errorf("buyer intent uniqueness schema is missing or drifted")
		}
	default:
		return fmt.Errorf("unsupported buyer intent schema dialect: %s", db.Dialector.Name())
	}
	return verifyBuyerIntentRows(db)
}

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

func migrateSQLiteBuyerIntentOpenUniqueness(db *gorm.DB) error {
	state, err := inspectSQLiteBuyerIntentSchema(db)
	if err != nil {
		return err
	}
	if state.MarkerPresent || state.RelevantLookalike {
		return fmt.Errorf("buyer intent uniqueness schema is missing or drifted")
	}
	switch {
	case state.Rows == 0 && !state.Legacy.Present && !state.Open.Present:
	case state.Legacy.Exact && !state.Open.Present:
	case state.Legacy.Exact && state.Open.Exact:
	case !state.Legacy.Present && state.Open.Exact:
		return verifyBuyerIntentOpenUniqueness(db)
	default:
		return fmt.Errorf("buyer intent uniqueness schema is missing or drifted")
	}
	if err := verifyBuyerIntentRows(db); err != nil {
		return err
	}
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
}

func inspectSQLiteBuyerIntentSchema(db *gorm.DB) (buyerIntentSchemaState, error) {
	var state buyerIntentSchemaState
	var columns []sqliteBuyerIntentColumn
	if err := db.Raw(`PRAGMA table_xinfo('buyer_intents')`).Scan(&columns).Error; err != nil {
		return state, fmt.Errorf("inspect buyer intent columns: %w", err)
	}
	if err := verifySQLiteBuyerIntentColumns(columns); err != nil {
		return state, err
	}
	for _, column := range columns {
		if column.Name == "open_marker" {
			state.MarkerPresent = true
		}
	}
	if err := db.Table("buyer_intents").Count(&state.Rows).Error; err != nil {
		return state, fmt.Errorf("count buyer intents: %w", err)
	}

	var indexes []sqliteBuyerIntentIndex
	if err := db.Raw(`PRAGMA index_list('buyer_intents')`).Scan(&indexes).Error; err != nil {
		return state, fmt.Errorf("inspect buyer intent indexes: %w", err)
	}
	for _, index := range indexes {
		if index.Origin == "pk" {
			continue
		}
		indexColumns, err := sqliteBuyerIntentIndexColumns(db, index.Name)
		if err != nil {
			return state, err
		}
		indexSQL, err := sqliteBuyerIntentIndexSQL(db, index.Name)
		if err != nil {
			return state, err
		}
		switch index.Name {
		case buyerIntentLegacyIndex:
			state.Legacy.Present = true
			state.Legacy.Exact = index.Unique == 1 && index.Partial == 0 &&
				equalStrings(indexColumns, []string{"buyer_id", "product_id", "is_open"}) &&
				normalizeSQLiteIndexSQL(indexSQL) == "createuniqueindexuk_buyer_product_openonbuyer_intents(buyer_id,product_id,is_open)"
		case buyerIntentOpenIndex:
			state.Open.Present = true
			state.Open.Exact = index.Unique == 1 && index.Partial == 1 &&
				equalStrings(indexColumns, []string{"buyer_id", "product_id"}) &&
				normalizeSQLiteIndexSQL(indexSQL) == "createuniqueindexuk_buyer_intent_openonbuyer_intents(buyer_id,product_id)whereis_open=1"
		default:
			if index.Unique == 1 && containsBuyerIntentKey(indexColumns) {
				state.RelevantLookalike = true
			}
		}
	}
	return state, nil
}

func verifySQLiteBuyerIntentColumns(columns []sqliteBuyerIntentColumn) error {
	required := map[string]int{
		"buyer_id":   -1,
		"product_id": -1,
		"status":     -1,
		"is_open":    -1,
	}
	for _, column := range columns {
		if _, ok := required[column.Name]; !ok {
			continue
		}
		if (column.NotNull != 0 && column.NotNull != 1) || column.Hidden != 0 {
			return fmt.Errorf("buyer intent columns are missing or drifted")
		}
		required[column.Name] = column.NotNull
	}
	// GORM development schemas omit NOT NULL; formal schemas declare it on all four columns.
	notNull := -1
	for _, columnNotNull := range required {
		if columnNotNull == -1 {
			return fmt.Errorf("buyer intent columns are missing or drifted")
		}
		if notNull == -1 {
			notNull = columnNotNull
			continue
		}
		if columnNotNull != notNull {
			return fmt.Errorf("buyer intent columns are missing or drifted")
		}
	}
	return nil
}

func sqliteBuyerIntentIndexColumns(db *gorm.DB, name string) ([]string, error) {
	var columns []struct {
		Sequence int `gorm:"column:seqno"`
		Name     string
	}
	if err := db.Raw(fmt.Sprintf("PRAGMA index_info(%q)", name)).Scan(&columns).Error; err != nil {
		return nil, fmt.Errorf("inspect buyer intent index %s: %w", name, err)
	}
	result := make([]string, len(columns))
	for i, column := range columns {
		result[i] = column.Name
	}
	return result, nil
}

func sqliteBuyerIntentIndexSQL(db *gorm.DB, name string) (string, error) {
	var indexSQL sql.NullString
	if err := db.Raw(`
		SELECT sql
		FROM sqlite_master
		WHERE type = 'index' AND name = ?`, name).Scan(&indexSQL).Error; err != nil {
		return "", fmt.Errorf("inspect buyer intent index SQL %s: %w", name, err)
	}
	return indexSQL.String, nil
}

func verifySQLiteBuyerIntentOpenIndex(db *gorm.DB) error {
	state, err := inspectSQLiteBuyerIntentSchema(db)
	if err != nil {
		return err
	}
	if !state.Open.Exact || state.RelevantLookalike {
		return fmt.Errorf("buyer intent uniqueness schema is missing or drifted")
	}
	return nil
}

func migrateMySQLBuyerIntentOpenUniqueness(db *gorm.DB) error {
	state, err := inspectMySQLBuyerIntentSchema(db)
	if err != nil {
		return err
	}
	if state.RelevantLookalike {
		return fmt.Errorf("buyer intent uniqueness schema is missing or drifted")
	}
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
	if err := verifyBuyerIntentRows(db); err != nil {
		return err
	}
	if !state.MarkerPresent {
		if err := db.Exec(`
			ALTER TABLE buyer_intents
			ADD COLUMN open_marker TINYINT
				GENERATED ALWAYS AS (
					CASE WHEN is_open = 1 THEN 1 ELSE NULL END
				) STORED AFTER is_open`).Error; err != nil {
			return fmt.Errorf("add buyer intent open marker: %w", err)
		}
		if err := verifyMySQLBuyerIntentMarker(db); err != nil {
			return err
		}
	}
	if !state.Open.Present {
		if err := db.Exec(`
			ALTER TABLE buyer_intents
			ADD UNIQUE KEY uk_buyer_intent_open
				(buyer_id, product_id, open_marker)`).Error; err != nil {
			return fmt.Errorf("add buyer intent open key: %w", err)
		}
		if err := verifyMySQLBuyerIntentOpenIndex(db); err != nil {
			return err
		}
	}
	if state.Legacy.Present {
		if err := db.Exec(`
			ALTER TABLE buyer_intents
			DROP INDEX uk_buyer_product_open`).Error; err != nil {
			return fmt.Errorf("drop legacy buyer intent index: %w", err)
		}
	}
	return verifyBuyerIntentOpenUniqueness(db)
}

func inspectMySQLBuyerIntentSchema(db *gorm.DB) (buyerIntentSchemaState, error) {
	var state buyerIntentSchemaState
	var columns []mysqlBuyerIntentColumn
	if err := db.Raw(`
		SELECT column_name, data_type, column_type, is_nullable,
			generation_expression, extra,
			CASE
				WHEN generation_expression IS NOT NULL AND generation_expression <> '' THEN 'ALWAYS'
				ELSE 'NEVER'
			END AS is_generated
		FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = 'buyer_intents'`).Scan(&columns).Error; err != nil {
		return state, fmt.Errorf("inspect buyer intent columns: %w", err)
	}
	if err := verifyMySQLBuyerIntentColumns(columns, &state); err != nil {
		return state, err
	}
	if err := db.Table("buyer_intents").Count(&state.Rows).Error; err != nil {
		return state, fmt.Errorf("count buyer intents: %w", err)
	}

	var indexColumns []mysqlBuyerIntentIndexColumn
	if err := db.Raw(`
		SELECT index_name, non_unique, seq_in_index, column_name
		FROM information_schema.statistics
		WHERE table_schema = DATABASE() AND table_name = 'buyer_intents'
		ORDER BY index_name, seq_in_index`).Scan(&indexColumns).Error; err != nil {
		return state, fmt.Errorf("inspect buyer intent indexes: %w", err)
	}
	grouped := make(map[string][]mysqlBuyerIntentIndexColumn)
	for _, column := range indexColumns {
		grouped[column.IndexName] = append(grouped[column.IndexName], column)
	}
	for name, index := range grouped {
		if name == "PRIMARY" {
			continue
		}
		columns := make([]string, len(index))
		unique := true
		for i, column := range index {
			columns[i] = column.ColumnName.String
			unique = unique && column.NonUnique == 0
		}
		switch name {
		case buyerIntentLegacyIndex:
			state.Legacy.Present = true
			state.Legacy.Exact = unique && equalStrings(columns, []string{"buyer_id", "product_id", "is_open"})
		case buyerIntentOpenIndex:
			state.Open.Present = true
			state.Open.Exact = unique && equalStrings(columns, []string{"buyer_id", "product_id", "open_marker"})
		default:
			if unique && containsBuyerIntentKey(columns) {
				state.RelevantLookalike = true
			}
		}
	}
	return state, nil
}

func verifyMySQLBuyerIntentColumns(columns []mysqlBuyerIntentColumn, state *buyerIntentSchemaState) error {
	required := map[string]mysqlBuyerIntentColumn{
		"buyer_id":   {},
		"product_id": {},
		"status":     {},
		"is_open":    {},
	}
	for _, column := range columns {
		if _, ok := required[column.Name]; ok {
			if strings.TrimSpace(column.GenerationExpression) != "" ||
				strings.Contains(strings.ToUpper(column.Extra), "GENERATED") ||
				!strings.EqualFold(column.IsGenerated, "NEVER") {
				return fmt.Errorf("buyer intent columns are missing or drifted")
			}
			required[column.Name] = column
		}
		if column.Name != "open_marker" {
			continue
		}
		state.MarkerPresent = true
		state.MarkerExact = strings.EqualFold(column.DataType, "tinyint") &&
			strings.EqualFold(column.ColumnType, "tinyint") &&
			strings.EqualFold(column.IsNullable, "YES") &&
			strings.EqualFold(column.IsGenerated, "ALWAYS") &&
			strings.Contains(strings.ToUpper(column.Extra), "STORED GENERATED") &&
			normalizeMySQLGenerationExpression(column.GenerationExpression) == "casewhenis_open=1then1elsenullend"
	}
	formal := map[string]mysqlBuyerIntentColumn{
		"buyer_id":   {DataType: "bigint", ColumnType: "bigint", IsNullable: "NO"},
		"product_id": {DataType: "bigint", ColumnType: "bigint", IsNullable: "NO"},
		"status":     {DataType: "varchar", ColumnType: "varchar(16)", IsNullable: "NO"},
		"is_open":    {DataType: "tinyint", ColumnType: "tinyint(1)", IsNullable: "NO"},
	}
	gormDevelopment := map[string]mysqlBuyerIntentColumn{
		"buyer_id":   {DataType: "bigint", ColumnType: "bigint unsigned", IsNullable: "YES"},
		"product_id": {DataType: "bigint", ColumnType: "bigint unsigned", IsNullable: "YES"},
		"status":     {DataType: "varchar", ColumnType: "varchar(16)", IsNullable: "YES"},
		"is_open":    {DataType: "tinyint", ColumnType: "tinyint(1)", IsNullable: "YES"},
	}
	if !matchesMySQLBuyerIntentColumnLayout(required, formal) &&
		!matchesMySQLBuyerIntentColumnLayout(required, gormDevelopment) {
		return fmt.Errorf("buyer intent columns are missing or drifted")
	}
	return nil
}

func matchesMySQLBuyerIntentColumnLayout(columns, layout map[string]mysqlBuyerIntentColumn) bool {
	for name, expected := range layout {
		column := columns[name]
		if !strings.EqualFold(column.DataType, expected.DataType) ||
			!strings.EqualFold(column.ColumnType, expected.ColumnType) ||
			!strings.EqualFold(column.IsNullable, expected.IsNullable) {
			return false
		}
	}
	return true
}

func verifyMySQLBuyerIntentMarker(db *gorm.DB) error {
	state, err := inspectMySQLBuyerIntentSchema(db)
	if err != nil {
		return err
	}
	if !state.MarkerExact {
		return fmt.Errorf("buyer intent uniqueness schema is missing or drifted")
	}
	return nil
}

func verifyMySQLBuyerIntentOpenIndex(db *gorm.DB) error {
	state, err := inspectMySQLBuyerIntentSchema(db)
	if err != nil {
		return err
	}
	if !state.Open.Exact || state.RelevantLookalike {
		return fmt.Errorf("buyer intent uniqueness schema is missing or drifted")
	}
	return nil
}

func normalizeSQLiteIndexSQL(value string) string {
	return normalizeSchemaExpression(value, false)
}

func normalizeMySQLGenerationExpression(value string) string {
	return normalizeSchemaExpression(value, true)
}

func normalizeSchemaExpression(value string, removeParentheses bool) string {
	var normalized strings.Builder
	for _, char := range strings.ToLower(value) {
		switch char {
		case ' ', '\t', '\n', '\r', '\v', '\f', '`':
			continue
		case '(', ')':
			if removeParentheses {
				continue
			}
		}
		normalized.WriteRune(char)
	}
	return normalized.String()
}

func equalStrings(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for i := range actual {
		if actual[i] != expected[i] {
			return false
		}
	}
	return true
}

func containsBuyerIntentKey(columns []string) bool {
	buyer := false
	product := false
	for _, column := range columns {
		buyer = buyer || column == "buyer_id"
		product = product || column == "product_id"
	}
	return buyer && product
}
