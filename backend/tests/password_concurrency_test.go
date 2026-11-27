package tests

import (
	"net/http"
	"testing"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

func TestPasswordChangeCannotOverwriteConcurrentReset(t *testing.T) {
	srv := newTestServer(t)
	_, username, password := registerMerchant(t, srv, "password_race")
	login := merchantLogin(t, srv, username, password)
	if login.Code != 0 {
		t.Fatal("login failed")
	}
	replacement, err := bcrypt.GenerateFromPassword([]byte("AdminResetPass456!"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	queries := 0
	callback := "test:concurrent_password_reset"
	err = srv.DB.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		account, ok := tx.Statement.Dest.(*model.MerchantAccount)
		if !ok || account.Username != username {
			return
		}
		queries++
		if queries != 2 {
			return
		} // session guard reads first; handler reads second.
		if err := srv.DB.Model(&model.MerchantAccount{}).Where("id = ?", account.ID).UpdateColumn("password_hash", string(replacement)).Error; err != nil {
			t.Fatal(err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.DB.Callback().Query().Remove(callback)
	result := requestJSON(t, srv.Router, http.MethodPut, "/api/v1/merchant/account/password", map[string]string{"old_password": password, "new_password": "UserChangePass789!"}, map[string]string{"Authorization": "Bearer " + str(login.Data["access_token"])})
	if queries != 2 || result.HTTPStatus != http.StatusConflict || result.Code != common.CodeConflict {
		t.Fatalf("stale password update was not reported as conflict; reads=%d status=%d code=%d", queries, result.HTTPStatus, result.Code)
	}
	var account model.MerchantAccount
	if err := srv.DB.Where("username = ?", username).First(&account).Error; err != nil {
		t.Fatal(err)
	}
	if account.PasswordHash != string(replacement) {
		t.Fatal("concurrent administrator reset was overwritten")
	}
}
