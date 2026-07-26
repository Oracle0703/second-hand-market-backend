package model

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestFileRecordHasAnonymousUploadGovernanceFields(t *testing.T) {
	typ := reflect.TypeOf(FileRecord{})
	want := map[string]struct {
		fieldType reflect.Type
		tagParts  []string
	}{
		"SourceIPHash": {
			fieldType: reflect.TypeOf((*string)(nil)),
			tagParts:  []string{"type:char(64)", "index:idx_file_source_created,priority:1"},
		},
		"CleanupAfter": {
			fieldType: reflect.TypeOf((*time.Time)(nil)),
			tagParts:  []string{"index:idx_file_cleanup_candidate,priority:3"},
		},
		"CleanupClaimedAt": {
			fieldType: reflect.TypeOf((*time.Time)(nil)),
			tagParts:  []string{"index:idx_file_cleanup_candidate,priority:4"},
		},
		"CleanupClaimToken": {
			fieldType: reflect.TypeOf((*string)(nil)),
			tagParts:  []string{"type:char(64)"},
		},
		"CleanupAttempts": {
			fieldType: reflect.TypeOf(uint32(0)),
			tagParts:  []string{"not null", "default:0"},
		},
	}

	for name, expected := range want {
		field, ok := typ.FieldByName(name)
		if !ok {
			t.Errorf("FileRecord missing %s", name)
			continue
		}
		if field.Type != expected.fieldType {
			t.Errorf("FileRecord.%s type = %v, want %v", name, field.Type, expected.fieldType)
		}
		gormTag := field.Tag.Get("gorm")
		for _, part := range expected.tagParts {
			if !strings.Contains(gormTag, part) {
				t.Errorf("FileRecord.%s gorm tag %q missing %q", name, gormTag, part)
			}
		}
	}

	indexTags := map[string]string{
		"UploaderType":    "index:idx_file_cleanup_candidate,priority:1",
		"OwnerMerchantID": "index:idx_file_cleanup_candidate,priority:2",
		"CreatedAt":       "index:idx_file_source_created,priority:2",
	}
	for name, expected := range indexTags {
		field, ok := typ.FieldByName(name)
		if !ok {
			t.Fatalf("FileRecord missing existing field %s", name)
		}
		if tag := field.Tag.Get("gorm"); !strings.Contains(tag, expected) {
			t.Errorf("FileRecord.%s gorm tag %q missing %q", name, tag, expected)
		}
	}
}

func TestFileQuotaGuardTableName(t *testing.T) {
	if got := (FileQuotaGuard{}).TableName(); got != "file_quota_guards" {
		t.Fatalf("table = %q, want file_quota_guards", got)
	}

	typ := reflect.TypeOf(FileQuotaGuard{})
	want := map[string]struct {
		fieldType reflect.Type
		tagParts  []string
	}{
		"ID": {
			fieldType: reflect.TypeOf(uint8(0)),
			tagParts:  []string{"primaryKey", "autoIncrement:false"},
		},
		"GuardName": {
			fieldType: reflect.TypeOf(""),
			tagParts:  []string{"size:32", "not null", "uniqueIndex:uk_file_quota_guard_name"},
		},
		"CreatedAt": {
			fieldType: reflect.TypeOf(time.Time{}),
			tagParts:  []string{"not null"},
		},
	}
	for name, expected := range want {
		field, ok := typ.FieldByName(name)
		if !ok {
			t.Errorf("FileQuotaGuard missing %s", name)
			continue
		}
		if field.Type != expected.fieldType {
			t.Errorf("FileQuotaGuard.%s type = %v, want %v", name, field.Type, expected.fieldType)
		}
		gormTag := field.Tag.Get("gorm")
		for _, part := range expected.tagParts {
			if !strings.Contains(gormTag, part) {
				t.Errorf("FileQuotaGuard.%s gorm tag %q missing %q", name, gormTag, part)
			}
		}
	}
}
