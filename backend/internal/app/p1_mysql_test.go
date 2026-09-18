package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	mysqlcfg "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

// Uses only a disposable local database and uniquely prefixed test tables.
func newP1MySQLServer(t *testing.T) *Server {
	t.Helper()
	dsn := os.Getenv("P1_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set P1_MYSQL_DSN to a disposable local p1_test database")
	}
	cfg, err := mysqlcfg.ParseDSN(dsn)
	if err != nil || cfg.Net != "tcp" || (cfg.Addr != "127.0.0.1:3306" && cfg.Addr != "localhost:3306") || cfg.DBName != "p1_test" {
		t.Fatal("P1_MYSQL_DSN must target localhost:3306/p1_test")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), NamingStrategy: schema.NamingStrategy{TablePrefix: fmt.Sprintf("p1_%d_", time.Now().UnixNano())}})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(16)
	tables := []interface{}{&model.Product{}, &model.ProductStockAdjustment{}, &model.IdempotencyRecord{}, &model.OperationLog{}}
	if err := db.Set("gorm:table_options", "ENGINE=InnoDB").AutoMigrate(tables...); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, table := range tables {
			db.Migrator().DropTable(table)
		}
		sqlDB.Close()
	})
	return &Server{DB: db}
}

func p1Router(s *Server) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		common.SetActor(c, common.Actor{UserID: 42, UserType: model.UserTypeMerchant, MerchantID: 7})
	})
	router.POST("/products/:id/stock", s.handleProductStockAdjustment)
	router.PATCH("/products/:id", s.handleUpdateProduct)
	return router
}

type p1Response struct {
	Code int                    `json:"code"`
	Data map[string]interface{} `json:"data"`
}

func p1Request(router http.Handler, method, path, body, key string) p1Response {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var result p1Response
	if json.Unmarshal(rec.Body.Bytes(), &result) != nil {
		result.Code = -1
	}
	return result
}

func TestP1MySQLConcurrentStockIdempotency(t *testing.T) {
	s := newP1MySQLServer(t)
	p := model.Product{ProductNo: "P1-stock", MerchantID: 7, Status: model.ProductOnShelf, Stock: 5, Version: 1}
	if err := s.DB.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	router := p1Router(s)
	start := make(chan struct{})
	results := make(chan p1Response, 12)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- p1Request(router, http.MethodPost, fmt.Sprintf("/products/%d/stock", p.ID), `{"adjustment_type":"INCREASE","quantity":2,"reason":"stock count"}`, "same-key")
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	var movement interface{}
	for result := range results {
		if result.Code != 0 {
			t.Fatalf("request failed: %+v", result)
		}
		if movement == nil {
			movement = result.Data["movement_id"]
		}
		if result.Data["movement_id"] != movement {
			t.Fatalf("different movements: %+v", result)
		}
	}
	if err := s.DB.First(&p, p.ID).Error; err != nil {
		t.Fatal(err)
	}
	if p.Stock != 7 {
		t.Fatalf("stock = %d, want 7", p.Stock)
	}
	var count int64
	s.DB.Model(&model.ProductStockAdjustment{}).Count(&count)
	if count != 1 {
		t.Fatalf("movements = %d, want 1", count)
	}
}

func TestP1MySQLEditDoesNotOverwriteConcurrentStock(t *testing.T) {
	s := newP1MySQLServer(t)
	p := model.Product{ProductNo: "P1-edit", MerchantID: 7, Status: model.ProductOnShelf, Stock: 5, Version: 1}
	if err := s.DB.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	read := make(chan struct{})
	release := make(chan struct{})
	var released sync.Once
	defer released.Do(func() { close(release) })
	var paused atomic.Bool
	if err := s.DB.Callback().Query().After("gorm:query").Register("p1:pause-edit", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "Product" && paused.CompareAndSwap(false, true) {
			close(read)
			<-release
		}
	}); err != nil {
		t.Fatal(err)
	}
	router := p1Router(s)
	edited := make(chan p1Response, 1)
	go func() {
		edited <- p1Request(router, http.MethodPatch, fmt.Sprintf("/products/%d", p.ID), `{"description":"edited"}`, "")
	}()
	select {
	case <-read:
	case <-time.After(5 * time.Second):
		t.Fatal("edit did not read product")
	}
	adjusted := make(chan p1Response, 1)
	go func() {
		adjusted <- p1Request(router, http.MethodPost, fmt.Sprintf("/products/%d/stock", p.ID), `{"adjustment_type":"INCREASE","quantity":2,"reason":"stock count"}`, "edit-race")
	}()
	select {
	case result := <-adjusted:
		t.Fatalf("stock bypassed edit row lock: %+v", result)
	case <-time.After(150 * time.Millisecond):
	}
	released.Do(func() { close(release) })
	for _, ch := range []chan p1Response{edited, adjusted} {
		select {
		case result := <-ch:
			if result.Code != 0 {
				t.Fatalf("request failed: %+v", result)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("request blocked")
		}
	}
	if err := s.DB.First(&p, p.ID).Error; err != nil {
		t.Fatal(err)
	}
	if p.Stock != 7 || p.Description != "edited" || p.Version != 3 {
		t.Fatalf("lost update: %+v", p)
	}
}
