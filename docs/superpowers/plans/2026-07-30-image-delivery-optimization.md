# 商品图片详情级压缩与回填实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. 本线程不主动派生子代理，默认内联执行。

**Goal:** 商品图片上传和历史回填都产出单一 `detail-v1` JPEG：最长边不超过 1280px，优先 300KB，严格不超过 500KB；回填可 dry-run、可中断续跑、可延迟清理，且不改变既有 `file_id`。

**Architecture:** 后端新增不可变 `detail-v1` media profile，`PRODUCT_IMAGE` 上传走该 profile，其他业务文件保持现有处理规则。商品关联通过共享谓词校验 `PASS`、业务类型、商户归属和可选严格 `detail-v1` 元数据；历史回填通过独立 CLI、账本表、MySQL 全局 named lock 和 no-replace 对象发布完成。小程序列表使用懒加载，详情 Swiper 用受控 `current` 只给当前及相邻项真实 URL。

**Tech Stack:** Go 1.22、Gin、GORM、MySQL/SQLite、libvips CLI、Taro 3.6.34、React 18、Vitest、Docker。

## 执行状态（2026-08-03）

| 项目 | 状态 | 证据 |
| --- | --- | --- |
| 设计文档修订 | 已完成 | `docs/superpowers/specs/2026-07-30-image-delivery-optimization-design.md` 已按 review response 同步 |
| 实施拆解 | 已完成 | 本计划文档按 Task 1-7 拆解后执行 |
| 后端实现 | 已完成 | `go test ./...` 除 Linux-only acceptance 合约外，其余包通过；`go vet ./...` 通过 |
| 回填安全补强 | 已完成 | `go test ./scripts/backfill_product_images -v` 通过，覆盖 dry-run JSON Lines、跨 run 阻断、`STAGED/PROCESSING` 恢复、`FAILED` 重试、cleanup 哈希与删除分支 |
| 小程序验证 | 已完成 | `npm.cmd test` 通过 14 files / 65 tests；`npm.cmd run build:weapp` 通过 |
| 前端验证 | 已完成 | `npm.cmd test` 通过 6 files / 20 tests；`npm.cmd run build` 通过 |
| 环境限制 | 已记录 | Docker CLI 不存在，真实镜像构建未执行；当前 Windows/无可用 WSL，`miniapp_auth_refresh_acceptance_contract_test.go` 的 Linux-only 测试无法本地完成 |
| 最终提交 | 已完成 | 最终产物随本次提交落库 |

## 全局约束

| 约束 | 要求 |
| --- | --- |
| 适用范围 | 仅 `PRODUCT_IMAGE` 强制进入 `detail-v1`；营业执照、头像及其他业务文件保持旧处理语义 |
| `detail-v1` 常量 | `1280px`、`300 * 1024` bytes、`500 * 1024` bytes、边长候选 `1280/1120/960`、质量候选 `82/78/74/70/66` 全部写死为代码常量 |
| 配置边界 | 不新增可改变 `detail-v1` 语义的环境变量；只新增 `REQUIRE_DETAIL_V1_PRODUCT_IMAGES` 作为商品关联严格开关 |
| 输出格式 | 成功商品图片必须是 `image/jpeg`，对象键扩展名固定 `.jpg` |
| 源 MIME | `PRODUCT_IMAGE/detail-v1` 分支只按实际字节检测源格式；预登记 `mime_type=image/jpeg` 是输出 MIME，不是源 claim |
| 对象发布 | 版本化目标必须使用 no-replace 原语；禁止普通 `os.Rename` 覆盖最终对象 |
| 商品归属 | 商品图片关联、编辑和删除按 `MerchantID` 校验，不能按上传账号 `UserID` 校验；软删除商户账号也要能解析历史归属 |
| 回填候选 | 只处理被 `product_images.file_id` 或 `products.cover_file_id` 引用、`PASS`、`PRODUCT_IMAGE`、且不在 `product_image/detail-v1/` 的文件 |
| 回填互斥 | `--apply`、`--cleanup` 仅支持 MySQL，并持有跨 run 全局 named lock；SQLite 只能 dry-run |
| 回填状态 | 主状态只有 `PENDING/PROCESSING/STAGED/COMMITTED/FAILED`；清理状态只有 `NOT_SCHEDULED/PENDING/DONE/FAILED`；不使用租约、不使用 `SKIPPED`、不提供 `--workers` |
| 事务边界 | `file_records` 切换和账本 `COMMITTED` 必须在同一个数据库事务中提交 |
| 清理边界 | 旧对象至少保留 24 小时，只能由同一 run 的 `--cleanup` 清理；删除前必须校验 `source_key != target_key` 和哈希 |
| 发布顺序 | 第一阶段 `REQUIRE_DETAIL_V1_PRODUCT_IMAGES=false`，严格谓词违规和阻断异常清零后第二阶段切为 `true` |
| 测试纪律 | 每个代码任务先写失败测试并确认失败，再做最小实现；Go 改动后执行 `gofmt` |

---

## 文件职责图

| 文件 | 责任 |
| --- | --- |
| `backend/internal/media/detail_profile.go` | `detail-v1` 常量、候选选择、JPEG 校验、对象键判定 |
| `backend/internal/media/processor.go` | `ProcessRequest.OutputProfile`、passthrough 的窄测试契约 |
| `backend/internal/media/vips_cli_processor.go` | vips 候选生成、60 秒超时、无 fallback 的 JPEG 输出 |
| `backend/internal/media/local_storage.go` | 受控对象路径、no-replace 发布、哈希读取、可幂等删除 |
| `backend/internal/app/config.go` | `REQUIRE_DETAIL_V1_PRODUCT_IMAGES` 显式布尔配置和生产 vips 校验 |
| `backend/internal/app/file_handlers.go` | 商品图预登记、上传处理、输出校验、不可变缓存头 |
| `backend/internal/app/product_handlers.go` | 商品图片共享谓词、同商户 STAFF 支持、删除引用计数 |
| `backend/internal/model/models.go` | `ImageBackfillRun`、`ImageBackfillItem` 账本模型 |
| `backend/internal/app/database_operations.go` | 测试库 AutoMigrate 加入账本模型 |
| `backend/migrations/0004_image_backfill_ledger.up.sql` | 生产 MySQL 前向迁移 |
| `backend/scripts/migrate/main.go` | 显式迁移白名单和 SHA-256 |
| `backend/scripts/backfill_product_images/main.go` | dry-run、apply、retry-failed、cleanup、全局锁和 JSON Lines |
| `backend/Dockerfile` | 打包 API、迁移命令、回填命令和 migrations |
| `miniapp/src/components/ProductCard.tsx` | 商品卡片图片懒加载 |
| `miniapp/src/pages/home/index.tsx` | 首页商品图懒加载 |
| `miniapp/src/pages/category/index.tsx` | 分类商品图懒加载 |
| `miniapp/src/pages/product/detail/index.tsx` | 详情 Swiper 受控 `current` 和相邻项真实 URL |

---

## Task 1: 固定 `detail-v1` media profile

**Files:**
- Create: `backend/internal/media/detail_profile.go`
- Modify: `backend/internal/media/processor.go`
- Modify: `backend/internal/media/vips_cli_processor.go`
- Modify: `backend/internal/media/processor_test.go`

**Interfaces:**
- Produces: `DetailProfileVersion`, `DefaultDetailImagePolicy`, `DetailCandidate`, `DetailImagePolicy.Select`, `ValidateDetailJPEG`, `IsDetailProductImageKey`
- Extends: `ProcessRequest{OutputProfile string}`

- [x] **Step 1: 写失败测试**

在 `backend/internal/media/processor_test.go` 增加：

```go
func TestDetailImagePolicyUsesFixedTraversalAndHardLimit(t *testing.T) {
    policy := DefaultDetailImagePolicy()
    candidate, err := policy.Select([]DetailCandidate{
        {LongestEdge: 1280, Quality: 82, SizeBytes: 360 * 1024},
        {LongestEdge: 1280, Quality: 78, SizeBytes: 299 * 1024},
        {LongestEdge: 1120, Quality: 82, SizeBytes: 250 * 1024},
    })
    if err != nil || candidate.LongestEdge != 1280 || candidate.Quality != 78 {
        t.Fatalf("selected candidate = %+v, err=%v", candidate, err)
    }
    _, err = policy.Select([]DetailCandidate{{LongestEdge: 960, Quality: 66, SizeBytes: 500*1024 + 1}})
    if err != common.ErrInvalidUpload {
        t.Fatalf("oversized candidate error = %v", err)
    }
}

func TestPassthroughDetailProfileIgnoresOutputMIMEClaimAndEmitsJPEG(t *testing.T) {
    result, err := NewPassthroughProcessor(DefaultUploadPolicy()).Process(context.Background(), ProcessRequest{
        InputMIME: "image/jpeg",
        OutputProfile: DetailProfileVersion,
        Content: encodedTestImage(t, "image/png"),
    })
    if err != nil || result.OutputMIME != "image/jpeg" || result.OutputExt != ".jpg" {
        t.Fatalf("detail passthrough result = %+v, err=%v", result, err)
    }
    if got := DetectImageMIME(result.Content); got != "image/jpeg" {
        t.Fatalf("detail output MIME = %q", got)
    }
}
```

- [x] **Step 2: 确认测试因新接口缺失失败**

Run: `cd backend && go test ./internal/media -run 'TestDetailImagePolicy|TestPassthroughDetailProfile' -v`

Expected: FAIL，失败原因是 `DefaultDetailImagePolicy`、`DetailCandidate` 或 `OutputProfile` 尚不存在。

- [x] **Step 3: 实现最小 media profile**

实现固定常量：

```go
const DetailProfileVersion = "detail-v1"
const DetailMaxEdge = 1280
const DetailTargetBytes int64 = 300 * 1024
const DetailHardLimitBytes int64 = 500 * 1024
var DetailEdges = []int{1280, 1120, 960}
var DetailQualities = []int{82, 78, 74, 70, 66}
```

`Select` 按传入顺序返回第一个 `SizeBytes <= DetailTargetBytes` 的候选；如没有目标候选，返回第一个 `SizeBytes <= DetailHardLimitBytes` 的候选；都没有则返回 `common.ErrInvalidUpload`。`PassthroughProcessor` 在 `OutputProfile == DetailProfileVersion` 时仅接受实际字节可解码为 JPEG/PNG 的静态小图，忽略 `InputMIME` claim，统一白底 JPEG，最长边或体积超限直接失败。

- [x] **Step 4: 实现 vips detail 分支**

`VipsCLIProcessor.Process` 在 `OutputProfile == DetailProfileVersion` 时：

```go
ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
defer cancel()
```

按 `DetailEdges x DetailQualities` 最多 15 次调用 vips，输出 `.jpg[Q=<quality>,strip]`，每个候选用 `ValidateDetailJPEG` 校验；遇到首个 `<=300KB` 立即返回，未命中目标时返回已记录的 `<=500KB` 兜底。vips 缺失、context 取消和临时 I/O 返回 `common.ErrInternal`；伪装/损坏源图、非 JPEG 输出和无候选满足硬上限返回 `common.ErrInvalidUpload`。

- [x] **Step 5: 验证并提交**

Run: `cd backend && go test ./internal/media -v`

Run: `cd backend && gofmt -w internal/media/detail_profile.go internal/media/processor.go internal/media/vips_cli_processor.go internal/media/processor_test.go`

Commit: `git add backend/internal/media && git commit -m "feat(media): add product detail image profile"`

---

## Task 2: 商品上传链路和 no-replace 本地存储

**Files:**
- Create: `backend/internal/media/local_storage.go`
- Create: `backend/internal/media/local_storage_test.go`
- Modify: `backend/internal/app/file_handlers.go`
- Modify: `backend/tests/file_upload_test.go`
- Modify: `backend/tests/file_upload_vips_integration_test.go`

**Interfaces:**
- Produces: `LocalObjectPath(root, objectKey)`, `PublishObjectNoReplace(path, content, mode)`, `RemoveLocalObject(root, objectKey)`, `SHA256File(path)`

- [x] **Step 1: 写失败测试**

新增测试断言：

```go
func TestProductImageUploadStoresDetailJPEGAndImmutableCache(t *testing.T) {
    srv := newTestServerWithUploadDir(t, t.TempDir())
    token := approvedMerchantToken(t, srv, "detail_upload")
    uploaded := uploadProductImage(t, srv, token, encodedUploadImage(t, "image/png"), "image/png")
    if !strings.HasPrefix(uploaded.ObjectKey, "product_image/detail-v1/") || !strings.HasSuffix(uploaded.ObjectKey, ".jpg") {
        t.Fatalf("object key = %q", uploaded.ObjectKey)
    }
    req := httptest.NewRequest(http.MethodGet, uploaded.URL, nil)
    w := httptest.NewRecorder()
    srv.Router.ServeHTTP(w, req)
    if got := w.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
        t.Fatalf("cache-control = %q", got)
    }
}
```

再写 no-replace 测试：同一个目标已有字节时发布新内容必须返回冲突，旧字节不变，`.tmp` 被清理。

- [x] **Step 2: 确认失败**

Run: `cd backend && go test ./tests ./internal/media -run 'TestProductImageUploadStoresDetailJPEG|TestPublishObjectNoReplace' -v`

Expected: FAIL，现有对象键不是 `detail-v1/*.jpg`，本地发布仍会覆盖。

- [x] **Step 3: 实现上传契约**

`handlePresign` 中 `biz_type=PRODUCT_IMAGE` 时生成：

```go
objectKey := fmt.Sprintf("product_image/detail-v1/%s.jpg", common.BuildBizNo("F"))
mimeType = "image/jpeg"
```

`handleUploadFile` 对商品图传入 `OutputProfile: media.DetailProfileVersion`，成功后强制 `ValidateDetailJPEG`、`OutputMIME == image/jpeg`、`OutputExt == .jpg`、`SizeBytes <= DetailHardLimitBytes`，然后用 no-replace 发布最终对象。数据库更新失败时删除刚发布且尚未引用的新对象，记录保持 `PENDING`。

- [x] **Step 4: 验证并提交**

Run: `cd backend && go test ./tests ./internal/media -run 'TestFile|TestProductImageUpload|TestPublicUpload|TestLocal' -v`

Run: `cd backend && gofmt -w internal/media/local_storage.go internal/media/local_storage_test.go internal/app/file_handlers.go tests/file_upload_test.go tests/file_upload_vips_integration_test.go`

Commit: `git add backend/internal/media backend/internal/app/file_handlers.go backend/tests && git commit -m "feat(upload): publish product detail images safely"`

---

## Task 3: 商品关联、删除和两阶段严格开关

**Files:**
- Modify: `backend/internal/app/config.go`
- Modify: `backend/internal/app/server.go`
- Modify: `backend/internal/app/product_handlers.go`
- Modify: `backend/internal/app/runtime_guardrails_test.go`
- Modify: `backend/tests/integration_flow_test.go`
- Modify: `backend/tests/restricted_and_security_test.go`
- Modify: `backend/configs/.env.example`
- Modify: `backend/configs/.env.production.mysql.example`
- Modify: `backend/configs/.env.production.sqlite.example`

**Interfaces:**
- Produces: `Config.RequireDetailV1ProductImages bool`
- Produces: `validateMerchantProductImageFiles(tx, actor, ids) error`

- [x] **Step 1: 写失败测试**

覆盖四种关联行为：

```go
// false: 允许同商户旧 PASS 商品图保存
// true: 拒绝非 detail-v1、非 jpg、非 image/jpeg、size_bytes<=0 或 >500KB
// 任意开关: 拒绝 PENDING、跨商户、admin 上传 PRODUCT_IMAGE
// 任意开关: OWNER 上传的 PASS 图允许同商户 STAFF 编辑和删除
```

主流程测试必须改为通过上传接口创建商品图片；不能继续用 presign 后的 `PENDING` 文件创建商品。

- [x] **Step 2: 确认失败**

Run: `cd backend && go test ./tests ./internal/app -run 'TestMainFlow|TestProductImageAssociation|TestRuntimeGuardrails' -v`

Expected: FAIL，现有实现允许 `PENDING` 图关联，且删除仍按 `actor.UserID` 回收。

- [x] **Step 3: 实现共享谓词**

`LoadConfig` 必须显式解析 `REQUIRE_DETAIL_V1_PRODUCT_IMAGES`；生产缺失或非法值拒绝启动。基础谓词固定校验：文件存在、`biz_type=PRODUCT_IMAGE`、`scan_status=PASS`、`uploader_type=MERCHANT`、可从 `merchant_accounts` 包含软删除账号解析到当前 `actor.MerchantID`。严格谓词额外校验 `IsDetailProductImageKey`、`.jpg`、`mime_type=image/jpeg`、`0 < size_bytes <= DetailHardLimitBytes`。

商品创建和编辑在写入 `products`、`product_images` 前调用共享谓词。商品删除先按 `product_images.file_id` 和其他商品 `products.cover_file_id` 双引用计数，只有引用均为 0 才删除文件记录；文件归属同样按 `MerchantID`。

- [x] **Step 4: 验证并提交**

Run: `cd backend && go test ./tests ./internal/app -run 'TestMainFlow|TestRestricted|TestProduct|TestRuntimeGuardrails' -v`

Run: `cd backend && gofmt -w internal/app/config.go internal/app/server.go internal/app/product_handlers.go internal/app/runtime_guardrails_test.go tests/integration_flow_test.go tests/restricted_and_security_test.go`

Commit: `git add backend/internal/app backend/tests backend/configs && git commit -m "feat(product): validate merchant product image ownership"`

---

## Task 4: 回填账本模型和显式迁移

**Files:**
- Modify: `backend/internal/model/models.go`
- Modify: `backend/internal/app/database_operations.go`
- Create: `backend/migrations/0004_image_backfill_ledger.up.sql`
- Modify: `backend/scripts/migrate/main.go`
- Modify: `backend/scripts/migrate/main_test.go`
- Modify: `backend/internal/app/database_operations_test.go`

**Interfaces:**
- Produces: `ImageBackfillRun`, `ImageBackfillItem`
- Constraints: `(run_id, file_id)` 唯一；不设置 `file_id` 全局唯一；不设置级联删除；不提供 down migration

- [x] **Step 1: 写失败测试**

测试 `MigrateSchema` 后能创建 run/item，并断言 `(run_id,file_id)` 唯一、不同 run 可记录同一 `file_id`。迁移脚本测试加入 `0004_image_backfill_ledger`，并确认 down migration 仍被拒绝。

- [x] **Step 2: 确认失败**

Run: `cd backend && go test ./internal/app ./scripts/migrate -run 'TestMigrateSchema|TestMigrationCatalog' -v`

Expected: FAIL，模型和 allowlist 尚未存在。

- [x] **Step 3: 实现模型和 SQL**

字段至少包含：`run_id`、`file_id`、`source_object_key`、`target_object_key`、`profile_version`、`source_sha256`、`output_sha256`、`source_size_bytes`、`output_size_bytes`、`status`、`attempts`、`error_code`、`committed_at`、`cleanup_after`、`cleanup_status`、`cleanup_error_code`、时间戳。主状态长度覆盖 `PROCESSING/COMMITTED`，清理状态覆盖 `NOT_SCHEDULED`。

- [x] **Step 4: 验证并提交**

Run: `cd backend && go test ./internal/app ./scripts/migrate -v`

Run: `cd backend && gofmt -w internal/model/models.go internal/app/database_operations.go internal/app/database_operations_test.go scripts/migrate/main.go scripts/migrate/main_test.go`

Commit: `git add backend/internal/model backend/internal/app backend/migrations backend/scripts/migrate && git commit -m "feat(media): add image backfill ledger"`

---

## Task 5: 回填命令 dry-run/apply/retry/cleanup

**Files:**
- Create: `backend/scripts/backfill_product_images/main.go`
- Create: `backend/scripts/backfill_product_images/main_test.go`

**Interfaces:**
- CLI modes: `--dry-run`、`--apply`、`--cleanup` 三选一；未指定默认 dry-run
- Required flags: `--apply --run-id <id>`、`--cleanup --run-id <id>`
- Optional flags: `--after-id`、`--limit`、`--retry-failed`
- Forbidden: `--workers`、租约、`SKIPPED`
- JSON writer: `func(io.Writer, any) error`

- [x] **Step 1: 写失败测试**

覆盖：

```go
// 参数非法组合在打开数据库或访问对象前失败
// dry-run 真实调用 fake processor，但不写对象、file_records、run 表、item 表
// apply 排除 detail-v1 候选，目标键 product_image/detail-v1/F<file_id>.jpg
// apply 获取 MySQL 全局 named lock；SQLite apply/cleanup fail closed
// STAGED 续跑三分支：仍为 source_key、已为 target_key、指向其他键
// file_records 更新和 COMMITTED 同事务，条件更新 0 行不删除目标
// FAILED 只有 --retry-failed 才递增 attempts 并重试
// cleanup 24 小时前不删除；到期前校验 source_key != target_key、哈希和引用
```

- [x] **Step 2: 确认失败**

Run: `cd backend && go test ./scripts/backfill_product_images -v`

Expected: FAIL，package 或接口尚未存在。

- [x] **Step 3: 实现命令**

候选查询只选 `PRODUCT_IMAGE/PASS`、被商品图片或封面引用、对象存在、且不在 `product_image/detail-v1/` 的 `file_records`。dry-run 执行真实处理并输出预计结果；apply 创建或加载账本项，按 `PENDING -> PROCESSING -> STAGED -> COMMITTED` 推进；事务失败保留 `STAGED`；cleanup 只处理同 run 到期项并按设计的安全分支收敛。全局锁连接丢失必须取消 context，后续禁止写对象或数据库。

- [x] **Step 4: 验证并提交**

Run: `cd backend && go test ./scripts/backfill_product_images ./internal/media -v`

Run: `cd backend && go vet ./scripts/backfill_product_images`

Run: `cd backend && gofmt -w scripts/backfill_product_images/main.go scripts/backfill_product_images/main_test.go`

Commit: `git add backend/scripts/backfill_product_images && git commit -m "feat(media): add safe product image backfill"`

---

## Task 6: 小程序图片按需加载

**Files:**
- Modify: `miniapp/src/components/ProductCard.tsx`
- Modify: `miniapp/src/pages/home/index.tsx`
- Modify: `miniapp/src/pages/category/index.tsx`
- Modify: `miniapp/src/pages/product/detail/index.tsx`
- Create: `miniapp/tests/product-image-loading.test.ts`

**Interfaces:**
- Details: `Swiper.current` 受控；`activeIndex` 初始为 0；真实 URL 条件为 `Math.abs(idx - activeIndex) <= 1`

- [x] **Step 1: 写失败测试**

测试用 React/Taro 可渲染或源码 AST 断言均可，但必须验证行为：详情页初始只给 0、1 项真实 `src`，切到 2 时只给 1、2、3 项真实 `src`；列表卡片、首页商品图和分类商品图设置 `lazyLoad`。

- [x] **Step 2: 确认失败**

Run: `cd miniapp && npm test -- --run tests/product-image-loading.test.ts`

Expected: FAIL，当前 Swiper 给所有图片真实 URL。

- [x] **Step 3: 实现小程序改动**

列表卡片、首页商品流、分类列表的商品图加 `lazyLoad`。详情页引入 `useState`，`Swiper` 设置 `current={activeIndex}` 和 `onChange`，图片 `src` 使用：

```tsx
const shouldLoadImage = Math.abs(idx - activeIndex) <= 1
const imageSrc = shouldLoadImage ? url : ''
```

预览仍使用完整 `imageURLs`，不改变 `resolveAssetURL`。

- [x] **Step 4: 验证并提交**

Run: `cd miniapp && npm test -- --run tests/product-image-loading.test.ts tests/asset-url.test.ts`

Run: `cd miniapp && npm run build:weapp`

Commit: `git add miniapp/src miniapp/tests && git commit -m "perf(miniapp): load product images on demand"`

---

## Task 7: 镜像、发布文档和全量验证

**Files:**
- Modify: `backend/Dockerfile`
- Modify: `docs/release-readiness.md`
- Modify: `backend/configs/.env.production.mysql.example`
- Modify: `backend/configs/.env.production.sqlite.example`

- [x] **Step 1: 写失败测试**

在后端测试中检查 Dockerfile 真实产物契约，而不是绑定无关空白：

```text
go build -o /out/migrate ./scripts/migrate
go build -o /out/backfill-product-images ./scripts/backfill_product_images
COPY --from=build /out/migrate /srv/migrate
COPY --from=build /out/backfill-product-images /srv/backfill-product-images
COPY backend/migrations /srv/migrations
```

- [x] **Step 2: 实现打包和发布说明**

Docker 镜像必须包含 `/srv/server`、`/srv/migrate`、`/srv/backfill-product-images` 和 `/srv/migrations/`。发布文档记录：线上小程序版本检查、基础谓词预检、写冻结、显式 `0004` 迁移、`REQUIRE_DETAIL_V1_PRODUCT_IMAGES=false` 发布、同 run canary/分批 apply、严格谓词清零、切 true、24 小时后 cleanup。

- [x] **Step 3: 全量测试**

Run: `cd backend && go test ./...`

Run: `cd backend && go vet ./...`

Run: `cd miniapp && npm test`

Run: `cd miniapp && npm run build:weapp`

Run: `cd frontend && npm test`

Run: `cd frontend && npm run build`

Run: `git diff --check`

- [x] **Step 4: 提交最终产物**

Commit remaining docs/config/build changes with a focused Conventional Commit message. Final handoff must include changed files, exact verification commands, skipped checks if any, and residual rollout risks.

---

## 自审结果

| 检查项 | 结果 |
| --- | --- |
| 审核 P0 覆盖 | P0-1 在 Task 5 候选排除和 cleanup `source_key != target_key` 覆盖；P0-2 在 Task 5 `STAGED` 对账和同事务覆盖；P0-3 在 Task 3/7 两阶段开关和发布冻结覆盖 |
| 已移除旧错误口径 | 不再有 `IMAGE_DETAIL_*` 环境变量、`--workers`、租约、`SKIPPED`、提交后立即删除旧对象、`file_id` 全局唯一、down migration |
| 测试先行 | 每个代码任务都有失败测试、失败验证、实现、通过验证步骤 |
| 发布边界 | 明确第一阶段 false、严格谓词清零、第二阶段 true、24 小时后 cleanup |
