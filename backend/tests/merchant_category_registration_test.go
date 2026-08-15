package tests

import (
	"net/http"
	"testing"
)

func TestMerchantRegistrationCreatesDefaultCategories(t *testing.T) {
	srv := newTestServer(t)
	merchantID, username, password := registerMerchant(t, srv, "category_registration")
	approveMerchant(t, srv, adminAccessToken(t, srv), merchantID)
	login := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/login", map[string]interface{}{
		"login_type": "MERCHANT", "username": username, "password": password,
	}, nil)
	if login.Code != 0 {
		t.Fatalf("merchant login failed: %+v", login)
	}
	merchantToken := str(login.Data["access_token"])

	cats := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/merchant/categories?level=1", nil, map[string]string{"Authorization": "Bearer " + merchantToken})
	if cats.Code != 0 {
		t.Fatalf("merchant categories code = %d data=%v", cats.Code, cats.Data)
	}
	items := cats.Data["items"].([]interface{})
	if len(items) != 3 {
		t.Fatalf("registered merchant root categories = %d, want 3", len(items))
	}
	for _, item := range items {
		row := item.(map[string]interface{})
		if got := numToUint64(row["merchant_id"]); got != merchantID {
			t.Fatalf("category merchant_id = %d, want %d row=%+v", got, merchantID, row)
		}
	}
}
