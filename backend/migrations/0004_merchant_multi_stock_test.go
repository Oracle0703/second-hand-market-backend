package migrations_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const migrationPrefix = "0004_merchant_multi_stock"

func TestMerchantMultiStockMigrationArtifactsAreBounded(t *testing.T) {
	expectedStatements := map[string]int{
		"preflight.sql":  7,
		"up.sql":         9,
		"postflight.sql": 7,
		"down.sql":       6,
	}
	for suffix, expectedCount := range expectedStatements {
		source := loadMigrationArtifact(t, suffix)
		if strings.Contains(source, "\r") {
			t.Fatalf("%s contains a carriage return", suffix)
		}
		if len(source) == 0 || len(source) > 64*1024 {
			t.Fatalf("%s size = %d", suffix, len(source))
		}
		normalized := normalizeSQL(source)
		for _, forbidden := range []string{
			"create procedure",
			"delimiter ",
			"drop table",
			"truncate table",
			"delete from",
			"update products",
			"update orders",
			"insert into",
			"replace into",
		} {
			if strings.Contains(normalized, forbidden) {
				t.Fatalf("%s contains forbidden SQL %q", suffix, forbidden)
			}
		}
		statements, err := splitMigrationSQL(source)
		if err != nil {
			t.Fatalf("split %s: %v", suffix, err)
		}
		if len(statements) != expectedCount {
			t.Fatalf(
				"%s statements = %d, want %d",
				suffix,
				len(statements),
				expectedCount,
			)
		}
	}
}

func TestMerchantMultiStockUpRepeatsPreflightBeforeAtomicDDL(t *testing.T) {
	preflight, err := splitMigrationSQL(loadMigrationArtifact(t, "preflight.sql"))
	if err != nil {
		t.Fatalf("split preflight: %v", err)
	}
	up, err := splitMigrationSQL(loadMigrationArtifact(t, "up.sql"))
	if err != nil {
		t.Fatalf("split up: %v", err)
	}
	if len(up) != len(preflight)+2 {
		t.Fatalf("up statements = %d, preflight = %d", len(up), len(preflight))
	}
	for statementIndex := range preflight {
		if normalizeSQL(up[statementIndex]) != normalizeSQL(preflight[statementIndex]) {
			t.Fatalf("up guard %d drifted from preflight", statementIndex+1)
		}
	}

	guardSQL := normalizeSQL(strings.Join(preflight, ";\n"))
	for _, required := range []string{
		"database() is null",
		"upper(engine) = 'innodb'",
		"column_name = 'reserved_stock'",
		"column_name = 'quantity'",
		"column_name = 'status'",
		"index_name = 'uk_product_active'",
		"non_unique = 0",
		"index_name = 'idx_product_active'",
		"index_name = 'idx_product_id'",
		"index_name = 'idx_active_order'",
		"sum( case when column_name = 'product_id' then 1 else 0 end ) > 0",
		"binary status not in ( binary 'completed', binary 'closed' )",
		"binary 'on_shelf'",
		"where is_active = 1",
		"active_order_id is not null",
		"locked_at is not null",
		"where stock < 0",
	} {
		if !strings.Contains(guardSQL, required) {
			t.Fatalf("preflight is missing %q", required)
		}
	}

	ddl := normalizeSQL(strings.Join(up[len(preflight):], ";\n"))
	for _, required := range []string{
		"alter table products",
		"add column reserved_stock int not null default 0 after stock",
		"constraint chk_products_stock_reservation_bounds",
		"check ( stock >= 0 and reserved_stock >= 0 and reserved_stock <= stock ) enforced",
		"alter table orders",
		"drop index uk_product_active",
		"add column quantity int not null default 1 after product_id",
		"constraint chk_orders_quantity_positive",
		"check (quantity > 0) enforced",
		"add index idx_product_active (product_id, is_active)",
	} {
		if !strings.Contains(ddl, required) {
			t.Fatalf("up migration is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"if exists",
		"if not exists",
		"add unique",
		"unique key",
		"create unique",
	} {
		if strings.Contains(ddl, forbidden) {
			t.Fatalf("up migration contains unsafe clause %q", forbidden)
		}
	}
	if strings.Count(ddl, "alter table products") != 1 ||
		strings.Count(ddl, "alter table orders") != 1 {
		t.Fatal("up migration must use one atomic ALTER per table")
	}
	if strings.Count(ddl, "uk_product_active") != 1 {
		t.Fatal("legacy unique index must appear only in DROP INDEX")
	}
}

func TestMerchantMultiStockPostflightVerifiesExactFinalState(t *testing.T) {
	source := normalizeSQL(loadMigrationArtifact(t, "postflight.sql"))
	for _, required := range []string{
		"column_name = 'reserved_stock'",
		"column_default = '0'",
		"column_name = 'quantity'",
		"column_default = '1'",
		"index_name = 'uk_product_active'",
		"index_name = 'idx_product_active'",
		"non_unique = 1",
		"sum( case when column_name = 'product_id' then 1 else 0 end ) > 0",
		"seq_in_index = 1 and column_name = 'product_id'",
		"seq_in_index = 2 and column_name = 'is_active'",
		"chk_products_stock_reservation_bounds",
		"'stock>=0andreserved_stock>=0andreserved_stock<=stock'",
		"chk_orders_quantity_positive",
		"'quantity>0'",
		"from orders where quantity <> 1",
		"reserved_stock <> 0",
		"where is_active = 1",
		"binary status not in ( binary 'completed', binary 'closed' )",
		"binary 'on_shelf'",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("postflight is missing %q", required)
		}
	}
	for _, forbidden := range []string{"alter table", "update ", "delete ", "insert "} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("postflight contains mutation %q", forbidden)
		}
	}
}

func TestMerchantMultiStockDownRejectsDataLossAndNeverRestoresUniqueness(t *testing.T) {
	statements, err := splitMigrationSQL(loadMigrationArtifact(t, "down.sql"))
	if err != nil {
		t.Fatalf("split down: %v", err)
	}
	normalized := normalizeSQL(strings.Join(statements, ";\n"))
	for _, required := range []string{
		"from orders where quantity <> 1",
		"from orders where is_active = 1",
		"sum( case when column_name = 'product_id' then 1 else 0 end ) > 0",
		"binary status not in ( binary 'completed', binary 'closed' )",
		"binary 'on_shelf'",
		"reserved_stock <> 0",
		"drop check chk_orders_quantity_positive",
		"drop column quantity",
		"drop check chk_products_stock_reservation_bounds",
		"drop column reserved_stock",
	} {
		if !strings.Contains(normalized, required) {
			t.Fatalf("limited down migration is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"add unique",
		"create unique",
		"unique key",
		"drop index idx_product_active",
		"drop index idx_product_id",
		"drop index idx_active_order",
	} {
		if strings.Contains(normalized, forbidden) {
			t.Fatalf("limited down migration contains forbidden SQL %q", forbidden)
		}
	}
}

func loadMigrationArtifact(t *testing.T, suffix string) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve migration test path")
	}
	path := filepath.Join(filepath.Dir(currentFile), migrationPrefix+"."+suffix)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Base(path), err)
	}
	return string(data)
}

func normalizeSQL(source string) string {
	return strings.ToLower(strings.Join(strings.Fields(source), " "))
}

func splitMigrationSQL(source string) ([]string, error) {
	var (
		statements   []string
		current      strings.Builder
		quote        byte
		lineComment  bool
		blockComment bool
	)

	appendStatement := func() {
		statement := strings.TrimSpace(current.String())
		current.Reset()
		if statement != "" {
			statements = append(statements, statement)
		}
	}

	for index := 0; index < len(source); index++ {
		character := source[index]
		if lineComment {
			if character == '\n' {
				lineComment = false
				current.WriteByte('\n')
			}
			continue
		}
		if blockComment {
			if character == '*' && index+1 < len(source) && source[index+1] == '/' {
				blockComment = false
				current.WriteByte(' ')
				index++
			}
			continue
		}
		if quote != 0 {
			current.WriteByte(character)
			if character == '\\' && quote != '`' && index+1 < len(source) {
				index++
				current.WriteByte(source[index])
				continue
			}
			if character == quote {
				if index+1 < len(source) && source[index+1] == quote {
					index++
					current.WriteByte(source[index])
					continue
				}
				quote = 0
			}
			continue
		}

		switch {
		case character == '-' && index+1 < len(source) && source[index+1] == '-':
			lineComment = true
			current.WriteByte(' ')
			index++
		case character == '#':
			lineComment = true
			current.WriteByte(' ')
		case character == '/' && index+1 < len(source) && source[index+1] == '*':
			blockComment = true
			current.WriteByte(' ')
			index++
		case character == '\'' || character == '"' || character == '`':
			quote = character
			current.WriteByte(character)
		case character == ';':
			appendStatement()
		default:
			current.WriteByte(character)
		}
	}

	if quote != 0 || blockComment {
		return nil, fmt.Errorf("unterminated SQL construct")
	}
	appendStatement()
	if len(statements) == 0 {
		return nil, fmt.Errorf("SQL is empty")
	}
	return statements, nil
}
