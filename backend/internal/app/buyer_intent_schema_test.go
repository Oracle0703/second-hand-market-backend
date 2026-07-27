package app

import (
	"errors"
	"strings"
	"testing"

	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/model"
)

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
		DBDSN: "file:" + strings.ReplaceAll(t.Name(), "/", "_") +
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
