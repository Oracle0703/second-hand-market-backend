# F-14 Session Access Revocation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make administrator, merchant, and buyer access tokens depend on the current database session and account state so logout, session revocation, account disablement, and merchant review changes take effect immediately.

**Architecture:** Replace the administrator-only session middleware with a global authenticated-actor gate. Each authenticated request performs one primary-key session read and one authoritative actor read; merchant account and merchant review state are loaded in one join, then current role/merchant/scope fields replace stale JWT claims. Logout uses an identity-scoped conditional update with an exact `RowsAffected` contract. No-token requests perform no authorization query, and every database or state-invariant failure fails closed.

**Tech Stack:** Go 1.22, Gin, GORM, glebarez SQLite for local tests, MySQL 8.4 for isolated acceptance, Bash, Docker Compose, Make.

## Global Constraints

- Implement only F-14. Do not implement F-10, F-11, F-12, F-15, or another open finding in these commits.
- Do not change JWT claims, signing algorithms, access/refresh TTLs, model schema, migrations, or database indexes.
- Keep the global order `OptionalAuth -> RequireActiveSession -> route middleware -> handler`.
- A no-token request remains anonymous and performs zero session/account reads.
- Every authenticated request performs exactly two authorization reads: session primary key, then actor primary key; the merchant actor read includes one merchant join.
- Session missing/expired/revoked/identity mismatch, zero `sid`, missing account, soft-deleted account, and unsupported actor type return HTTP 401 / code `10002`.
- Explicitly disabled administrator, merchant account, buyer, or merchant review state returns HTTP 403 / code `10007`.
- Query failures and unknown account status, review state, or unsupported role value return HTTP 500 / code `20001`.
- Merchant `APPROVED` maps to `scope=full`; `PENDING` and `REJECTED` map to `scope=onboarding`; `DISABLED` is account disabled.
- Current administrator/merchant role, merchant relationship, and merchant scope replace stale JWT authorization fields before route middleware runs.
- Logout updates only exact `id + user_type + user_id + revoked_at IS NULL`; one row is success, zero rows is unauthorized, database failure is internal error.
- Never add authentication logs containing tokens, session IDs, actor/account IDs, account state, or query results. Existing business audit behavior remains unchanged.
- All behavior changes use RED -> GREEN, Go changes are formatted with `gofmt`, and each task ends in a focused Conventional Commit.
- Do not modify, stage, commit, or transfer `.tmp/`, `docs/architecture-evolution-plan-2026-07-24.md`, `docs/first-round-fix-review-2026-07-24.md`, or `docs/second-round-fix-review-2026-07-24.md`.
- Do not read from or write to production databases, deploy code, restart production services, modify production files, or change production data.
- Test-server transfer/execution requires separate authorization for the exact path, Compose project, and whitelist in Task 6.

---

## File Map

| Path | Responsibility |
| --- | --- |
| `backend/internal/middleware/auth.go` | Universal active-session gate and authoritative actor loading |
| `backend/internal/middleware/auth_test.go` | Focused session/account/error/actor-rehydration middleware matrix |
| `backend/internal/app/server.go` | Replace global administrator-only middleware with universal middleware |
| `backend/internal/app/auth_handlers.go` | Exact conditional current-session revocation helper and logout handler |
| `backend/internal/app/auth_handlers_test.go` | Direct revocation predicate and `RowsAffected` contract tests |
| `backend/tests/session_revocation_test.go` | Three-actor API logout, disablement, scope downgrade, and unrelated-session regressions |
| `backend/tests/buyer_flow_test.go` | Extend existing buyer logout contract to assert old access-token rejection |
| `backend/tests/session_revocation_mysql_test.go` | Opt-in MySQL 8.4 API/concurrency/query-plan acceptance matrix |
| `backend/tests/session_revocation_acceptance_contract_test.go` | Executable fail-closed guard behavior for the acceptance harness |
| `deploy/acceptance/session-revocation-smoke.sh` | Dedicated Compose project, MySQL setup, tests, evidence sanitization, and production snapshot comparison |
| `deploy/acceptance/README.md` | Exact F-14 isolated-run procedure and prohibitions |
| `Makefile` | Guarded `acceptance-session-revocation-smoke` target |
| `docs/full-project-code-review-2026-07-24.md` | Append evidence-backed F-14 implementation/acceptance status without rewriting the finding |
| `docs/release-readiness.md` | Distinguish code-side, isolated-server, and production status |
| `docs/superpowers/reviews/2026-07-27-session-access-revocation-isolated-acceptance.md` | Sanitized test-server evidence after separate authorization and successful execution |

---

### Task 1: Require an active identity-matched session for every authenticated actor

**Files:**
- Create: `backend/internal/middleware/auth_test.go`
- Modify: `backend/internal/middleware/auth.go:40-70`
- Modify: `backend/internal/app/server.go:72`

**Interfaces:**
- Consumes: `common.Actor`, `model.AuthSession`, `common.ErrUnauthorized`, `common.ErrInternal`, and the actor set by `OptionalAuth`.
- Produces: `func RequireActiveSession(db *gorm.DB) gin.HandlerFunc` and `func requireActiveSession(db *gorm.DB, actor common.Actor, now time.Time) error` for Task 2.

- [ ] **Step 1: Create focused test database/router helpers**

Create `auth_test.go` in package `middleware` so tests can call the unexported time-injected helper. Use a unique shared in-memory database and real models:

```go
func newAuthMiddlewareDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf(
		"file:%s?mode=memory&cache=shared&_pragma=busy_timeout(5000)",
		strings.ReplaceAll(t.Name(), "/", "_"),
	)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open auth middleware database: %v", err)
	}
	if err := db.AutoMigrate(
		&model.AuthSession{}, &model.AdminUser{}, &model.Merchant{},
		&model.MerchantAccount{}, &model.BuyerUser{},
	); err != nil {
		t.Fatalf("migrate auth middleware database: %v", err)
	}
	return db
}

func authMiddlewareRouter(db *gorm.DB, actor *common.Actor) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if actor != nil {
		r.Use(func(c *gin.Context) {
			common.SetActor(c, *actor)
			c.Next()
		})
	}
	r.Use(RequireActiveSession(db))
	r.GET("/probe", func(c *gin.Context) {
		current, _ := common.GetActor(c)
		common.Success(c, gin.H{
			"user_type": current.UserType,
			"role": current.Role,
			"merchant_id": current.MerchantID,
			"scope": current.Scope,
		})
	})
	return r
}
```

Add response and request helpers that decode `common.APIResponse` without printing Authorization headers or token values.

- [ ] **Step 2: Write the RED anonymous/session-state matrix**

Add tests with fixed `now := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)`:

```go
func TestRequireActiveSessionSkipsAnonymousRequest(t *testing.T) {
	db := newAuthMiddlewareDB(t)
	w := serveAuthProbe(t, authMiddlewareRouter(db, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("anonymous request status = %d", w.Code)
	}
}

func activeTestActor() common.Actor {
	return common.Actor{
		UserType:  model.UserTypeMerchant,
		UserID:    11,
		SessionID: 22,
	}
}

func activeTestSession(now time.Time) model.AuthSession {
	return model.AuthSession{
		ID:        22,
		UserType:  model.UserTypeMerchant,
		UserID:    11,
		ExpiredAt: now.Add(time.Hour),
	}
}

func TestRequireActiveSessionAcceptsMatchingActiveSession(t *testing.T) {
	db := newAuthMiddlewareDB(t)
	now := time.Now()
	merchant := model.Merchant{
		ID: 31, MerchantNo: "F14-M-31", MerchantName: "F14 Merchant",
		ReviewStatus: model.ReviewApproved,
	}
	account := model.MerchantAccount{
		ID: 11, MerchantID: merchant.ID, Username: "f14_active_merchant",
		Role: model.AccountRoleOwner, Status: model.AccountStatusActive,
	}
	session := activeTestSession(now)
	if err := db.Create(&merchant).Error; err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	if err := db.Create(&account).Error; err != nil {
		t.Fatalf("create merchant account: %v", err)
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create active session: %v", err)
	}
	actor := activeTestActor()
	w := serveAuthProbe(t, authMiddlewareRouter(db, &actor))
	if w.Code != http.StatusOK {
		t.Fatalf("active session status = %d", w.Code)
	}
}

func TestRequireActiveSessionRejectsInvalidSessionState(t *testing.T) {
	now := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		actor  common.Actor
		mutate func(*model.AuthSession)
		omit   bool
	}{
		{name: "zero sid", actor: common.Actor{UserType: model.UserTypeMerchant, UserID: 11}},
		{name: "missing", actor: common.Actor{UserType: model.UserTypeMerchant, UserID: 11, SessionID: 999}, omit: true},
		{name: "expired", actor: activeTestActor(), mutate: func(s *model.AuthSession) { s.ExpiredAt = now }},
		{name: "revoked", actor: activeTestActor(), mutate: func(s *model.AuthSession) { s.RevokedAt = &now }},
		{name: "user mismatch", actor: activeTestActor(), mutate: func(s *model.AuthSession) { s.UserID++ }},
		{name: "type mismatch", actor: activeTestActor(), mutate: func(s *model.AuthSession) { s.UserType = model.UserTypeBuyer }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newAuthMiddlewareDB(t)
			session := activeTestSession(now)
			if tc.mutate != nil {
				tc.mutate(&session)
			}
			if !tc.omit {
				if err := db.Create(&session).Error; err != nil {
					t.Fatalf("create session: %v", err)
				}
			}
			err := requireActiveSession(db, tc.actor, now)
			if !errors.Is(err, common.ErrUnauthorized) {
				t.Fatalf("error = %v, want unauthorized", err)
			}
		})
	}
}

func TestRequireActiveSessionMapsSessionQueryFailureToInternal(t *testing.T) {
	db := newAuthMiddlewareDB(t)
	errSynthetic := errors.New("synthetic auth session query failure")
	if err := db.Callback().Query().Before("gorm:query").
		Register("test:fail_auth_session_query", func(tx *gorm.DB) {
			if tx.Statement.Table == "auth_sessions" {
				tx.AddError(errSynthetic)
			}
		}); err != nil {
		t.Fatalf("register query callback: %v", err)
	}
	err := requireActiveSession(db, activeTestActor(), time.Now())
	if !errors.Is(err, common.ErrInternal) {
		t.Fatalf("error = %v, want internal", err)
	}
}
```

- [ ] **Step 3: Run the focused test and verify RED**

Run:

```bash
cd backend
go test ./internal/middleware -run 'TestRequireActiveSession' -count=1 -v
```

Expected: FAIL to compile because `RequireActiveSession` and `requireActiveSession` do not exist.

- [ ] **Step 4: Implement the universal session gate**

Replace `RequireActiveAdminSession` with:

```go
func RequireActiveSession(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := common.GetActor(c)
		if !ok {
			c.Next()
			return
		}
		if err := requireActiveSession(db, actor, time.Now()); err != nil {
			common.Fail(c, err)
			c.Abort()
			return
		}
		c.Next()
	}
}

func requireActiveSession(db *gorm.DB, actor common.Actor, now time.Time) error {
	if actor.SessionID == 0 {
		return common.ErrUnauthorized
	}
	var session model.AuthSession
	if err := db.Select("id", "user_type", "user_id", "expired_at", "revoked_at").
		Where("id = ?", actor.SessionID).Take(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return common.ErrUnauthorized
		}
		return common.ErrInternal
	}
	if session.UserType != actor.UserType || session.UserID != actor.UserID ||
		session.RevokedAt != nil || !session.ExpiredAt.After(now) {
		return common.ErrUnauthorized
	}
	return nil
}
```

Do not log the query, actor, or session. Keep `errors` and `time` imports because both remain required.

- [ ] **Step 5: Wire the universal middleware globally**

Change only the middleware name at server construction:

```go
r.Use(
	gin.Recovery(),
	middleware.RequestID(),
	middleware.OptionalAuth(cfg.JWTAccessSecret),
	middleware.RequireActiveSession(db),
)
```

Do not move the middleware into selected route groups.

- [ ] **Step 6: Run focused and existing administrator security tests**

Run:

```bash
cd backend
gofmt -w internal/middleware/auth.go internal/middleware/auth_test.go internal/app/server.go
go test ./internal/middleware -run 'TestRequireActiveSession' -count=1 -v
go test ./tests -run 'TestAdmin(ChangePasswordRevokesOnlyTargetAdminSessions|SessionDatabaseFailureReturnsInternalError)' -count=1 -v
```

Expected: PASS. The administrator database-failure test must still return code `20001`.

- [ ] **Step 7: Commit the universal session gate**

```bash
git add backend/internal/middleware/auth.go backend/internal/middleware/auth_test.go \
  backend/internal/app/server.go
git commit -m "fix(auth): require active sessions for all actors"
```

---

### Task 2: Reload authoritative account, role, merchant relationship, and scope

**Files:**
- Modify: `backend/internal/middleware/auth.go`
- Modify: `backend/internal/middleware/auth_test.go`
- Test: `backend/tests/restricted_and_security_test.go`

**Interfaces:**
- Consumes: `requireActiveSession`, `model.AccountStatusActive`, `model.AccountStatusDisabled`, administrator and merchant role constants, buyer status constants, and merchant review constants.
- Produces: `func loadAuthoritativeActor(db *gorm.DB, actor common.Actor) (common.Actor, error)` and a refreshed `common.Actor` stored before downstream scope middleware.

- [ ] **Step 1: Write RED account-state and rehydration tests**

Seed one active row for each actor type and issue actors with intentionally stale JWT authorization fields. Require the `/probe` response to contain database values:

```go
func TestRequireActiveSessionReloadsAuthoritativeActor(t *testing.T) {
	// Admin JWT role ADMIN, database role SUPER_ADMIN -> probe SUPER_ADMIN/full/merchant_id=0.
	// Merchant JWT role STAFF, merchant_id=999, scope=onboarding;
	// database OWNER/current merchant/APPROVED -> probe OWNER/current/full.
	// Buyer JWT role stale, merchant_id=999, scope=onboarding -> probe BUYER/0/full.
}
```

Add a table matrix that requires:

```text
missing or soft-deleted admin/merchant/buyer       -> 10002
admin/merchant account/buyer DISABLED              -> 10007
merchant review DISABLED                           -> 10007
admin status/role unknown                          -> 20001
merchant account status/role/review unknown        -> 20001
buyer status unknown                               -> 20001
unsupported actor type, including PUBLIC           -> 10002
```

For an account-query failure, register a callback before `gorm:query` that checks `tx.Statement.Table` for `admin_users`, `merchant_accounts`, or `buyer_users`, adds `errors.New("synthetic account query failure")`, and require `20001` without invoking `/probe`.

- [ ] **Step 2: Write the RED merchant review downgrade integration test**

In `restricted_and_security_test.go`, add:

```go
func TestMerchantAccessTokenUsesCurrentReviewScope(t *testing.T) {
	srv := newTestServer(t)
	merchantID, username, password := registerMerchant(t, srv, "f14_scope")
	approveMerchant(t, srv, adminAccessToken(t, srv), merchantID)
	login := merchantLogin(t, srv, username, password)
	access := str(login.Data["access_token"])

	if err := srv.DB.Model(&model.Merchant{}).Where("id = ?", merchantID).
		Update("review_status", model.ReviewRejected).Error; err != nil {
		t.Fatalf("reject merchant after token issue: %v", err)
	}
	full := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/merchant/products", nil,
		map[string]string{"Authorization": "Bearer " + access})
	if full.Code != common.CodeReviewNotApproved {
		t.Fatalf("stale full token code = %d", full.Code)
	}
	profile := requestJSON(t, srv.Router, http.MethodGet, "/api/v1/merchant/profile", nil,
		map[string]string{"Authorization": "Bearer " + access})
	if profile.Code != common.CodeOK || str(profile.Data["review_status"]) != model.ReviewRejected {
		t.Fatalf("current onboarding profile contract failed")
	}
}
```

- [ ] **Step 3: Run the focused tests and verify RED**

Run:

```bash
cd backend
go test ./internal/middleware -run 'TestRequireActiveSession(Reloads|Account|RejectsUnsupported)' -count=1 -v
go test ./tests -run '^TestMerchantAccessTokenUsesCurrentReviewScope$' -count=1 -v
```

Expected: FAIL because Task 1 validates only the session and leaves stale account/scope claims untouched.

- [ ] **Step 4: Implement authoritative actor loading**

Add narrow query result types:

```go
type accountAuthorizationState struct {
	Status string
	Role   string
}

type merchantAuthorizationState struct {
	AccountStatus string `gorm:"column:account_status"`
	Role          string
	MerchantID    uint64
	ReviewStatus  string
}
```

Add a shared query error mapping:

```go
func authorizationQueryError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return common.ErrUnauthorized
	}
	return common.ErrInternal
}
```

Implement `loadAuthoritativeActor` with exact branches:

```go
func loadAuthoritativeActor(db *gorm.DB, actor common.Actor) (common.Actor, error) {
	switch actor.UserType {
	case model.UserTypeAdmin:
		var state accountAuthorizationState
		if err := db.Model(&model.AdminUser{}).Select("status", "role").
			Where("id = ?", actor.UserID).Take(&state).Error; err != nil {
			return common.Actor{}, authorizationQueryError(err)
		}
		if state.Status == model.AccountStatusDisabled {
			return common.Actor{}, common.ErrAccountDisabled
		}
		if state.Status != model.AccountStatusActive ||
			(state.Role != model.AdminRoleSuper && state.Role != model.AdminRoleAdmin) {
			return common.Actor{}, common.ErrInternal
		}
		actor.Role, actor.MerchantID, actor.Scope = state.Role, 0, "full"
		return actor, nil

	case model.UserTypeMerchant:
		var state merchantAuthorizationState
		err := db.Table("merchant_accounts AS account").
			Select("account.status AS account_status, account.role, account.merchant_id, merchant.review_status").
			Joins("JOIN merchants AS merchant ON merchant.id = account.merchant_id AND merchant.deleted_at IS NULL").
			Where("account.id = ? AND account.deleted_at IS NULL", actor.UserID).
			Take(&state).Error
		if err != nil {
			return common.Actor{}, authorizationQueryError(err)
		}
		if state.AccountStatus == model.AccountStatusDisabled || state.ReviewStatus == model.ReviewDisabled {
			return common.Actor{}, common.ErrAccountDisabled
		}
		if state.AccountStatus != model.AccountStatusActive ||
			(state.Role != model.AccountRoleOwner && state.Role != model.AccountRoleStaff) {
			return common.Actor{}, common.ErrInternal
		}
		switch state.ReviewStatus {
		case model.ReviewApproved:
			actor.Scope = "full"
		case model.ReviewPending, model.ReviewRejected:
			actor.Scope = "onboarding"
		default:
			return common.Actor{}, common.ErrInternal
		}
		actor.Role, actor.MerchantID = state.Role, state.MerchantID
		return actor, nil

	case model.UserTypeBuyer:
		var state struct{ Status string }
		if err := db.Model(&model.BuyerUser{}).Select("status").
			Where("id = ?", actor.UserID).Take(&state).Error; err != nil {
			return common.Actor{}, authorizationQueryError(err)
		}
		if state.Status == model.BuyerStatusDisabled {
			return common.Actor{}, common.ErrAccountDisabled
		}
		if state.Status != model.BuyerStatusActive {
			return common.Actor{}, common.ErrInternal
		}
		actor.Role, actor.MerchantID, actor.Scope = model.UserTypeBuyer, 0, "full"
		return actor, nil
	default:
		return common.Actor{}, common.ErrUnauthorized
	}
}
```

After `requireActiveSession` succeeds, call the loader and replace the context actor:

```go
current, err := loadAuthoritativeActor(db, actor)
if err != nil {
	common.Fail(c, err)
	c.Abort()
	return
}
common.SetActor(c, current)
c.Next()
```

- [ ] **Step 5: Add exact authorization-query-count coverage**

Use a fresh database and a GORM query callback with an atomic counter. Require:

```text
anonymous request                         -> 0 authorization SELECT callbacks
active admin request                      -> 2 callbacks
active buyer request                      -> 2 callbacks
active merchant request including join    -> 2 callbacks
```

The callback must count only requests issued while serving `/probe`, not fixture inserts or setup queries.

- [ ] **Step 6: Run focused and full backend tests**

```bash
cd backend
gofmt -w internal/middleware/auth.go internal/middleware/auth_test.go \
  tests/restricted_and_security_test.go
go test ./internal/middleware -count=1 -v
go test ./tests -run 'TestMerchantAccessTokenUsesCurrentReviewScope|TestAdminChangePasswordRevokesOnlyTargetAdminSessions' -count=1 -v
go test ./... -count=1
```

Expected: PASS. Existing pending/rejected merchant login and onboarding tests must remain green.

- [ ] **Step 7: Commit authoritative actor loading**

```bash
git add backend/internal/middleware/auth.go backend/internal/middleware/auth_test.go \
  backend/tests/restricted_and_security_test.go
git commit -m "fix(auth): enforce current account authorization"
```

---

### Task 3: Make logout use an exact conditional revocation contract

**Files:**
- Modify: `backend/internal/app/auth_handlers.go:264-276`
- Create: `backend/internal/app/auth_handlers_test.go`
- Create: `backend/tests/session_revocation_test.go`
- Modify: `backend/tests/buyer_flow_test.go:34-70`

**Interfaces:**
- Consumes: current actor validated by `RequireActiveSession`, `model.AuthSession`, `common.ErrUnauthorized`, and `common.ErrInternal`.
- Produces: `func revokeCurrentSession(db *gorm.DB, actor common.Actor, now time.Time) error` and complete three-actor access/refresh revocation contracts.

- [ ] **Step 1: Write RED direct revocation predicate tests**

In package `app`, create a real SQLite table and test the helper directly:

```go
func TestRevokeCurrentSessionRequiresExactActiveIdentity(t *testing.T) {
	db := newAuthHandlerTestDB(t)
	now := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	session := model.AuthSession{
		UserType: model.UserTypeMerchant,
		UserID: 41,
		ExpiredAt: now.Add(time.Hour),
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}

	wrong := common.Actor{UserType: model.UserTypeMerchant, UserID: 42, SessionID: session.ID}
	if err := revokeCurrentSession(db, wrong, now); !errors.Is(err, common.ErrUnauthorized) {
		t.Fatalf("identity mismatch error = %v", err)
	}
	var unchanged model.AuthSession
	if err := db.First(&unchanged, session.ID).Error; err != nil || unchanged.RevokedAt != nil {
		t.Fatal("identity mismatch changed session")
	}

	exact := common.Actor{UserType: model.UserTypeMerchant, UserID: 41, SessionID: session.ID}
	if err := revokeCurrentSession(db, exact, now); err != nil {
		t.Fatalf("exact revoke: %v", err)
	}
	if err := revokeCurrentSession(db, exact, now); !errors.Is(err, common.ErrUnauthorized) {
		t.Fatalf("second revoke error = %v", err)
	}
}
```

Register an update callback returning a synthetic database error and require `common.ErrInternal`.

- [ ] **Step 2: Write RED three-actor API revocation tests**

Create `session_revocation_test.go` with one subtest each for admin, merchant, and buyer. Each subtest must:

1. create/login the actor twice and capture two access/refresh pairs;
2. prove both access tokens currently reach an actor-appropriate route;
3. logout the first session through `POST /api/v1/auth/logout`;
4. require the first old access token and refresh token both return code `10002`;
5. require the second access token and refresh token remain usable;
6. query `auth_sessions` by parsed `sid` and require only the first row has `revoked_at`.

Use these routes:

```text
ADMIN:    GET /api/v1/admin/logs;       POST /api/v1/auth/refresh
MERCHANT: GET /api/v1/merchant/profile; POST /api/v1/auth/refresh
BUYER:    GET /api/v1/buyer/intents;    POST /api/v1/buyer/auth/refresh
```

Never include access or refresh strings in `Fatalf` messages.

- [ ] **Step 3: Extend the existing buyer logout test**

Immediately after buyer logout, add:

```go
accessAfterLogout := requestJSON(t, srv.Router, http.MethodGet,
	"/api/v1/buyer/intents", nil,
	map[string]string{"Authorization": "Bearer " + access})
if accessAfterLogout.Code != common.CodeUnauthorized {
	t.Fatalf("access after logout code = %d", accessAfterLogout.Code)
}
```

Use `common.CodeUnauthorized` rather than literal `10002` and add the import.

- [ ] **Step 4: Run focused tests and verify RED**

```bash
cd backend
go test ./internal/app -run '^TestRevokeCurrentSession' -count=1 -v
go test ./tests -run 'TestSessionRevocation|TestBuyerAuthRefreshLogout' -count=1 -v
```

Expected: the internal app test FAILS to compile because `revokeCurrentSession` does not exist. The API tests may already prove access rejection after Tasks 1-2; retain them as the end-to-end contract.

- [ ] **Step 5: Implement exact conditional revocation**

Add:

```go
func revokeCurrentSession(db *gorm.DB, actor common.Actor, now time.Time) error {
	result := db.Model(&model.AuthSession{}).
		Where("id = ? AND user_type = ? AND user_id = ? AND revoked_at IS NULL",
			actor.SessionID, actor.UserType, actor.UserID).
		Update("revoked_at", &now)
	if result.Error != nil {
		return common.ErrInternal
	}
	if result.RowsAffected != 1 {
		return common.ErrUnauthorized
	}
	return nil
}
```

Replace the handler update with:

```go
if err := revokeCurrentSession(s.DB, actor, time.Now()); err != nil {
	common.Fail(c, err)
	return
}
common.Success(c, gin.H{"success": true})
```

- [ ] **Step 6: Run focused, race, and full tests**

```bash
cd backend
gofmt -w internal/app/auth_handlers.go internal/app/auth_handlers_test.go \
  tests/session_revocation_test.go tests/buyer_flow_test.go
go test ./internal/app -run '^TestRevokeCurrentSession' -count=1 -v
go test ./tests -run 'TestSessionRevocation|TestBuyerAuthRefreshLogout|TestAdminChangePasswordRevokesOnlyTargetAdminSessions' -count=1 -v
go test -race ./internal/middleware ./internal/app -run 'TestRequireActiveSession|TestRevokeCurrentSession' -count=1
go test ./... -count=1
```

Expected: PASS with one session revoked and unrelated sessions still active.

- [ ] **Step 7: Commit logout revocation contracts**

```bash
git add backend/internal/app/auth_handlers.go backend/internal/app/auth_handlers_test.go \
  backend/tests/session_revocation_test.go backend/tests/buyer_flow_test.go
git commit -m "fix(auth): revoke access tokens at logout"
```

---

### Task 4: Build the guarded MySQL 8.4 acceptance matrix

**Files:**
- Create: `backend/tests/session_revocation_mysql_test.go`
- Create: `backend/tests/session_revocation_acceptance_contract_test.go`
- Create: `deploy/acceptance/session-revocation-smoke.sh`
- Modify: `deploy/acceptance/README.md`
- Modify: `Makefile`

**Interfaces:**
- Consumes: committed F-14 middleware/logout behavior, existing acceptance `docker-compose.yml` and `prepare.sh`, current `0001..0008` migration chain, and isolated `DB_DSN` pointing to `mysql:3306/second_hand_market_acceptance`.
- Produces: opt-in `TestSessionRevocationMySQLAcceptance`, guarded Make target `acceptance-session-revocation-smoke`, final marker `isolated session access revocation acceptance passed`, and sanitized evidence under `deploy/acceptance/evidence/session-access-revocation/`.

- [ ] **Step 1: Write the RED acceptance-script guard behavior test**

Create a test that executes `../../deploy/acceptance/session-revocation-smoke.sh` with unsafe environments. Put a fake `docker` executable first on `PATH`; every case must exit nonzero before that fake is invoked:

```go
func TestSessionRevocationAcceptanceRejectsUnsafeEnvironmentBeforeDocker(t *testing.T) {
	script := "../../deploy/acceptance/session-revocation-smoke.sh"
	stubDir := t.TempDir()
	dockerCalled := filepath.Join(stubDir, "docker-called")
	dockerStub := filepath.Join(stubDir, "docker")
	stub := "#!/bin/sh\n: >\"$DOCKER_CALLED\"\nexit 99\n"
	if err := os.WriteFile(dockerStub, []byte(stub), 0o700); err != nil {
		t.Fatalf("write docker stub: %v", err)
	}
	cases := []struct {
		name    string
		confirm string
		engine  string
		project string
	}{
		{name: "missing confirmation", engine: "mysql8.4"},
		{name: "wrong confirmation", confirm: "unsafe", engine: "mysql8.4"},
		{name: "wrong database engine", confirm: "I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_SESSION_REVOCATION_DATA", engine: "mysql8.0"},
		{name: "wrong compose project", confirm: "I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_SESSION_REVOCATION_DATA", engine: "mysql8.4", project: "secondhand-market"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.Remove(dockerCalled); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("reset docker marker: %v", err)
			}
			cmd := exec.Command("/bin/bash", script)
			cmd.Env = append(os.Environ(),
				"PATH="+stubDir+":/usr/bin:/bin",
				"DOCKER_CALLED="+dockerCalled,
				"SESSION_REVOCATION_ACCEPTANCE_CONFIRM="+tc.confirm,
				"ACCEPTANCE_DB_ENGINE="+tc.engine,
				"COMPOSE_PROJECT_NAME="+tc.project,
			)
			if err := cmd.Run(); err == nil {
				t.Fatal("unsafe acceptance environment succeeded")
			}
			if _, err := os.Stat(dockerCalled); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("unsafe acceptance environment reached docker")
			}
		})
	}
}
```

- [ ] **Step 2: Run the contract test and verify RED**

```bash
cd backend
go test ./tests -run '^TestSessionRevocationAcceptanceRejectsUnsafeEnvironmentBeforeDocker$' -count=1 -v
```

Expected: FAIL because the script does not exist.

- [ ] **Step 3: Add the opt-in MySQL behavior test**

The test begins with strict isolation validation:

```go
func TestSessionRevocationMySQLAcceptance(t *testing.T) {
	if os.Getenv("SESSION_REVOCATION_MYSQL_TEST") != "1" {
		t.Skip("set SESSION_REVOCATION_MYSQL_TEST=1 only in the isolated session revocation project")
	}
	dsn := strings.TrimSpace(os.Getenv("DB_DSN"))
	parsed, err := mysqlcfg.ParseDSN(dsn)
	if err != nil || parsed.Net != "tcp" || parsed.Addr != "mysql:3306" ||
		parsed.DBName != "second_hand_market_acceptance" {
		t.Fatal("DB_DSN must target isolated mysql:3306/second_hand_market_acceptance")
	}
	cfg := app.Config{
		AppEnv:                   "test",
		Addr:                     ":0",
		DBDriver:                 "mysql",
		DBDSN:                    dsn,
		JWTAccessSecret:          "session-revocation-test-access",
		JWTRefreshSecret:         "session-revocation-test-refresh",
		AccessTTL:                time.Hour,
		RefreshTTL:               24 * time.Hour,
		AutoMigrate:              strings.EqualFold(os.Getenv("AUTO_MIGRATE"), "true"),
		FileStorageProvider:      "local",
		FileUploadLocalDir:       t.TempDir(),
		ImageCompressTargetBytes: 10 * 1024 * 1024,
		ImageProcessorDriver:     "passthrough",
		BuyerWechatLoginMode:     "mock",
		BuyerDouyinLoginMode:     "mock",
		BuyerWechatHTTPTimeout:   5 * time.Second,
		BuyerDouyinHTTPTimeout:   5 * time.Second,
	}
	configureTestUploadGovernance(&cfg)
	srv, err := app.NewServer(cfg)
	if err != nil {
		t.Fatalf("start isolated session revocation server: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, sqlErr := srv.DB.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
}
```

Use unique synthetic usernames and no production-derived values. The test must execute and assert without logging tokens or IDs:

```text
- ADMIN/MERCHANT/BUYER: two sessions, logout first, old access+refresh 10002, second session still works.
- Two concurrent logout requests for one token: exactly one code 0 and one code 10002.
- ADMIN/MERCHANT/BUYER explicit account disable: old access returns 10007.
- Merchant APPROVED -> REJECTED: old full token gets 10006 on full route and code 0 on profile.
- Missing, expired, revoked, and identity-mismatched session: code 10002.
- Anonymous /healthz still succeeds after closing the SQL pool; an authenticated request returns 20001.
- EXPLAIN session/admin/buyer primary-key lookups reports key PRIMARY.
- EXPLAIN merchant account + merchant join reports PRIMARY for both rows and no ALL scan.
```

Close the SQL pool only in the final subtest. Failure messages report only case name, HTTP/business code, row count, or query-plan access type; never report response data or credentials.

- [ ] **Step 4: Implement the guarded acceptance script**

Start with exact guards:

```bash
#!/usr/bin/env bash
set -euo pipefail

base_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd -- "$base_dir/../.." && pwd)"
project_name="secondhand-session-revocation-acceptance"
evidence_dir="$base_dir/evidence/session-access-revocation"
compose=(docker compose --project-name "$project_name" --env-file "$base_dir/.env" --file "$base_dir/docker-compose.yml")
production_containers=(secondhand-market-api secondhand-market-web secondhand-market-mysql)

[[ "${SESSION_REVOCATION_ACCEPTANCE_CONFIRM:-}" == \
  "I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_SESSION_REVOCATION_DATA" ]] || exit 1
[[ "${ACCEPTANCE_DB_ENGINE:-}" == "mysql8.4" ]] || exit 1
[[ -z "${COMPOSE_PROJECT_NAME:-}" || "$COMPOSE_PROJECT_NAME" == "$project_name" ]] || exit 1
[[ -f "$base_dir/.env" ]] || exit 1
```

The script must:

1. refuse any existing container, volume, network, or evidence directory for this project;
2. create evidence mode `0700` and a runtime directory with `mktemp -d`;
3. snapshot production container name/ID/state/restart count read-only before the run, recording `absent` when a named container does not exist;
4. hash only committed whitelist-shaped source files, excluding `.env`, secrets, evidence, caches, uploads, and `backend/app.db`;
5. start only the dedicated MySQL service and require `SELECT VERSION()` to match `8.4.*`;
6. reset only the isolated database and apply `0001`, `0002`, `0003`, then every preflight/up/postflight artifact through `0008`;
7. build the `bootstrap-admin` tool image and run `TestSessionRevocationMySQLAcceptance` with `AUTO_MIGRATE=false`;
8. reset/reapply the migration chain and rerun with `AUTO_MIGRATE=true`;
9. run `go test ./... -count=1` with `SESSION_REVOCATION_MYSQL_TEST=0` and `go vet ./...`;
10. compare the before/after production snapshots byte-for-byte;
11. reject evidence containing `Authorization`, `access_token`, `refresh_token`, JWT/DB secret variable assignments, OpenID, session IDs, or actor/account fixture IDs;
12. write SHA-256 for every retained evidence text file;
13. stop but retain the dedicated Compose resources and print the final success marker only after all assertions pass.

Use a trap that stops only resources with the exact dedicated project label. Never use `docker compose down`, never remove volumes, and never address a production Compose project.

- [ ] **Step 5: Add Make and README contracts**

Add the target to `.PHONY` and implement:

```make
acceptance-session-revocation-smoke:
	@test "$${SESSION_REVOCATION_ACCEPTANCE_CONFIRM:-}" = "I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_SESSION_REVOCATION_DATA" || { echo "set SESSION_REVOCATION_ACCEPTANCE_CONFIRM for isolated session revocation tests" >&2; exit 1; }
	@test "$${ACCEPTANCE_DB_ENGINE:-}" = "mysql8.4" || { echo "set ACCEPTANCE_DB_ENGINE=mysql8.4" >&2; exit 1; }
	./deploy/acceptance/session-revocation-smoke.sh
```

Document the exact remote path, project, confirmation, MySQL version, retained evidence location, source exclusions, and production non-action in `deploy/acceptance/README.md`.

- [ ] **Step 6: Run local harness and opt-in guard checks**

```bash
bash -n deploy/acceptance/session-revocation-smoke.sh
cd backend
go test ./tests -run '^TestSessionRevocationAcceptanceRejectsUnsafeEnvironmentBeforeDocker$' -count=1 -v
go test ./tests -run '^TestSessionRevocationMySQLAcceptance$' -count=1 -v
cd ..
env -u SESSION_REVOCATION_ACCEPTANCE_CONFIRM -u ACCEPTANCE_DB_ENGINE \
  make acceptance-session-revocation-smoke
```

Expected: Bash syntax PASS, contract PASS, MySQL behavior test SKIP without opt-in, and the unconfirmed Make target exits nonzero before Docker access.

- [ ] **Step 7: Commit the acceptance harness**

```bash
git add backend/tests/session_revocation_mysql_test.go \
  backend/tests/session_revocation_acceptance_contract_test.go \
  deploy/acceptance/session-revocation-smoke.sh deploy/acceptance/README.md Makefile
git commit -m "test(acceptance): verify session access revocation"
```

---

### Task 5: Close F-14 on the code side with local evidence

**Files:**
- Modify: `docs/full-project-code-review-2026-07-24.md`
- Modify: `docs/release-readiness.md`

**Interfaces:**
- Consumes: committed Tasks 1-4, approved design/plan, focused and full local test output, and a clean staged scope.
- Produces: precise `code-side fixed; test-server review pending; production not deployed/data unchanged` status.

- [ ] **Step 1: Run complete local verification from a fresh command**

```bash
make test
cd backend
go vet ./...
cd ..
bash -n deploy/acceptance/session-revocation-smoke.sh
git diff --check
git status --short --branch
```

Expected: all commands exit 0. Only `.tmp/` and the three protected untracked documents may remain outside committed work.

- [ ] **Step 2: Audit the exact F-14 invariants in source**

```bash
rg -n 'RequireActiveAdminSession|RequireActiveSession|requireActiveSession|loadAuthoritativeActor' \
  backend/internal/middleware backend/internal/app/server.go
rg -n 'revoked_at IS NULL|RowsAffected|revokeCurrentSession' \
  backend/internal/app/auth_handlers.go backend/internal/app/auth_handlers_test.go
rg -n 'access_token|refresh_token|Authorization|SessionID|actor\.UserID' \
  backend/internal/middleware/auth.go deploy/acceptance/session-revocation-smoke.sh
```

Expected: no production reference to `RequireActiveAdminSession`; exact conditional revocation exists; middleware/acceptance code does not log or echo secret/identity fields. Header/token names may appear only in test request construction and evidence leak detection, never in output commands.

- [ ] **Step 3: Request a specification and code-quality review**

Invoke `superpowers:requesting-code-review` against the approved design, this plan, and the Task 1-4 commit range. The review must check:

```text
- every authenticated actor requires exactly one active identity-matched session;
- account and merchant review states use exact approved error codes;
- current role/merchant/scope replaces stale JWT claims before route middleware;
- no-token public requests avoid DB reads;
- logout zero-row and DB errors do not report success;
- tests cannot leak token/identity values;
- no schema, TTL, production, or unrelated-finding change entered the range.
```

Do not mark F-14 code-side fixed while any P0-P2 finding remains unresolved. Any correction must begin with a focused failing regression test, then implementation, focused/full verification, and a Conventional Commit.

- [ ] **Step 4: Append the code-side closure record**

Under F-14 in `docs/full-project-code-review-2026-07-24.md`, preserve the original finding and append:

```markdown
**Follow-up status (2026-07-27): code-side fixed; isolated test-server review pending; production not deployed**

Design: `docs/superpowers/specs/2026-07-27-session-access-revocation-design.md`
Plan: `docs/superpowers/plans/2026-07-27-session-access-revocation.md`

The branch now validates the active identity-matched session and current account state for administrators, merchants, and buyers on every authenticated request. Logout immediately invalidates the current access and refresh token, while unrelated sessions remain active. Local focused, full Go, race, vet, and acceptance-harness contract gates passed. No source was transferred for F-14, no isolated server was run, and production data/services were not changed.
```

Add an F-14 row to the release-readiness status table using:

```text
修复状态：代码侧已修复
测试服务器审核：未审核；专用 Compose 项目尚未获授权运行
生产状态：未部署，未修改生产数据或 session
```

Do not modify the three protected review documents.

- [ ] **Step 5: Verify and commit code-side status**

```bash
git diff --check
git diff -- docs/full-project-code-review-2026-07-24.md docs/release-readiness.md
git add docs/full-project-code-review-2026-07-24.md docs/release-readiness.md
git diff --cached --check
git commit -m "docs(auth): record F-14 code closure"
```

---

### Task 6: Obtain authorization and run isolated MySQL 8.4 acceptance

**Files:**
- Create after successful execution: `docs/superpowers/reviews/2026-07-27-session-access-revocation-isolated-acceptance.md`
- Modify after successful execution: `docs/full-project-code-review-2026-07-24.md`
- Modify after successful execution: `docs/release-readiness.md`

**Interfaces:**
- Consumes: exact committed whitelist, remote alias `aliyun-server`, remote path `/home/yu/services/secondhand-session-revocation-acceptance-20260727`, Compose project `secondhand-session-revocation-acceptance`, and separate written authorization.
- Produces: sanitized MySQL 8.4 evidence, source-hash equality, unchanged production container snapshots, and `fixed and passed isolated test-server review` status.

- [ ] **Step 1: Request the exact separate authorization**

Request this scope verbatim before any SSH directory creation or transfer:

```text
授权将本地私有仓库中 F-14 验收所需的 backend/ Go 源码、测试、migrations、Dockerfile、deploy/acceptance/ 非敏感源码与清单、Makefile、backend/go.mod、backend/go.sum 白名单文件传输到 aliyun-server:/home/yu/services/secondhand-session-revocation-acceptance-20260727，仅用于独立 Compose 项目 secondhand-session-revocation-acceptance 的隔离 MySQL 8.4 测试；禁止传输 .env、密钥、数据库、上传文件、证据目录、.git、缓存、node_modules、backend/app.db、miniapp 私有配置、.tmp 和三份受保护审查文档。
```

Design/spec/plan approval and earlier F-02/F-04/F-05/F-06 authorization do not authorize this path or project.

- [ ] **Step 2: Build and inspect the committed transfer manifest**

After authorization, from the repository root:

```bash
f14_manifest="$(mktemp)"
git ls-files -z -- \
  ':(glob)backend/**/*.go' \
  ':(glob)backend/migrations/*.sql' \
  backend/Dockerfile backend/go.mod backend/go.sum \
  ':(glob)deploy/acceptance/*.sh' \
  ':(glob)deploy/acceptance/*.yml' \
  ':(glob)deploy/acceptance/*.conf' \
  ':(glob)deploy/acceptance/*.md' \
  ':(glob)deploy/acceptance/*.Dockerfile' \
  ':(glob)deploy/acceptance/sql/*.sql' \
  Makefile >"$f14_manifest"
```

Inspect it without exposing file contents:

```bash
tr '\0' '\n' <"$f14_manifest" | LC_ALL=C sort
tr '\0' '\n' <"$f14_manifest" | rg -n \
  '(^|/)(\.env|app\.db|evidence|secrets|uploads|\.git|\.cache|node_modules|\.tmp)(/|$)|project\.private\.config\.json|first-round-fix-review|second-round-fix-review|architecture-evolution-plan' \
  && exit 1 || true
```

Expected: only approved tracked source files; no forbidden match.

- [ ] **Step 3: Create the exact new remote directory and transfer only the manifest**

```bash
ssh aliyun-server 'test ! -e /home/yu/services/secondhand-session-revocation-acceptance-20260727 && install -d -m 700 /home/yu/services/secondhand-session-revocation-acceptance-20260727'
rsync --archive --relative --from0 --files-from="$f14_manifest" ./ \
  aliyun-server:/home/yu/services/secondhand-session-revocation-acceptance-20260727/
```

Do not transfer the Git directory or reuse a production/source service directory.

- [ ] **Step 4: Prepare generated remote-only secrets and run the guarded target**

```bash
ssh aliyun-server '
  set -eu
  cd /home/yu/services/secondhand-session-revocation-acceptance-20260727
  ./deploy/acceptance/prepare.sh
  COMPOSE_PROJECT_NAME=secondhand-session-revocation-acceptance \
  SESSION_REVOCATION_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_SESSION_REVOCATION_DATA \
  ACCEPTANCE_DB_ENGINE=mysql8.4 \
  make acceptance-session-revocation-smoke
'
```

Expected final marker: `isolated session access revocation acceptance passed`. The script stops and retains only the dedicated project resources.

- [ ] **Step 5: Compare committed source hashes without copying secrets**

Create a local whitelist hash in a temporary directory outside the repository, using the same file set as the script. Retrieve only the sanitized evidence directory into another `mktemp -d` location and compare `source-sha256.txt` byte-for-byte.

Required comparisons:

```text
local committed whitelist hash == remote source-sha256.txt
production-before.txt == production-after.txt
MySQL version starts 8.4.
both AUTO_MIGRATE=false/true behavior runs PASS
full backend test and vet exit codes are zero
evidence leak scan exit code is zero
```

Do not copy remote `.env`, secrets, volumes, databases, uploads, or raw container logs.

- [ ] **Step 6: Write the sanitized acceptance report**

Create the tracked report with:

```markdown
# F-14 Session Access Revocation Isolated Acceptance

- Date: 2026-07-27
- Branch and exact commit range
- Remote path and Compose project
- MySQL 8.4.x exact version
- Local/remote source manifest SHA-256 equality
- Focused MySQL test names and PASS counts
- Full backend test package/pass counts and vet result
- Sanitized evidence file names and SHA-256 values
- Production container before/after snapshot equality
- Retained dedicated resources
- Explicit non-actions: no production SQL, deployment, restart, data/session/file mutation, or secret transfer
```

Do not include host IPs, usernames, DSNs, passwords, JWTs, access/refresh tokens, OpenIDs, actor/session IDs, request bodies, or production record values.

- [ ] **Step 7: Upgrade only the evidence-backed status**

Update F-14 in the two tracked status documents to:

```text
修复状态：已修复
测试服务器审核：已通过独立 MySQL 8.4 测试服务器审核
生产状态：未部署，未修改生产数据或 session
```

Keep production deployment as pending. Do not change another finding's server-review status.

- [ ] **Step 8: Verify and commit isolated acceptance evidence**

```bash
git diff --check
git status --short --branch
git add docs/superpowers/reviews/2026-07-27-session-access-revocation-isolated-acceptance.md \
  docs/full-project-code-review-2026-07-24.md docs/release-readiness.md
git diff --cached --check
git commit -m "docs(auth): record F-14 isolated acceptance"
```

Expected: no evidence directory, `.env`, secret, database, upload, `.git`, cache, `backend/app.db`, `.tmp/`, or protected document is staged.

---

### Task 7: Run final F-14 review gates and hand back to the all-findings sequence

**Files:**
- Verify only; modify only through a focused RED -> GREEN correction if a gate finds a defect.

**Interfaces:**
- Consumes: approved F-14 design/plan, all F-14 commits, local results, and optional separately authorized isolated-server evidence.
- Produces: a reviewable F-14 commit range with precise remaining gates, then returns to the next open finding without claiming F-01..F-15 are complete.

- [ ] **Step 1: Run final verification from clean processes**

```bash
make test
cd backend
go vet ./...
cd ..
bash -n deploy/acceptance/session-revocation-smoke.sh
git diff --check
git status --short --branch
git log --oneline --decorate -15
```

Expected: all commands pass and only known protected untracked paths remain.

- [ ] **Step 2: Verify commit scope and forbidden paths**

```bash
git diff --name-only 73e194f..HEAD
git diff --name-only 73e194f..HEAD | rg \
  '(^|/)(app\.db|\.env|uploads|evidence|secrets|node_modules|\.tmp)(/|$)|first-round-fix-review|second-round-fix-review|architecture-evolution-plan' \
  && exit 1 || true
git ls-files --error-unmatch backend/app.db >/dev/null
```

Expected: no forbidden path in the F-14 range. `backend/app.db` remains tracked only because its removal/history rewrite belongs to later F-10 work.

- [ ] **Step 3: Reconcile every design acceptance criterion**

Record a concise matrix in the final handoff:

```text
active session required for ADMIN/MERCHANT/BUYER        -> focused + API tests
logout invalidates access and refresh immediately       -> API + MySQL tests
unrelated session remains active                        -> API + MySQL tests
disabled/deleted accounts fail closed                   -> middleware + MySQL tests
merchant review/role/relationship is current            -> middleware + API + MySQL tests
anonymous no-token route performs zero auth reads       -> query-count test
database failure returns 20001                          -> middleware + API/MySQL tests
two indexed reads                                       -> query-count + MySQL EXPLAIN
no schema/JWT/TTL/production mutation                    -> diff + evidence snapshots
```

If Task 6 is not yet authorized, report F-14 as code-side fixed and server review pending, then continue design work for the next finding. Do not represent all first-round findings as fixed while F-10/F-11/F-12/F-15 or outstanding server gates remain open.

## Plan Self-Review Record

- **Spec coverage:** Tasks 1-2 cover universal session validation, authoritative actor loading, exact error semantics, two-query/zero-query behavior, and merchant scope downgrade. Task 3 covers precise logout and unrelated-session behavior. Task 4 covers the isolated MySQL/concurrency/query-plan harness. Tasks 5-7 cover local evidence, separate authorization, server evidence, status precision, and final review.
- **Required-content scan:** Every implementation step has concrete code or exact commands, and no incomplete instruction or undefined execution path remains. Conditional server work has a concrete authorization gate and exact commands.
- **Type consistency:** `RequireActiveSession`, `requireActiveSession`, `loadAuthoritativeActor`, and `revokeCurrentSession` have one signature each and are consumed consistently by later tasks.
- **Scope check:** No migration, model, JWT, TTL, frontend, miniapp, production configuration, or unrelated finding is included.
- **Safety check:** The exact remote path/project/whitelist is isolated; secret generation stays remote; production access is read-only container metadata; protected paths and `backend/app.db` are excluded from transfer and commits.
