package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// VerifyTransactionTables checks the tables used together by order, intent,
// inventory and product-image transactions before a MySQL release.
func VerifyTransactionTables(ctx context.Context, db *sql.DB) error {
	return verifyTransactionTables(ctx, db, []string{
		"buyer_intents", "files", "idempotency_records", "operation_logs",
		"order_events", "orders", "product_images", "product_stock_adjustments", "products",
	})
}

func verifyTransactionTables(ctx context.Context, db *sql.DB, required []string) error {
	rows, err := db.QueryContext(ctx, `
		SELECT TABLE_NAME FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_TYPE = 'BASE TABLE' AND ENGINE = 'InnoDB'`)
	inspectionFailed := errors.New("could not inspect transaction tables")
	if err != nil {
		return inspectionFailed
	}
	defer rows.Close()
	found := make(map[string]bool)
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return inspectionFailed
		}
		found[table] = true
	}
	if rows.Err() != nil {
		return inspectionFailed
	}
	var missing []string
	for _, table := range required {
		if !found[table] {
			missing = append(missing, table)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing or not visible as InnoDB tables: %s", strings.Join(missing, ", "))
	}
	return nil
}
