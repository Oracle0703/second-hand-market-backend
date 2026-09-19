package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

func TestP1MySQLAdminPasswordSerializesConcurrentLogin(t *testing.T) {
	s := newP1MySQLServer(t)
	s.cfg.JWTAccessSecret = "synthetic-access-secret"
	s.cfg.JWTRefreshSecret = "synthetic-refresh-secret"
	s.cfg.AccessTTL = time.Hour
	s.cfg.RefreshTTL = 24 * time.Hour
	if err := s.DB.AutoMigrate(&model.AdminUser{}, &model.AuthSession{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DB.Migrator().DropTable(&model.AuthSession{}, &model.AdminUser{}) })
	hash, _ := bcrypt.GenerateFromPassword([]byte("BeforeChangePass123!"), bcrypt.MinCost)
	account := model.AdminUser{Username: "race_admin", DisplayName: "Race Admin", PasswordHash: string(hash), Role: model.AdminRoleAdmin, Status: model.AccountStatusActive}
	if err := s.DB.Create(&account).Error; err != nil {
		t.Fatal(err)
	}
	session := model.AuthSession{UserID: account.ID, UserType: model.UserTypeAdmin, RefreshTokenHash: "existing-hash", ExpiredAt: time.Now().Add(time.Hour)}
	if err := s.DB.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.POST("/login", s.handleLogin)
	r.PUT("/password", func(c *gin.Context) {
		common.SetActor(c, common.Actor{UserID: account.ID, UserType: model.UserTypeAdmin, SessionID: session.ID})
		s.handleAdminChangePassword(c)
	})
	loginLocked := make(chan struct{})
	changeStarted := make(chan struct{})
	release := make(chan struct{})
	var closeOnce sync.Once
	defer closeOnce.Do(func() { close(release) })
	var reads atomic.Int32
	s.DB.Callback().Query().Before("gorm:query").Register("test:password_read", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "AdminUser" && reads.Add(1) == 2 {
			close(changeStarted)
		}
	})
	s.DB.Callback().Create().Before("gorm:create").Register("test:login_session", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "AuthSession" {
			close(loginLocked)
			select {
			case <-release:
			case <-time.After(5 * time.Second):
				tx.AddError(fmt.Errorf("test barrier timeout"))
			}
		}
	})
	request := func(method, path, body string) int {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}
	loginDone := make(chan int, 1)
	changeDone := make(chan int, 1)
	go func() {
		loginDone <- request(http.MethodPost, "/login", `{"login_type":"ADMIN","username":"race_admin","password":"BeforeChangePass123!"}`)
	}()
	select {
	case <-loginLocked:
	case <-time.After(5 * time.Second):
		t.Fatal("login did not reach session creation")
	}
	go func() {
		changeDone <- request(http.MethodPut, "/password", `{"old_password":"BeforeChangePass123!","new_password":"AfterChangePass456!"}`)
	}()
	select {
	case <-changeStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("password change did not reach account lock")
	}
	select {
	case <-changeDone:
		t.Fatal("password change escaped the login's row lock")
	case <-time.After(100 * time.Millisecond):
	}
	closeOnce.Do(func() { close(release) })
	for name, ch := range map[string]<-chan int{"login": loginDone, "password": changeDone} {
		select {
		case status := <-ch:
			if status != 200 {
				t.Fatalf("%s status=%d", name, status)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s timed out", name)
		}
	}
	var active int64
	if err := s.DB.Model(&model.AuthSession{}).Where("user_type = ? AND user_id = ? AND revoked_at IS NULL", model.UserTypeAdmin, account.ID).Count(&active).Error; err != nil {
		t.Fatal(err)
	}
	if active != 0 {
		t.Fatal("old-password login created an unrevoked session")
	}
}
