package tests

import (
	"database/sql"
	"testing"

	"gorm.io/gorm"
	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

func TestMerchantLoginRejectsNonActiveAccount(t *testing.T) {
	srv := newTestServer(t)
	_, username, password := registerMerchant(t, srv, "login_status")
	if err := srv.DB.Model(&model.MerchantAccount{}).Where("username = ?", username).Update("status", "LOCKED").Error; err != nil {
		t.Fatal(err)
	}
	if login := merchantLogin(t, srv, username, password); login.Code != common.ErrUnauthorized.Code {
		t.Fatalf("non-active account login code=%d", login.Code)
	}
	if err := srv.DB.Model(&model.MerchantAccount{}).Where("username = ?", username).Update("status", model.AccountStatusDisabled).Error; err != nil {
		t.Fatal(err)
	}
	if login := merchantLogin(t, srv, username, password); login.Code != common.ErrAccountDisabled.Code {
		t.Fatalf("disabled account login code=%d", login.Code)
	}
}

func TestMerchantLoginLocksAccountAndIssuesSessionInSameTransaction(t *testing.T) {
	srv := newTestServer(t)
	_, username, password := registerMerchant(t, srv, "login_lock")
	var lockedTx, sessionTx gorm.ConnPool
	query := "test:merchant_login_lock"
	if err := srv.DB.Callback().Query().Before("gorm:query").Register(query, func(tx *gorm.DB) {
		if tx.Statement.Schema == nil || tx.Statement.Schema.Name != "MerchantAccount" {
			return
		}
		if _, locked := tx.Statement.Clauses["FOR"]; locked {
			lockedTx = tx.Statement.ConnPool
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer srv.DB.Callback().Query().Remove(query)
	create := "test:merchant_login_session"
	if err := srv.DB.Callback().Create().Before("gorm:create").Register(create, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "AuthSession" {
			sessionTx = tx.Statement.ConnPool
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer srv.DB.Callback().Create().Remove(create)
	if login := merchantLogin(t, srv, username, password); login.Code != 0 {
		t.Fatalf("login failed: %+v", login)
	}
	if _, ok := lockedTx.(*sql.Tx); !ok {
		t.Fatal("merchant account was not read with a row lock inside a transaction")
	}
	if sessionTx != lockedTx {
		t.Fatal("session was not created in the transaction holding the account lock")
	}
}
