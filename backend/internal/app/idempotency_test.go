package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

type idempotencyTestEffect struct {
	ID    uint64 `gorm:"primaryKey"`
	Scope string `gorm:"size:64;uniqueIndex"`
}

type idempotencyTestDuplicate struct {
	ID   uint64 `gorm:"primaryKey"`
	Name string `gorm:"size:64;uniqueIndex"`
}

var errForcedFinalize = errors.New("forced idempotency finalize failure")

func newIdempotencyTestServer(t *testing.T) *Server {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=busy_timeout(5000)", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger:         logger.Default.LogMode(logger.Silent),
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("open idempotency test database: %v", err)
	}
	if err := db.AutoMigrate(&model.IdempotencyRecord{}, &idempotencyTestEffect{}, &idempotencyTestDuplicate{}); err != nil {
		t.Fatalf("migrate idempotency test database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get idempotency test sql database: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return &Server{DB: db}
}

func invokeIdempotencyTest(t *testing.T, server *Server, key string, payload interface{}, fn idempotentOperation) (map[string]interface{}, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	var data map[string]interface{}
	var runErr error
	router.POST("/idempotency/test", func(c *gin.Context) {
		common.SetActor(c, common.Actor{UserID: 42, UserType: model.UserTypeMerchant})
		data, runErr = server.runWithIdempotency(c, payload, fn)
	})
	req := httptest.NewRequest(http.MethodPost, "/idempotency/test", nil)
	req.Header.Set("Idempotency-Key", key)
	router.ServeHTTP(httptest.NewRecorder(), req)
	return data, runErr
}

func createIdempotencyTestEffect(tx *gorm.DB, scope string) error {
	return tx.Create(&idempotencyTestEffect{Scope: scope}).Error
}

func countIdempotencyTestRows(t *testing.T, db *gorm.DB, model interface{}) int64 {
	t.Helper()
	var count int64
	if err := db.Model(model).Count(&count).Error; err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return count
}

func TestRunWithIdempotencyCommitsAndReplaysSuccessfulObject(t *testing.T) {
	server := newIdempotencyTestServer(t)
	calls := 0
	fn := func(tx *gorm.DB) (map[string]interface{}, error) {
		calls++
		if err := createIdempotencyTestEffect(tx, "success"); err != nil {
			return nil, err
		}
		return map[string]interface{}{"effect": "success"}, nil
	}

	data, err := invokeIdempotencyTest(t, server, "success-key", map[string]string{"request": "success"}, fn)
	if err != nil || data["effect"] != "success" || data["idempotent"] != nil {
		t.Fatalf("first result = %#v, %v", data, err)
	}
	data, err = invokeIdempotencyTest(t, server, "success-key", map[string]string{"request": "success"}, fn)
	if err != nil || data["effect"] != "success" || data["idempotent"] != true {
		t.Fatalf("replay result = %#v, %v", data, err)
	}
	if calls != 1 || countIdempotencyTestRows(t, server.DB, &idempotencyTestEffect{}) != 1 || countIdempotencyTestRows(t, server.DB, &model.IdempotencyRecord{}) != 1 {
		t.Fatalf("calls=%d effects=%d claims=%d", calls, countIdempotencyTestRows(t, server.DB, &idempotencyTestEffect{}), countIdempotencyTestRows(t, server.DB, &model.IdempotencyRecord{}))
	}
}

func TestRunWithIdempotencyRejectsDifferentHashAfterSuccess(t *testing.T) {
	server := newIdempotencyTestServer(t)
	fn := func(tx *gorm.DB) (map[string]interface{}, error) {
		if err := createIdempotencyTestEffect(tx, "hash"); err != nil {
			return nil, err
		}
		return map[string]interface{}{"effect": "hash"}, nil
	}
	if _, err := invokeIdempotencyTest(t, server, "hash-key", map[string]string{"request": "one"}, fn); err != nil {
		t.Fatalf("initial call: %v", err)
	}
	if _, err := invokeIdempotencyTest(t, server, "hash-key", map[string]string{"request": "two"}, fn); !errors.Is(err, common.ErrDuplicateSubmit) {
		t.Fatalf("different payload error = %v", err)
	}
	if countIdempotencyTestRows(t, server.DB, &idempotencyTestEffect{}) != 1 || countIdempotencyTestRows(t, server.DB, &model.IdempotencyRecord{}) != 1 {
		t.Fatal("different payload changed committed state")
	}
}

func TestRunWithIdempotencyRollsBackCallbackAndClaimThenAllowsRetry(t *testing.T) {
	server := newIdempotencyTestServer(t)
	failing := func(tx *gorm.DB) (map[string]interface{}, error) {
		if err := createIdempotencyTestEffect(tx, "retry"); err != nil {
			return nil, err
		}
		return nil, common.ErrInvalidArgument
	}
	if _, err := invokeIdempotencyTest(t, server, "retry-key", map[string]string{"request": "retry"}, failing); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("callback error = %v", err)
	}
	if countIdempotencyTestRows(t, server.DB, &idempotencyTestEffect{}) != 0 || countIdempotencyTestRows(t, server.DB, &model.IdempotencyRecord{}) != 0 {
		t.Fatal("failed callback committed effect or claim")
	}
	success := func(tx *gorm.DB) (map[string]interface{}, error) {
		if err := createIdempotencyTestEffect(tx, "retry"); err != nil {
			return nil, err
		}
		return map[string]interface{}{"effect": "retry"}, nil
	}
	if _, err := invokeIdempotencyTest(t, server, "retry-key", map[string]string{"request": "retry"}, success); err != nil {
		t.Fatalf("retry error = %v", err)
	}
	if countIdempotencyTestRows(t, server.DB, &idempotencyTestEffect{}) != 1 || countIdempotencyTestRows(t, server.DB, &model.IdempotencyRecord{}) != 1 {
		t.Fatal("successful retry did not commit exactly once")
	}
}

func TestRunWithIdempotencyAcceptsEmptyObjectAndRejectsNilObject(t *testing.T) {
	server := newIdempotencyTestServer(t)
	empty := func(*gorm.DB) (map[string]interface{}, error) { return map[string]interface{}{}, nil }
	data, err := invokeIdempotencyTest(t, server, "empty-key", map[string]string{"request": "empty"}, empty)
	if err != nil || data == nil || len(data) != 0 {
		t.Fatalf("empty object result = %#v, %v", data, err)
	}
	nilObject := func(*gorm.DB) (map[string]interface{}, error) { return nil, nil }
	if _, err := invokeIdempotencyTest(t, server, "nil-key", map[string]string{"request": "nil"}, nilObject); !errors.Is(err, common.ErrInternal) {
		t.Fatalf("nil object error = %v", err)
	}
	if countIdempotencyTestRows(t, server.DB, &model.IdempotencyRecord{}) != 1 {
		t.Fatal("nil object persisted an idempotency claim")
	}
}

func TestRunWithIdempotencyRollsBackUnsupportedResponseValue(t *testing.T) {
	server := newIdempotencyTestServer(t)
	fn := func(tx *gorm.DB) (map[string]interface{}, error) {
		if err := createIdempotencyTestEffect(tx, "unsupported"); err != nil {
			return nil, err
		}
		return map[string]interface{}{"unsupported": func() {}}, nil
	}
	if _, err := invokeIdempotencyTest(t, server, "unsupported-key", map[string]string{"request": "unsupported"}, fn); !errors.Is(err, common.ErrInternal) {
		t.Fatalf("unsupported response error = %v", err)
	}
	if countIdempotencyTestRows(t, server.DB, &idempotencyTestEffect{}) != 0 || countIdempotencyTestRows(t, server.DB, &model.IdempotencyRecord{}) != 0 {
		t.Fatal("unsupported response committed effect or claim")
	}
}

func TestRunWithIdempotencyRollsBackWhenTerminalUpdateFails(t *testing.T) {
	server := newIdempotencyTestServer(t)
	if err := server.DB.Callback().Update().Before("gorm:update").Register("idempotency-test:fail-finalize", func(tx *gorm.DB) {
		if tx.Statement.Table == "idempotency_records" {
			tx.AddError(errForcedFinalize)
		}
	}); err != nil {
		t.Fatalf("register finalize failure callback: %v", err)
	}
	fn := func(tx *gorm.DB) (map[string]interface{}, error) {
		if err := createIdempotencyTestEffect(tx, "finalize"); err != nil {
			return nil, err
		}
		return map[string]interface{}{"effect": "finalize"}, nil
	}
	if _, err := invokeIdempotencyTest(t, server, "finalize-key", map[string]string{"request": "finalize"}, fn); !errors.Is(err, common.ErrInternal) {
		t.Fatalf("terminal update error = %v", err)
	}
	if countIdempotencyTestRows(t, server.DB, &idempotencyTestEffect{}) != 0 || countIdempotencyTestRows(t, server.DB, &model.IdempotencyRecord{}) != 0 {
		t.Fatal("terminal update failure committed effect or claim")
	}
}

func TestRunWithIdempotencyDoesNotTreatBusinessDuplicateAsClaimConflict(t *testing.T) {
	server := newIdempotencyTestServer(t)
	if err := server.DB.Create(&idempotencyTestDuplicate{Name: "duplicate"}).Error; err != nil {
		t.Fatalf("seed duplicate: %v", err)
	}
	fn := func(tx *gorm.DB) (map[string]interface{}, error) {
		return nil, tx.Create(&idempotencyTestDuplicate{Name: "duplicate"}).Error
	}
	if _, err := invokeIdempotencyTest(t, server, "business-duplicate-key", map[string]string{"request": "duplicate"}, fn); !errors.Is(err, gorm.ErrDuplicatedKey) {
		t.Fatalf("business duplicate error = %v", err)
	}
	if countIdempotencyTestRows(t, server.DB, &idempotencyTestDuplicate{}) != 1 || countIdempotencyTestRows(t, server.DB, &model.IdempotencyRecord{}) != 0 {
		t.Fatal("business duplicate was recorded as an idempotency replay")
	}
}

func TestRunWithIdempotencyWithoutKeyIsTransactional(t *testing.T) {
	server := newIdempotencyTestServer(t)
	success := func(tx *gorm.DB) (map[string]interface{}, error) {
		if err := createIdempotencyTestEffect(tx, "without-key-success"); err != nil {
			return nil, err
		}
		return map[string]interface{}{"effect": "without-key-success"}, nil
	}
	if _, err := invokeIdempotencyTest(t, server, "", map[string]string{"request": "without-key-success"}, success); err != nil {
		t.Fatalf("no-key success error = %v", err)
	}
	failing := func(tx *gorm.DB) (map[string]interface{}, error) {
		if err := createIdempotencyTestEffect(tx, "without-key-failure"); err != nil {
			return nil, err
		}
		return nil, common.ErrInvalidArgument
	}
	if _, err := invokeIdempotencyTest(t, server, "", map[string]string{"request": "without-key-failure"}, failing); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("no-key failure error = %v", err)
	}
	if countIdempotencyTestRows(t, server.DB, &idempotencyTestEffect{}) != 1 || countIdempotencyTestRows(t, server.DB, &model.IdempotencyRecord{}) != 0 {
		t.Fatal("no-key calls did not use a single transaction without idempotency records")
	}
}

func TestRunWithIdempotencyFailsClosedForNullOrCorruptStoredResponse(t *testing.T) {
	cases := []struct {
		name        string
		responseRaw string
		resultCode  int
	}{
		{name: "null", responseRaw: "null", resultCode: common.CodeOK},
		{name: "corrupt", responseRaw: "{", resultCode: common.CodeOK},
		{name: "array", responseRaw: "[]", resultCode: common.CodeOK},
		{name: "non-success", responseRaw: `{"effect":"stored"}`, resultCode: common.CodeConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := newIdempotencyTestServer(t)
			payload := map[string]string{"request": tc.name}
			raw, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshal payload: %v", err)
			}
			if err := server.DB.Create(&model.IdempotencyRecord{
				IdemKey: "stored-key", OperatorID: 42, Path: "/idempotency/test",
				RequestHash: common.SHA256(string(raw)), ResultCode: tc.resultCode,
				ResponseRaw: []byte(tc.responseRaw),
			}).Error; err != nil {
				t.Fatalf("seed stored response: %v", err)
			}
			called := false
			fn := func(*gorm.DB) (map[string]interface{}, error) {
				called = true
				return map[string]interface{}{"effect": "new"}, nil
			}
			if _, err := invokeIdempotencyTest(t, server, "stored-key", payload, fn); !errors.Is(err, common.ErrInternal) {
				t.Fatalf("stored response error = %v", err)
			}
			if called || countIdempotencyTestRows(t, server.DB, &model.IdempotencyRecord{}) != 1 {
				t.Fatal("invalid stored response was executed or changed")
			}
		})
	}
}

func TestRunWithIdempotencyRetryConvergesForBothCommitOutcomes(t *testing.T) {
	t.Run("committed transaction replays", func(t *testing.T) {
		server := newIdempotencyTestServer(t)
		calls := 0
		fn := func(tx *gorm.DB) (map[string]interface{}, error) {
			calls++
			if err := createIdempotencyTestEffect(tx, "committed"); err != nil {
				return nil, err
			}
			return map[string]interface{}{"outcome": "committed"}, nil
		}
		for i := 0; i < 2; i++ {
			data, err := invokeIdempotencyTest(t, server, "committed-key", map[string]string{"request": "committed"}, fn)
			if err != nil || data["outcome"] != "committed" {
				t.Fatalf("attempt %d result = %#v, %v", i, data, err)
			}
		}
		if calls != 1 || countIdempotencyTestRows(t, server.DB, &idempotencyTestEffect{}) != 1 {
			t.Fatal("retry after commit did not converge to the stored result")
		}
	})
	t.Run("rolled back transaction reruns", func(t *testing.T) {
		server := newIdempotencyTestServer(t)
		fail := func(tx *gorm.DB) (map[string]interface{}, error) {
			if err := createIdempotencyTestEffect(tx, "rolled-back"); err != nil {
				return nil, err
			}
			return nil, common.ErrInvalidArgument
		}
		if _, err := invokeIdempotencyTest(t, server, "rolled-back-key", map[string]string{"request": "rolled-back"}, fail); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("initial rolled-back call error = %v", err)
		}
		succeed := func(tx *gorm.DB) (map[string]interface{}, error) {
			if err := createIdempotencyTestEffect(tx, "rolled-back"); err != nil {
				return nil, err
			}
			return map[string]interface{}{"outcome": "rerun"}, nil
		}
		data, err := invokeIdempotencyTest(t, server, "rolled-back-key", map[string]string{"request": "rolled-back"}, succeed)
		if err != nil || data["outcome"] != "rerun" || countIdempotencyTestRows(t, server.DB, &idempotencyTestEffect{}) != 1 {
			t.Fatalf("retry after rollback result = %#v, %v", data, err)
		}
	})
}
