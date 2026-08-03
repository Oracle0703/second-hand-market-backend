package model

import (
	"reflect"
	"strings"
	"testing"
)

func TestImageBackfillRunPrimaryKeyColumnIsExplicit(t *testing.T) {
	field, ok := reflect.TypeOf(ImageBackfillRun{}).FieldByName("ID")
	if !ok {
		t.Fatal("ImageBackfillRun.ID field is missing")
	}

	tag := field.Tag.Get("gorm")
	if !strings.Contains(tag, "column:id") {
		t.Fatalf("ImageBackfillRun.ID gorm tag must explicitly bind to column:id, got %q", tag)
	}
}
