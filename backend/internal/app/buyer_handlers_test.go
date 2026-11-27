package app

import (
	"errors"
	"fmt"
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

func TestHandleBuyerSummaryFailsWhenCountQueryFails(t *testing.T) {
	cases := []struct {
		name    string
		missing any
		buyer   bool
	}{
		{name: "favorites", missing: &model.BuyerFavorite{}},
		{name: "histories", missing: &model.BuyerHistory{}},
		{name: "buyer_profile", missing: &model.BuyerUser{}, buyer: true},
		{name: "intents_open", missing: &model.BuyerIntent{}, buyer: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var models []any
			for _, m := range buyerSummaryTestModels() {
				if fmt.Sprintf("%T", m) != fmt.Sprintf("%T", tc.missing) {
					models = append(models, m)
				}
			}
			recorder := runBuyerSummaryWithModels(t, models, tc.buyer)
			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500; body = %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestHandleBuyerSummaryDegradesMissingBuyerProfile(t *testing.T) {
	for _, asBuyer := range []bool{false, true} {
		recorder := runBuyerSummaryWithModels(t, buyerSummaryTestModels(), asBuyer)
		if recorder.Code != http.StatusOK {
			t.Fatalf("asBuyer=%v status = %d, want 200; body = %s", asBuyer, recorder.Code, recorder.Body.String())
		}
	}
}

func buyerSummaryTestModels() []any {
	return []any{&model.Merchant{}, &model.Product{}, &model.BuyerFavorite{}, &model.BuyerHistory{}, &model.BuyerUser{}, &model.BuyerIntent{}}
}

func runBuyerSummaryWithModels(t *testing.T, models []any, asBuyer bool) *httptest.ResponseRecorder {
	t.Helper()
	db := newDatabaseOperationsTestDB(t)
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	merchant := model.Merchant{MerchantNo: "MSUMMARY", MerchantName: "Summary", ContactName: "A", ContactPhone: "1", ReviewStatus: model.ReviewApproved}
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	srv := &Server{DB: db}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/buyer/me/summary?merchant_no=MSUMMARY", nil)
	c.Request.Header.Set("X-Device-Id", "dev-summary-001")
	if asBuyer {
		common.SetActor(c, common.Actor{UserID: 999, UserType: model.UserTypeBuyer})
	}
	srv.handleBuyerSummary(c)
	return recorder
}
