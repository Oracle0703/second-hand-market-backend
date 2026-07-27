package app

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

func TestBuyerIntentTransitionCompareAndSetRejectsStaleConcurrentWrite(t *testing.T) {
	db := openBuyerIntentSchemaTestDB(t)
	if err := db.AutoMigrate(&model.BuyerIntent{}); err != nil {
		t.Fatalf("migrate buyer intent transition fixture: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get buyer intent transition pool: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	intent := model.BuyerIntent{
		IntentNo:   "BI-STALE-TRANSITION",
		BuyerID:    10,
		ProductID:  20,
		MerchantID: 30,
		Status:     model.IntentNew,
		IsOpen:     true,
	}
	if err := db.Create(&intent).Error; err != nil {
		t.Fatalf("create buyer intent transition fixture: %v", err)
	}

	staleContacted := intent
	staleClosed := intent
	start := make(chan struct{})
	type transitionResult struct {
		current model.BuyerIntent
		won     bool
		err     error
	}
	results := make(chan transitionResult, 2)
	closeCommitted := make(chan struct{})
	var ready sync.WaitGroup
	ready.Add(2)
	run := func(expected model.BuyerIntent, updates map[string]interface{}, wait <-chan struct{}, committed chan<- struct{}) {
		ready.Done()
		<-start
		if wait != nil {
			<-wait
		}
		current, won, err := compareAndSetBuyerIntentTransition(db, expected, updates)
		if committed != nil {
			close(committed)
		}
		results <- transitionResult{current: current, won: won, err: err}
	}
	go run(staleContacted, map[string]interface{}{
		"status": model.IntentContacted,
	}, closeCommitted, nil)
	go run(staleClosed, map[string]interface{}{
		"status":  model.IntentClosed,
		"is_open": false,
	}, nil, closeCommitted)
	ready.Wait()
	close(start)

	winners := 0
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatalf("compare-and-set transition: %v", result.err)
		}
		if result.won {
			winners++
		}
		if err := validateBuyerIntentState(result.current); err != nil {
			t.Fatalf("transition returned invalid state %+v: %v", result.current, err)
		}
	}
	if winners != 1 {
		t.Fatalf("transition winners = %d, want exactly 1", winners)
	}

	var persisted model.BuyerIntent
	if err := db.First(&persisted, intent.ID).Error; err != nil {
		t.Fatalf("reload buyer intent transition fixture: %v", err)
	}
	if err := validateBuyerIntentState(persisted); err != nil {
		t.Fatalf("persisted transition state %+v is invalid: %v", persisted, err)
	}
	if persisted.Status != model.IntentClosed || persisted.IsOpen {
		t.Fatalf("persisted transition state = %s/%v, want CLOSED/false", persisted.Status, persisted.IsOpen)
	}
}

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
