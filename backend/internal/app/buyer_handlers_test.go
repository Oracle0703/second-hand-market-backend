package app

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

func TestResolveBuyerMerchantScopeUsesDefaultForLegacyMiniapp(t *testing.T) {
	db := newDatabaseOperationsTestDB(t)
	if err := db.AutoMigrate(&model.Merchant{}); err != nil {
		t.Fatalf("migrate merchant: %v", err)
	}

	defaultMerchant := model.Merchant{MerchantNo: "MDEFAULT", MerchantName: "Default", ContactName: "A", ContactPhone: "1", ReviewStatus: model.ReviewApproved}
	explicitMerchant := model.Merchant{MerchantNo: "MEXPLICIT", MerchantName: "Explicit", ContactName: "B", ContactPhone: "2", ReviewStatus: model.ReviewApproved}
	if err := db.Create(&defaultMerchant).Error; err != nil {
		t.Fatalf("create default merchant: %v", err)
	}
	if err := db.Create(&explicitMerchant).Error; err != nil {
		t.Fatalf("create explicit merchant: %v", err)
	}

	srv := &Server{cfg: Config{BuyerDefaultMerchantNo: " MDEFAULT "}, DB: db}

	t.Run("missing_merchant_no_uses_default", func(t *testing.T) {
		merchant, err := srv.resolveBuyerMerchantScope(newBuyerScopeTestContext(t, ""))
		if err != nil {
			t.Fatalf("resolve merchant: %v", err)
		}
		if merchant.ID != defaultMerchant.ID {
			t.Fatalf("merchant ID = %d, want %d", merchant.ID, defaultMerchant.ID)
		}
	})

	t.Run("explicit_merchant_no_wins", func(t *testing.T) {
		merchant, err := srv.resolveBuyerMerchantScope(newBuyerScopeTestContext(t, "?merchant_no=MEXPLICIT"))
		if err != nil {
			t.Fatalf("resolve merchant: %v", err)
		}
		if merchant.ID != explicitMerchant.ID {
			t.Fatalf("merchant ID = %d, want %d", merchant.ID, explicitMerchant.ID)
		}
	})
}

func TestResolveBuyerMerchantScopeStillRejectsMissingMerchantWithoutDefault(t *testing.T) {
	db := newDatabaseOperationsTestDB(t)
	if err := db.AutoMigrate(&model.Merchant{}); err != nil {
		t.Fatalf("migrate merchant: %v", err)
	}

	srv := &Server{DB: db}
	_, err := srv.resolveBuyerMerchantScope(newBuyerScopeTestContext(t, ""))
	var bizErr *common.BizError
	if !errors.As(err, &bizErr) {
		t.Fatalf("error = %v, want BizError", err)
	}
	if bizErr.Code != common.CodeInvalidArgument || bizErr.HTTPStatus != http.StatusBadRequest {
		t.Fatalf("error = %+v, want invalid argument", bizErr)
	}
}

func TestLoadConfigReadsBuyerDefaultMerchantNo(t *testing.T) {
	t.Setenv("APP_ENV", appEnvTest)
	t.Setenv("BUYER_DEFAULT_MERCHANT_NO", " MDEFAULT ")

	cfg := LoadConfig()
	if cfg.BuyerDefaultMerchantNo != "MDEFAULT" {
		t.Fatalf("BuyerDefaultMerchantNo = %q", cfg.BuyerDefaultMerchantNo)
	}
	if err := cfg.ValidateRuntime(); err != nil {
		t.Fatalf("config with default buyer merchant was rejected: %v", err)
	}
}

func newBuyerScopeTestContext(t *testing.T, rawQuery string) *gin.Context {
	t.Helper()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/buyer/products"+rawQuery, nil)
	return context
}
