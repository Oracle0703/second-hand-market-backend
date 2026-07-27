package app

import (
	"errors"
	"fmt"
	"testing"

	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

func TestValidateBuyerIntentStatus(t *testing.T) {
	tests := []struct {
		name   string
		status string
		open   bool
		valid  bool
	}{
		{"new open", model.IntentNew, true, true},
		{"contacted open", model.IntentContacted, true, true},
		{"closed closed", model.IntentClosed, false, true},
		{"new closed", model.IntentNew, false, false},
		{"contacted closed", model.IntentContacted, false, false},
		{"closed open", model.IntentClosed, true, false},
		{"unknown open", "BOGUS", true, false},
		{"unknown closed", "BOGUS", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBuyerIntentStatus(tt.status, tt.open)
			if tt.valid && err != nil {
				t.Fatalf("valid state error = %v", err)
			}
			if !tt.valid && !errors.Is(err, common.ErrInternal) {
				t.Fatalf("invalid state error = %v", err)
			}
		})
	}
}

func TestValidateBuyerIntentFindOpenBuyerIntent(t *testing.T) {
	tests := []struct {
		name      string
		rows      []buyerIntentFixtureRow
		dropIndex bool
		wantFound bool
		wantErr   error
	}{
		{name: "zero rows"},
		{
			name: "multiple valid closed histories",
			rows: []buyerIntentFixtureRow{
				{ID: 1, IntentNo: "BI-C1", BuyerID: 10, ProductID: 20, Status: model.IntentClosed, IsOpen: false},
				{ID: 2, IntentNo: "BI-C2", BuyerID: 10, ProductID: 20, Status: model.IntentClosed, IsOpen: false},
			},
		},
		{
			name: "closed histories plus one open row",
			rows: []buyerIntentFixtureRow{
				{ID: 1, IntentNo: "BI-C1", BuyerID: 10, ProductID: 20, Status: model.IntentClosed, IsOpen: false},
				{ID: 2, IntentNo: "BI-C2", BuyerID: 10, ProductID: 20, Status: model.IntentClosed, IsOpen: false},
				{ID: 3, IntentNo: "BI-O1", BuyerID: 10, ProductID: 20, Status: model.IntentContacted, IsOpen: true},
			},
			wantFound: true,
		},
		{
			name:    "new marked closed",
			rows:    []buyerIntentFixtureRow{{ID: 1, IntentNo: "BI-B1", BuyerID: 10, ProductID: 20, Status: model.IntentNew, IsOpen: false}},
			wantErr: common.ErrInternal,
		},
		{
			name:    "contacted marked closed",
			rows:    []buyerIntentFixtureRow{{ID: 1, IntentNo: "BI-B1", BuyerID: 10, ProductID: 20, Status: model.IntentContacted, IsOpen: false}},
			wantErr: common.ErrInternal,
		},
		{
			name:    "closed marked open",
			rows:    []buyerIntentFixtureRow{{ID: 1, IntentNo: "BI-B1", BuyerID: 10, ProductID: 20, Status: model.IntentClosed, IsOpen: true}},
			wantErr: common.ErrInternal,
		},
		{
			name:    "unknown marked open",
			rows:    []buyerIntentFixtureRow{{ID: 1, IntentNo: "BI-B1", BuyerID: 10, ProductID: 20, Status: "BOGUS", IsOpen: true}},
			wantErr: common.ErrInternal,
		},
		{
			name:    "unknown marked closed",
			rows:    []buyerIntentFixtureRow{{ID: 1, IntentNo: "BI-B1", BuyerID: 10, ProductID: 20, Status: "BOGUS", IsOpen: false}},
			wantErr: common.ErrInternal,
		},
		{
			name: "two open rows after constraint drift",
			rows: []buyerIntentFixtureRow{
				{ID: 1, IntentNo: "BI-O1", BuyerID: 10, ProductID: 20, Status: model.IntentNew, IsOpen: true},
				{ID: 2, IntentNo: "BI-O2", BuyerID: 10, ProductID: 20, Status: model.IntentContacted, IsOpen: true},
			},
			dropIndex: true,
			wantErr:   common.ErrInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openBuyerIntentSchemaTestDB(t)
			indexSQL := []string{testBuyerIntentOpenIndexSQL}
			if tt.dropIndex {
				indexSQL = nil
			}
			createBuyerIntentSQLiteFixture(t, db, indexSQL, tt.rows)

			found, err := findOpenBuyerIntent(db, 10, 20)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("find error = %v, want %v", err, tt.wantErr)
			}
			if found != tt.wantFound {
				t.Fatalf("found = %v, want %v", found, tt.wantFound)
			}
		})
	}
}

func TestClassifyBuyerIntentCreateError(t *testing.T) {
	tests := []struct {
		name      string
		createErr error
		rows      []buyerIntentFixtureRow
		dropIndex bool
		closeDB   bool
		wantErr   error
	}{
		{
			name:      "non-duplicate create error",
			createErr: errors.New("unique wording is not a translated duplicate"),
			wantErr:   common.ErrInternal,
		},
		{
			name:      "duplicated key plus valid new open winner",
			createErr: fmt.Errorf("create failed: %w", gorm.ErrDuplicatedKey),
			rows:      []buyerIntentFixtureRow{{ID: 1, IntentNo: "BI-O1", BuyerID: 10, ProductID: 20, Status: model.IntentNew, IsOpen: true}},
			wantErr:   common.ErrConflict,
		},
		{
			name:      "duplicated key plus valid contacted open winner",
			createErr: gorm.ErrDuplicatedKey,
			rows:      []buyerIntentFixtureRow{{ID: 1, IntentNo: "BI-O1", BuyerID: 10, ProductID: 20, Status: model.IntentContacted, IsOpen: true}},
			wantErr:   common.ErrConflict,
		},
		{
			name:      "duplicated key plus no open row",
			createErr: gorm.ErrDuplicatedKey,
			wantErr:   common.ErrInternal,
		},
		{
			name:      "duplicated key plus malformed open row",
			createErr: gorm.ErrDuplicatedKey,
			rows:      []buyerIntentFixtureRow{{ID: 1, IntentNo: "BI-B1", BuyerID: 10, ProductID: 20, Status: model.IntentClosed, IsOpen: true}},
			wantErr:   common.ErrInternal,
		},
		{
			name:      "duplicated key plus malformed closed-flag history",
			createErr: gorm.ErrDuplicatedKey,
			rows: []buyerIntentFixtureRow{
				{ID: 1, IntentNo: "BI-B1", BuyerID: 10, ProductID: 20, Status: model.IntentNew, IsOpen: false},
				{ID: 2, IntentNo: "BI-O1", BuyerID: 10, ProductID: 20, Status: model.IntentContacted, IsOpen: true},
			},
			wantErr: common.ErrInternal,
		},
		{
			name:      "duplicated key plus closed history only",
			createErr: gorm.ErrDuplicatedKey,
			rows: []buyerIntentFixtureRow{
				{ID: 1, IntentNo: "BI-C1", BuyerID: 10, ProductID: 20, Status: model.IntentClosed, IsOpen: false},
				{ID: 2, IntentNo: "BI-C2", BuyerID: 10, ProductID: 20, Status: model.IntentClosed, IsOpen: false},
			},
			wantErr: common.ErrInternal,
		},
		{
			name:      "duplicated key plus multiple open rows",
			createErr: gorm.ErrDuplicatedKey,
			rows: []buyerIntentFixtureRow{
				{ID: 1, IntentNo: "BI-O1", BuyerID: 10, ProductID: 20, Status: model.IntentNew, IsOpen: true},
				{ID: 2, IntentNo: "BI-O2", BuyerID: 10, ProductID: 20, Status: model.IntentContacted, IsOpen: true},
			},
			dropIndex: true,
			wantErr:   common.ErrInternal,
		},
		{
			name:      "duplicated key plus re-query failure",
			createErr: gorm.ErrDuplicatedKey,
			closeDB:   true,
			wantErr:   common.ErrInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openBuyerIntentSchemaTestDB(t)
			indexSQL := []string{testBuyerIntentOpenIndexSQL}
			if tt.dropIndex {
				indexSQL = nil
			}
			createBuyerIntentSQLiteFixture(t, db, indexSQL, tt.rows)
			if tt.closeDB {
				sqlDB, err := db.DB()
				if err != nil {
					t.Fatalf("get SQL pool: %v", err)
				}
				if err := sqlDB.Close(); err != nil {
					t.Fatalf("close SQL pool: %v", err)
				}
			}

			err := classifyBuyerIntentCreateError(db, tt.createErr, 10, 20)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("classification error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
