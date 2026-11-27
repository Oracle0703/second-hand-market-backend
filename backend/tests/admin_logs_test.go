package tests

import (
	"net/http"
	"testing"

	"gorm.io/datatypes"
	"second-hand-market-backend/backend/internal/model"
)

func TestAdminLogsResponseUsesSnakeCaseFields(t *testing.T) {
	srv := newTestServer(t)
	admin := adminAccessToken(t, srv)
	merchantID := uint64(42)
	fromStatus := "PENDING"
	toStatus := "APPROVED"
	entry := model.OperationLog{
		RequestID:    "req-admin-logs-contract",
		OperatorType: "ADMIN",
		OperatorID:   7,
		MerchantID:   &merchantID,
		Action:       "admin_logs_contract_test",
		ResourceType: "MERCHANT",
		ResourceID:   99,
		FromStatus:   &fromStatus,
		ToStatus:     &toStatus,
		Method:       http.MethodPost,
		Path:         "/api/v1/admin/test",
		IP:           "10.0.0.1",
		UserAgent:    "contract-test-agent",
		ResultCode:   0,
		DetailJSON:   datatypes.JSON([]byte(`{"secret":"value"}`)),
	}
	if err := srv.DB.Create(&entry).Error; err != nil {
		t.Fatalf("seed operation log: %v", err)
	}

	resp := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/admin/logs?action=admin_logs_contract_test", nil, map[string]string{"Authorization": "Bearer " + admin})
	if resp.Code != 0 {
		t.Fatalf("admin logs failed: %+v", resp)
	}
	items, ok := resp.Data["items"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("unexpected items: %+v", resp.Data)
	}
	row, ok := items[0].(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected item shape: %#v", items[0])
	}

	expected := map[string]interface{}{
		"id":            float64(entry.ID),
		"request_id":    "req-admin-logs-contract",
		"operator_type": "ADMIN",
		"operator_id":   float64(7),
		"merchant_id":   float64(42),
		"action":        "admin_logs_contract_test",
		"resource_type": "MERCHANT",
		"resource_id":   float64(99),
		"from_status":   "PENDING",
		"to_status":     "APPROVED",
		"result_code":   float64(0),
	}
	for key, want := range expected {
		got, present := row[key]
		if !present {
			t.Fatalf("missing field %q in %+v", key, row)
		}
		if got != want {
			t.Fatalf("field %q = %#v, want %#v", key, got, want)
		}
	}
	if str(row["created_at"]) == "" {
		t.Fatalf("missing created_at in %+v", row)
	}
	if len(row) != len(expected)+1 {
		t.Fatalf("unexpected extra fields in %+v", row)
	}
}
