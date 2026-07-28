package app

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const idempotencyTransactionalTablesDriftError = "idempotency transactional tables are missing or drifted"

var idempotencyTransactionalTables = []string{
	"buyer_intents",
	"idempotency_records",
	"operation_logs",
	"order_events",
	"orders",
	"products",
}

type mysqlTableEngine struct {
	TableName string `gorm:"column:table_name"`
	Engine    string `gorm:"column:engine"`
}

func verifyIdempotencyTransactionalTables(db *gorm.DB) error {
	if db.Dialector.Name() != "mysql" {
		return nil
	}
	var rows []mysqlTableEngine
	if err := db.Raw(`
		SELECT table_name, engine
		FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_name IN ?
		ORDER BY table_name`, idempotencyTransactionalTables).Scan(&rows).Error; err != nil {
		return fmt.Errorf("inspect idempotency transactional tables: %w", err)
	}
	return validateIdempotencyTableEngines(rows)
}

func validateIdempotencyTableEngines(rows []mysqlTableEngine) error {
	if len(rows) != len(idempotencyTransactionalTables) {
		return fmt.Errorf(idempotencyTransactionalTablesDriftError)
	}

	expected := make(map[string]bool, len(idempotencyTransactionalTables))
	for _, tableName := range idempotencyTransactionalTables {
		expected[tableName] = false
	}
	for _, row := range rows {
		seen, ok := expected[row.TableName]
		if !ok || seen || !strings.EqualFold(row.Engine, "InnoDB") {
			return fmt.Errorf(idempotencyTransactionalTablesDriftError)
		}
		expected[row.TableName] = true
	}
	for _, seen := range expected {
		if !seen {
			return fmt.Errorf(idempotencyTransactionalTablesDriftError)
		}
	}
	return nil
}
