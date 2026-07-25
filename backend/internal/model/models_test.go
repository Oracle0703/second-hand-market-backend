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
