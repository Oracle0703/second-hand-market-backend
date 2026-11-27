package app

import (
	"context"
	"strings"
	"testing"

	"gorm.io/gorm/clause"
)

func TestP1MySQLTransactionTablePreflight(t *testing.T) {
	db := newP1MySQLServer(t).DB
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	stock := db.NamingStrategy.TableName("ProductStockAdjustment")
	if err := verifyTransactionTables(context.Background(), sqlDB, []string{stock}); err != nil {
		t.Fatalf("InnoDB stock ledger failed preflight: %v", err)
	}

	// These names are unique to the disposable P1 fixture.
	other := stock + "_memory"
	missing := stock + "_missing"
	table := clause.Table{Name: other}
	if err := db.Exec("CREATE TABLE ? (id INT PRIMARY KEY) ENGINE=MEMORY", table).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Exec("DROP TABLE ?", table).Error; err != nil {
			t.Error(err)
		}
	})
	err = verifyTransactionTables(context.Background(), sqlDB, []string{stock, other, missing})
	if err == nil || !strings.HasSuffix(err.Error(), other+", "+missing) {
		t.Fatalf("expected both invalid tables in preflight result, got %v", err)
	}
}
