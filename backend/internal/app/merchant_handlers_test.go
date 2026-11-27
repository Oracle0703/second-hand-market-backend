package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

func TestHandleDashboardFailsWhenStatusCountQueryFails(t *testing.T) {
	db := newDatabaseOperationsTestDB(t)
	if err := db.AutoMigrate(&model.Product{}); err != nil {
		t.Fatalf("migrate product: %v", err)
	}

	srv := &Server{DB: db}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/merchant/dashboard", nil)
	common.SetActor(c, common.Actor{UserID: 1, UserType: "merchant", MerchantID: 1})

	srv.handleDashboard(c)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusInternalServerError, recorder.Body.String())
	}
	var resp common.APIResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Code != common.CodeInternal {
		t.Fatalf("code = %d, want %d", resp.Code, common.CodeInternal)
	}
}
