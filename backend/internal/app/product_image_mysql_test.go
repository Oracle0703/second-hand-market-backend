package app

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

func TestP1MySQLProductImageBindingSerializesWithDeletion(t *testing.T) {
	for _, first := range []string{"bind", "delete"} {
		t.Run(first+"_first", func(t *testing.T) {
			s := newP1MySQLServer(t)
			// FileRecord has an explicit table name. Use this connection's prefixed
			// fixture table, as for the other models, before starting either request.
			statement := &gorm.Statement{DB: s.DB}
			if err := statement.Parse(&model.FileRecord{}); err != nil {
				t.Fatal(err)
			}
			statement.Schema.Table = s.DB.NamingStrategy.TableName("File")
			tables := []interface{}{&model.FileRecord{}, &model.ProductImage{}, &model.Order{}, &model.BuyerIntent{}, &model.MerchantAccount{}}
			if err := s.DB.Set("gorm:table_options", "ENGINE=InnoDB").AutoMigrate(tables...); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { s.DB.Migrator().DropTable(tables...) })
			s.cfg.FileStorageProvider = "local"
			s.cfg.FileUploadLocalDir = t.TempDir()
			uploader := model.MerchantAccount{ID: 42, MerchantID: 7}
			file := model.FileRecord{BizType: model.FileBizProductImage, ObjectKey: "image.jpg", ScanStatus: model.FileScanPass, UploaderType: model.UserTypeMerchant, UploaderID: &uploader.ID}
			for _, row := range []interface{}{&uploader, &file} {
				if err := s.DB.Create(row).Error; err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(s.cfg.FileUploadLocalDir, file.ObjectKey)
			if err := os.WriteFile(path, []byte("retained product image"), 0600); err != nil {
				t.Fatal(err)
			}
			source := model.Product{ProductNo: "source", MerchantID: 7, Status: model.ProductDraft, CoverFileID: &file.ID}
			target := model.Product{ProductNo: "target", MerchantID: 7, Status: model.ProductDraft}
			for _, row := range []interface{}{&source, &target} {
				if err := s.DB.Create(row).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := s.DB.Create(&model.ProductImage{ProductID: source.ID, FileID: file.ID, SortOrder: 1}).Error; err != nil {
				t.Fatal(err)
			}
			router := p1Router(s)
			router.DELETE("/products/:id", s.handleDeleteProduct)
			bind := func() p1Response {
				return p1Request(router, http.MethodPatch, fmt.Sprintf("/products/%d", target.ID), fmt.Sprintf(`{"image_file_ids":[%d]}`, file.ID), "")
			}
			remove := func() p1Response {
				return p1Request(router, http.MethodDelete, fmt.Sprintf("/products/%d", source.ID), "", "")
			}

			firstLocked, secondEntered, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			var starts atomic.Int32
			queries := s.DB.Callback().Query()
			if err := queries.Before("gorm:query").Register("p1:image-start", func(tx *gorm.DB) {
				if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "FileRecord" && starts.Add(1) == 2 {
					close(secondEntered)
				}
			}); err != nil {
				t.Fatal(err)
			}
			var held atomic.Bool
			if err := queries.After("gorm:query").Register("p1:image-hold", func(tx *gorm.DB) {
				if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "FileRecord" && held.CompareAndSwap(false, true) {
					close(firstLocked)
					<-release
				}
			}); err != nil {
				t.Fatal(err)
			}
			firstCall, secondCall := bind, remove
			wantSecond := 0
			if first == "delete" {
				firstCall, secondCall = remove, bind
				wantSecond = common.CodeInvalidUpload
			}
			firstResult, secondResult := make(chan p1Response, 1), make(chan p1Response, 1)
			go func() { firstResult <- firstCall() }()
			select {
			case <-firstLocked:
			case <-time.After(10 * time.Second):
				t.Fatal("first request did not read the file")
			}
			go func() { secondResult <- secondCall() }()
			select {
			case <-secondEntered:
			case <-time.After(10 * time.Second):
				t.Fatal("second request did not reach the file lookup")
			}
			select {
			case result := <-secondResult:
				t.Fatalf("second request bypassed the file lock: %+v", result)
			case <-time.After(150 * time.Millisecond):
			}
			unblock()
			for _, check := range []struct {
				result chan p1Response
				code   int
			}{{firstResult, 0}, {secondResult, wantSecond}} {
				select {
				case res := <-check.result:
					if res.Code != check.code {
						t.Fatalf("result = %+v, want code %d", res, check.code)
					}
				case <-time.After(10 * time.Second):
					t.Fatal("request did not finish")
				}
			}

			var fileCount, galleryCount int64
			if err := s.DB.Model(&model.FileRecord{}).Where("id = ?", file.ID).Count(&fileCount).Error; err != nil {
				t.Fatal(err)
			}
			if err := s.DB.Model(&model.ProductImage{}).Where("product_id = ? AND file_id = ?", target.ID, file.ID).Count(&galleryCount).Error; err != nil {
				t.Fatal(err)
			}
			if err := s.DB.First(&target, target.ID).Error; err != nil {
				t.Fatal(err)
			}
			_, statErr := os.Stat(path)
			if first == "bind" {
				if fileCount != 1 || galleryCount != 1 || target.CoverFileID == nil || *target.CoverFileID != file.ID || statErr != nil {
					t.Fatalf("shared image lost: file=%d gallery=%d cover=%v stat=%v", fileCount, galleryCount, target.CoverFileID, statErr)
				}
			} else if fileCount != 0 || galleryCount != 0 || target.CoverFileID != nil || !os.IsNotExist(statErr) {
				t.Fatalf("deleted image partially retained: file=%d gallery=%d cover=%v stat=%v", fileCount, galleryCount, target.CoverFileID, statErr)
			}
		})
	}
}
