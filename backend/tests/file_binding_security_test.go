package tests

import (
	"net/http"
	"testing"
	"time"

	"second-hand-market-backend/backend/internal/app"
	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

func registrationPayload(username string, fileID uint64, rawToken string) map[string]interface{} {
	return map[string]interface{}{
		"merchant_name":      "Claim Store",
		"contact_name":       "Owner",
		"phone":              "13800138000",
		"username":           username,
		"password":           "Passw0rd!2026",
		"license_file_id":    fileID,
		"license_file_token": rawToken,
	}
}

func countRegistrationRows(t *testing.T, srv *app.Server) (int64, int64, int64) {
	t.Helper()
	var merchants int64
	var accounts int64
	var audits int64
	if err := srv.DB.Model(&model.Merchant{}).Count(&merchants).Error; err != nil {
		t.Fatalf("count merchants: %v", err)
	}
	if err := srv.DB.Model(&model.MerchantAccount{}).Count(&accounts).Error; err != nil {
		t.Fatalf("count accounts: %v", err)
	}
	if err := srv.DB.Model(&model.MerchantAuditLog{}).Count(&audits).Error; err != nil {
		t.Fatalf("count audits: %v", err)
	}
	return merchants, accounts, audits
}

func TestRegisterClaimsUploadedLicense(t *testing.T) {
	srv := newTestServer(t)
	fileID, rawToken := uploadReadyPublicLicense(t, srv)
	resp := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/register",
		registrationPayload(uniqueUsername("claim"), fileID, rawToken), nil)
	if resp.Code != common.CodeOK {
		t.Fatalf("register: %+v", resp)
	}
	merchantID := numToUint64(resp.Data["merchant_id"])
	var file model.FileRecord
	if err := srv.DB.First(&file, fileID).Error; err != nil {
		t.Fatalf("load claimed license: %v", err)
	}
	if file.OwnerMerchantID == nil || *file.OwnerMerchantID != merchantID ||
		file.CapabilityTokenHash != nil || file.CapabilityExpiresAt != nil {
		t.Fatalf("license was not consumed atomically: %+v", file)
	}
	var merchant model.Merchant
	if err := srv.DB.First(&merchant, merchantID).Error; err != nil {
		t.Fatalf("load merchant: %v", err)
	}
	if merchant.LicenseFileID == nil || *merchant.LicenseFileID != fileID {
		t.Fatalf("merchant license = %v, want %d", merchant.LicenseFileID, fileID)
	}
}

func TestRegisterRejectsInvalidLicenseBinding(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*testing.T, *app.Server) (uint64, string)
	}{
		{name: "missing file", prepare: func(_ *testing.T, _ *app.Server) (uint64, string) {
			return 999999, "missing-file-token"
		}},
		{name: "missing token", prepare: func(t *testing.T, srv *app.Server) (uint64, string) {
			fileID, _ := uploadReadyPublicLicense(t, srv)
			return fileID, ""
		}},
		{name: "wrong token", prepare: func(t *testing.T, srv *app.Server) (uint64, string) {
			fileID, _ := uploadReadyPublicLicense(t, srv)
			return fileID, "wrong-token"
		}},
		{name: "expired token", prepare: func(t *testing.T, srv *app.Server) (uint64, string) {
			fileID, rawToken := uploadReadyPublicLicense(t, srv)
			expired := time.Now().Add(-time.Minute)
			if err := srv.DB.Model(&model.FileRecord{}).Where("id = ?", fileID).Update("capability_expires_at", expired).Error; err != nil {
				t.Fatalf("expire token: %v", err)
			}
			return fileID, rawToken
		}},
		{name: "wrong biz type", prepare: func(t *testing.T, srv *app.Server) (uint64, string) {
			fileID, rawToken := uploadReadyPublicLicense(t, srv)
			if err := srv.DB.Model(&model.FileRecord{}).Where("id = ?", fileID).Update("biz_type", model.FileBizProductImage).Error; err != nil {
				t.Fatalf("change biz type: %v", err)
			}
			return fileID, rawToken
		}},
		{name: "pending", prepare: func(t *testing.T, srv *app.Server) (uint64, string) {
			presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
				"biz_type": model.FileBizMerchantLicense, "file_name": "pending.jpg", "file_size": 22, "mime_type": "image/jpeg",
			}, nil)
			return numToUint64(presign.Data["file_id"]), str(presign.Data["file_token"])
		}},
		{name: "already owned", prepare: func(t *testing.T, srv *app.Server) (uint64, string) {
			fileID, rawToken := uploadReadyPublicLicense(t, srv)
			owner := uint64(42)
			if err := srv.DB.Model(&model.FileRecord{}).Where("id = ?", fileID).Update("owner_merchant_id", owner).Error; err != nil {
				t.Fatalf("set owner: %v", err)
			}
			return fileID, rawToken
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t)
			fileID, rawToken := tt.prepare(t, srv)
			resp := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/register",
				registrationPayload(uniqueUsername("invalid"), fileID, rawToken), nil)
			if resp.Code != common.CodeInvalidFileBinding {
				t.Fatalf("register response = %+v", resp)
			}
			merchants, accounts, audits := countRegistrationRows(t, srv)
			if merchants != 0 || accounts != 0 || audits != 0 {
				t.Fatalf("partial registration persisted: merchants=%d accounts=%d audits=%d", merchants, accounts, audits)
			}
		})
	}
}

func TestRegisterLicenseCapabilityCannotBeReplayed(t *testing.T) {
	srv := newTestServer(t)
	fileID, rawToken := uploadReadyPublicLicense(t, srv)
	first := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/register",
		registrationPayload(uniqueUsername("first"), fileID, rawToken), nil)
	if first.Code != common.CodeOK {
		t.Fatalf("first registration: %+v", first)
	}
	second := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/auth/register",
		registrationPayload(uniqueUsername("second"), fileID, rawToken), nil)
	if second.Code != common.CodeInvalidFileBinding {
		t.Fatalf("replayed registration = %+v", second)
	}
	merchants, accounts, audits := countRegistrationRows(t, srv)
	if merchants != 1 || accounts != 1 || audits != 1 {
		t.Fatalf("replay changed registration rows: merchants=%d accounts=%d audits=%d", merchants, accounts, audits)
	}
}

func TestMerchantReapplyValidatesLicenseBinding(t *testing.T) {
	tests := []struct {
		name     string
		prepare  func(*testing.T, *app.Server, uint64, uint64) model.FileRecord
		wantCode int
	}{
		{
			name: "foreign license",
			prepare: func(t *testing.T, srv *app.Server, _ uint64, merchantB uint64) model.FileRecord {
				return createReadyOwnedFile(t, srv, merchantB, model.FileBizMerchantLicense)
			},
			wantCode: common.CodeInvalidFileBinding,
		},
		{
			name: "wrong type",
			prepare: func(t *testing.T, srv *app.Server, merchantA uint64, _ uint64) model.FileRecord {
				return createReadyOwnedFile(t, srv, merchantA, model.FileBizProductImage)
			},
			wantCode: common.CodeInvalidFileBinding,
		},
		{
			name: "pending",
			prepare: func(t *testing.T, srv *app.Server, merchantA uint64, _ uint64) model.FileRecord {
				file := createReadyOwnedFile(t, srv, merchantA, model.FileBizMerchantLicense)
				if err := srv.DB.Model(&model.FileRecord{}).Where("id = ?", file.ID).Updates(map[string]interface{}{
					"scan_status": model.FileScanPending,
					"url":         "",
				}).Error; err != nil {
					t.Fatalf("make pending file: %v", err)
				}
				return file
			},
			wantCode: common.CodeInvalidFileBinding,
		},
		{
			name: "valid own license",
			prepare: func(t *testing.T, srv *app.Server, merchantA uint64, _ uint64) model.FileRecord {
				return createReadyOwnedFile(t, srv, merchantA, model.FileBizMerchantLicense)
			},
			wantCode: common.CodeOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t)
			merchantA, usernameA, passwordA := registerMerchant(t, srv, "reapply_a")
			merchantB, _, _ := registerMerchant(t, srv, "reapply_b")
			adminToken := adminAccessToken(t, srv)
			rejectMerchant(t, srv, adminToken, merchantA)
			rejectMerchant(t, srv, adminToken, merchantB)

			login := merchantLogin(t, srv, usernameA, passwordA)
			if login.Code != common.CodeOK || str(login.Data["token_scope"]) != "onboarding" {
				t.Fatalf("onboarding login: %+v", login)
			}
			var before model.Merchant
			if err := srv.DB.First(&before, merchantA).Error; err != nil {
				t.Fatalf("load merchant before reapply: %v", err)
			}
			var auditBefore int64
			if err := srv.DB.Model(&model.MerchantAuditLog{}).
				Where("merchant_id = ? AND action = ?", merchantA, "REAPPLY").
				Count(&auditBefore).Error; err != nil {
				t.Fatalf("count reapply audits: %v", err)
			}

			file := tt.prepare(t, srv, merchantA, merchantB)
			resp := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/merchant/reapply", map[string]interface{}{
				"license_file_id": file.ID,
			}, map[string]string{"Authorization": "Bearer " + str(login.Data["access_token"])})
			if resp.Code != tt.wantCode {
				t.Fatalf("reapply response = %+v", resp)
			}

			var after model.Merchant
			if err := srv.DB.First(&after, merchantA).Error; err != nil {
				t.Fatalf("load merchant after reapply: %v", err)
			}
			var auditAfter int64
			if err := srv.DB.Model(&model.MerchantAuditLog{}).
				Where("merchant_id = ? AND action = ?", merchantA, "REAPPLY").
				Count(&auditAfter).Error; err != nil {
				t.Fatalf("count reapply audits after: %v", err)
			}

			if tt.wantCode == common.CodeOK {
				if after.ReviewStatus != model.ReviewPending || after.LicenseFileID == nil || *after.LicenseFileID != file.ID {
					t.Fatalf("valid reapply not persisted: %+v", after)
				}
				if auditAfter != auditBefore+1 {
					t.Fatalf("reapply audit count = %d, want %d", auditAfter, auditBefore+1)
				}
				return
			}

			if after.ReviewStatus != model.ReviewRejected || before.LicenseFileID == nil || after.LicenseFileID == nil ||
				*after.LicenseFileID != *before.LicenseFileID {
				t.Fatalf("invalid reapply changed merchant: before=%+v after=%+v", before, after)
			}
			if auditAfter != auditBefore {
				t.Fatalf("invalid reapply added audit: before=%d after=%d", auditBefore, auditAfter)
			}
		})
	}
}
