package model

import (
	"reflect"
	"strings"
	"testing"
)

func TestFileRecordHasBindingOwnershipFields(t *testing.T) {
	typ := reflect.TypeOf(FileRecord{})
	wantTags := map[string][]string{
		"OwnerMerchantID":     {"idx_file_owner_biz_scan", "priority:1"},
		"CapabilityTokenHash": {"type:char(64)", "uk_file_capability_token"},
		"CapabilityExpiresAt": {"idx_file_capability_expires"},
	}
	for fieldName, snippets := range wantTags {
		field, ok := typ.FieldByName(fieldName)
		if !ok {
			t.Fatalf("FileRecord missing %s", fieldName)
		}
		tag := field.Tag.Get("gorm")
		for _, snippet := range snippets {
			if !strings.Contains(tag, snippet) {
				t.Errorf("%s gorm tag %q missing %q", fieldName, tag, snippet)
			}
		}
	}

	for fieldName, snippets := range map[string][]string{
		"BizType":    {"idx_file_owner_biz_scan", "priority:2"},
		"ScanStatus": {"idx_file_owner_biz_scan", "priority:3"},
	} {
		field, ok := typ.FieldByName(fieldName)
		if !ok {
			t.Fatalf("FileRecord missing %s", fieldName)
		}
		tag := field.Tag.Get("gorm")
		for _, snippet := range snippets {
			if !strings.Contains(tag, snippet) {
				t.Errorf("%s gorm tag %q missing %q", fieldName, tag, snippet)
			}
		}
	}
}
