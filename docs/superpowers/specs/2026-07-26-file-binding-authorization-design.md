# 文件绑定授权设计

**日期：** 2026-07-26
**问题：** F-02 - 商品图片及营业执照文件 ID 缺少归属、类型和扫描状态校验
**状态：** 已批准进入实施计划，代码实施仍待用户确认
**前置依赖：** F-09 `file_records` 表结构对齐完成并提交
**范围：** 文件业务归属、匿名执照能力令牌、绑定事务与隔离验证

## 1. 背景

当前上传接口会在 `file_records` 中记录上传者、业务类型和扫描状态，上传/确认阶段也会校验上传者；但业务记录绑定文件时没有复用这些安全信息：

- 创建、编辑商品直接把 `image_file_ids` 写入 `product_images` 和 `products.cover_file_id`。
- 商家注册直接把匿名上传得到的 `license_file_id` 写入 `merchants.license_file_id`。
- 商家重新提交审核时也直接替换 `license_file_id`。
- 买家商品接口会把已绑定商品图片解析成公开 URL。

因此，知道或猜到顺序文件 ID 的调用方可以尝试绑定不存在、未上传完成、类型错误或属于其他商家的文件。最严重的路径是把营业执照文件绑定为商品图片，再由公开买家接口暴露。

F-09 正在把 SQL migration 与 GORM 统一到 `file_records`。F-02 必须以该结果为基础，不能与 `0005_file_records_table` 并行修改相同 migration 编号或表名契约。

## 2. 目标

1. 每次写入商品图片或营业执照关联前，在同一数据库事务内校验文件存在、业务类型、扫描状态和商家归属。
2. 匿名注册上传的营业执照使用不可猜测、短期、单次消费的能力令牌，不能只凭 `file_id` 被认领。
3. 文件上传者与文件业务归属分开建模，商家员工变化不影响已归属文件。
4. 所有失败路径保持业务记录和文件归属原子回滚，不留下半绑定状态。
5. 对外统一返回文件绑定失败，不通过响应泄露某个文件 ID 是否存在或属于谁。
6. 对既有干净数据提供可预检、可回填、可复演的 SQL migration 门禁。

## 3. 非目标

- 不实现 F-04 管理员营业执照预览。
- 不实现 F-06 上传频率、存储配额和孤儿文件清理。
- 不实现 F-13 营业执照私有访问或签名 URL。
- 不改变商品图片当前的公开访问方式。
- 不引入对象存储、病毒扫描服务或新的认证系统。
- 不修改订单、库存、商品状态流转和买家下单边界。
- 不部署应用，不在生产执行 migration，也不处理生产数据。
- 不修改或提交三个受保护的未跟踪审核文档。

## 4. 方案比较

### 方案 A：显式商家归属 + 匿名能力令牌（采用）

在 `file_records` 增加 `owner_merchant_id`，把业务归属与 `uploader_type/uploader_id` 分开。匿名执照预签名时生成一次性能力令牌，只保存哈希；注册事务消费令牌并把文件归属到新商家。

优点：归属语义直接；不依赖某个员工账号长期存在；注册前匿名上传也能安全认领；查询和审计简单。代价是需要一次受控 schema migration 和前端注册协议增加令牌字段。

### 方案 B：继续从 `uploader_id` 推导归属（不采用）

商品绑定时通过 `merchant_accounts` 反查上传账号所属商家；匿名注册成功后把 `PUBLIC` 上传者改写成新建账号。

该方案少一列，但混淆“谁上传”和“文件属于哪个商家”。账号删除、后台代上传或员工变更都会影响归属判断，后续很容易再次引入例外分支。

### 方案 C：新增通用 `file_bindings` 表（不采用）

用资源类型、资源 ID、文件 ID 建通用关联表。

该方案扩展性强，但会与 `product_images` 和 `merchants.license_file_id` 重复保存同一关系，还需要解决双写一致性。当前只有两种绑定场景，不值得引入第二套关系事实来源。

## 5. 数据模型

在 F-09 完成后的 `file_records` 上增加以下字段：

```go
type FileRecord struct {
	// existing fields omitted
	OwnerMerchantID     *uint64    `gorm:"index:idx_file_owner_biz_scan,priority:1"`
	CapabilityTokenHash *string    `gorm:"type:char(64);uniqueIndex:uk_file_capability_token"`
	CapabilityExpiresAt *time.Time `gorm:"index:idx_file_capability_expires"`
}
```

最终字段名在实现时保持为：

- `owner_merchant_id BIGINT NULL`
- `capability_token_hash CHAR(64) NULL`
- `capability_expires_at DATETIME NULL`

索引：

- `idx_file_owner_biz_scan (owner_merchant_id, biz_type, scan_status)`，服务绑定校验查询。
- `uk_file_capability_token (capability_token_hash)`，防止令牌哈希意外重复；MySQL 允许多行 `NULL`。
- `idx_file_capability_expires (capability_expires_at)`，为后续 F-06 清理提供查询入口，但本次不实现清理任务。

本次不增加外键。现有 schema 主要依赖应用事务和发布门禁；新增外键会扩大删除、回滚和历史数据兼容面。

### 5.1 字段不变量

1. 认证商家预签名文件时，`owner_merchant_id` 立即写为 actor 的 `merchant_id`。
2. 匿名上传仅允许 `MERCHANT_LICENSE`，初始 `owner_merchant_id` 为 `NULL`。
3. 匿名文件必须同时具有能力令牌哈希和过期时间；原始令牌永不落库、永不写日志。
4. 匿名执照被注册事务成功认领后，写入 `owner_merchant_id`，并把令牌哈希和过期时间清为 `NULL`。
5. 已有 `owner_merchant_id` 不允许通过普通 API 改成另一个商家。
6. `uploader_type/uploader_id` 保留原始上传主体，只用于审计和上传阶段授权，不再充当业务归属。
7. 管理员预签名没有目标商家上下文，`owner_merchant_id` 保持 `NULL`；这类文件不能绑定到商家资源。未来若需要管理员代上传，必须另行设计显式的 acting merchant 参数和审计规则。

## 6. 能力令牌协议

### 6.1 生成

匿名调用 `POST /api/v1/files/presign` 且 `biz_type=MERCHANT_LICENSE` 时：

1. 用 `crypto/rand` 生成 32 字节随机值。
2. 用 base64url 无填充编码作为原始 `file_token`。
3. 数据库只保存 `SHA-256(file_token)` 的 64 位十六进制字符串。
4. `capability_expires_at` 与现有响应的 `expire_at` 保持一致，固定为创建后 15 分钟。
5. 响应额外返回一次 `file_token`；服务端之后无法恢复原文。

认证商家和管理员预签名不返回 `file_token`，继续依赖 bearer actor 授权。

### 6.2 使用位置

匿名执照流程必须在三个位置携带同一个令牌：

| 请求 | 字段 | 用途 |
| --- | --- | --- |
| `POST /files/upload` | multipart `file_token` | 授权写入该匿名文件槽位 |
| `POST /files/confirm` | JSON `file_token` | 授权确认该匿名文件槽位 |
| `POST /auth/register` | JSON `license_file_token` | 原子认领执照并绑定新商家 |

认证调用文件上传/确认时不要求令牌。匿名调用缺失、错误或过期令牌时，统一返回文件授权失败。

前端只把令牌保存在注册页面内存状态中，与 `license_file_id` 成对替换；不得写入 URL、localStorage、控制台或埋点。页面刷新或令牌过期后，用户需要重新选择并上传执照。

## 7. 统一绑定组件

新增 `backend/internal/app/file_binding.go`，只依赖 `gorm.DB`、model、时钟和哈希函数，不依赖 `gin.Context`。

建议接口：

```go
func validateMerchantFilesForBinding(
	tx *gorm.DB,
	merchantID uint64,
	fileIDs []uint64,
	wantBizType string,
) error

func claimPublicMerchantLicense(
	tx *gorm.DB,
	fileID uint64,
	rawToken string,
	merchantID uint64,
	now time.Time,
) error
```

### 7.1 `validateMerchantFilesForBinding`

该函数用于商品创建、商品编辑和商家重新提交：

1. 拒绝空 ID、重复 ID；商品仍保持 1 至 5 张的 DTO 限制。
2. 一次查询取回全部文件，结果数必须等于去重后请求数。
3. 每个文件必须满足：
   - `biz_type == wantBizType`
   - `scan_status == PASS`
   - `url` 非空
   - `owner_merchant_id == merchantID`
4. 任一条件失败时返回统一的 `ErrInvalidFileBinding`，不指出具体失败原因。

查询必须对命中的文件行加 `FOR UPDATE` 锁，并使用调用方已经开启的 `tx`。不得先在事务外校验，再在事务内写关系。这样上传确认、扫描状态更新或归属变更不能在校验与业务关系写入之间穿插。

### 7.2 `claimPublicMerchantLicense`

该函数只用于首次注册，通过单条条件更新消费令牌：

```text
WHERE id = ?
  AND biz_type = 'MERCHANT_LICENSE'
  AND scan_status = 'PASS'
  AND url <> ''
  AND uploader_type = 'PUBLIC'
  AND owner_merchant_id IS NULL
  AND capability_token_hash = SHA256(?)
  AND capability_expires_at > now
```

更新内容为 `owner_merchant_id = merchantID`，同时清空令牌哈希和过期时间。只有 `RowsAffected == 1` 才算成功。并发注册消费同一令牌时，至多一个事务能成功。

## 8. 业务流程变更

### 8.1 首次商家注册

`RegisterRequest` 增加必填 `license_file_token`。事务顺序固定为：

1. 创建不含 `license_file_id` 的 merchant。
2. 创建 owner account。
3. 条件消费匿名执照令牌并写入 `owner_merchant_id`。
4. 更新 merchant 的 `license_file_id`。
5. 写 `MerchantAuditLog`。
6. 提交事务。

第 3 至 5 步任一步失败，merchant、account、文件归属和审核日志全部回滚。事务失败后令牌仍可在未过期时重试，因为消费更新也被回滚。

### 8.2 商家重新提交

若请求包含 `license_file_id`，在修改 merchant 和审核状态前调用：

```go
validateMerchantFilesForBinding(tx, actor.MerchantID, []uint64{*req.LicenseFileID}, model.FileBizMerchantLicense)
```

重新提交不接受匿名令牌。商家必须在已登录 onboarding 会话下重新预签名和上传，文件在预签名时已经获得该商家归属。

### 8.3 商品创建

在创建 `Product` 和 `ProductImage` 前，对全部 `image_file_ids` 调用统一校验，要求 `PRODUCT_IMAGE + PASS + 当前商家归属`。校验失败时不得创建商品、图片关系或操作日志。

### 8.4 商品编辑

只有请求实际包含 `image_file_ids` 时才校验。校验必须发生在删除旧 `product_images` 之前；即使后续写入失败，事务也必须保留旧封面和图片关系。

## 9. API 错误契约

新增业务码：

```go
CodeInvalidFileBinding = 10012
ErrInvalidFileBinding = NewBizError(
	CodeInvalidFileBinding,
	"invalid file binding",
	http.StatusBadRequest,
)
```

下列情况统一返回 HTTP 400 / code `10012`：

- 文件不存在
- 文件类型错误
- 扫描状态不是 `PASS`
- URL 为空
- 文件属于其他商家或尚未归属
- 文件 ID 重复
- 匿名能力令牌缺失、错误、过期或已消费

数据库不可用、更新错误等基础设施故障仍返回 `20001`，不得伪装成客户端错误。日志可以记录失败类别和 request ID，但不得记录原始能力令牌，也不得把其他商家 ID 返回给调用方。

## 10. Migration 设计

F-09 的 `0005_file_records_table` 提交后，本项使用新的连续编号：

- `backend/migrations/0006_file_binding_ownership.preflight.sql`
- `backend/migrations/0006_file_binding_ownership.up.sql`
- `backend/migrations/0006_file_binding_ownership.postflight.sql`

### 10.1 Preflight

在任何 ALTER 或回填前检查：

1. 仅存在规范表 `file_records`，不存在旧表 `files`。
2. 所有 `product_images.file_id` 和 `merchants.license_file_id` 都能找到文件记录。
3. 商品图片关联文件均为 `PRODUCT_IMAGE + PASS + url 非空`。
4. 营业执照关联文件均为 `MERCHANT_LICENSE + PASS + url 非空`。
5. 同一文件没有被两个不同商家引用。
6. 已认证商家上传的已绑定文件，其上传账号所属商家与业务记录商家一致。

任何异常都停止 migration，并输出受影响文件 ID 和商家 ID 供人工逐行处置；禁止自动把冲突文件归到任意一方。

### 10.2 Up

1. 以幂等方式增加三个 nullable 字段和索引。
2. 从现有商品图片、营业执照关联回填 `owner_merchant_id`。
3. 对尚未绑定但 `uploader_type=MERCHANT` 且上传账号仍存在的文件，从 `merchant_accounts.merchant_id` 回填归属。
4. 既有匿名且未绑定的文件保持 `owner_merchant_id=NULL`，不会凭空获得能力令牌。

部署新代码前，应完成或取消正在进行的旧版匿名注册。升级后，未绑定旧匿名执照必须重新上传；不得生成可预测的补发令牌。

### 10.3 Postflight

检查字段、索引和回填不变量，并再次确认所有现有业务关联的文件均属于对应商家。成功输出稳定标记 `file_binding_ownership_postflight_passed`。

不提供破坏性 down migration。旧版本应用会忽略新增 nullable 列；应用回滚时保留归属数据比删除列更安全。

## 11. 测试设计

### 11.1 绑定组件测试

- 同一商家的全部 `PRODUCT_IMAGE + PASS` 文件通过。
- 不存在、重复、错误类型、`PENDING`、`BLOCKED`、空 URL 和跨商家文件均返回 `10012`。
- 多文件请求中只要一项不合法，整体失败。
- 正确匿名执照令牌能认领一次，并清空令牌字段。
- 错误、过期和已消费令牌失败。
- 认领事务回滚后，未过期令牌仍可重试。

### 11.2 API 集成测试

- 首次注册完整执行 presign、upload、register；注册后文件归属新商家。
- 猜测其他匿名执照的 `file_id` 但没有令牌时注册失败，且不产生 merchant/account/audit 行。
- 商品创建和编辑拒绝另一个商家的文件、营业执照文件和未通过扫描的文件。
- 商品编辑失败后，旧封面与 `product_images` 保持不变。
- 重新提交拒绝其他商家的营业执照，审核状态和旧执照保持不变。
- 同一匿名令牌重复注册只有第一次成功。

与文件安全无关的业务测试可以直接通过测试 helper 创建 `PASS` 文件，但 helper 必须显式填写正确的 `owner_merchant_id`；不得继续用只有 presign、尚为 `PENDING` 的文件伪装上传完成。

### 11.3 前端测试

- 注册页把 presign 返回的 `file_token` 与 `file_id` 一起保存并提交。
- 更换执照时同时替换 ID 和令牌，上传失败时不保留半套状态。
- 令牌不进入 localStorage 或 URL。
- 服务端返回 `10012` 时提示重新上传执照，不自动重复提交旧令牌。

### 11.4 MySQL 隔离复演

在保留的非生产 MySQL 环境中覆盖：

1. 既有干净商品图片和执照关系正确回填。
2. 跨商家冲突、错误类型、非 PASS 和孤儿关系均在 preflight 阶段失败，up 未执行。
3. 未绑定 PUBLIC 文件保持无归属；未绑定 MERCHANT 文件按账号商家回填。
4. 全 migration 链 `0001..0006` 在空库执行后，`AUTO_MIGRATE=false` 下完整注册和商品绑定流程通过。
5. 并发消费同一能力令牌时只有一个事务成功。
6. `AUTO_MIGRATE=true` 兼容启动不创建重复列、索引或第二张文件表。

隔离脚本必须要求显式环境确认，不接受生产主机、生产 DSN 或共享生产库名。

## 12. 发布与回滚边界

实现完成不等于生产生效。后续若安排生产维护窗，顺序必须是：备份证据、`0006` preflight、up、postflight、部署后端、部署 frontend、执行只读和受控写 smoke。

应用回滚时保留 `0006` 新列和回填数据；旧应用忽略新增列。若新后端部署后出现问题，只回滚应用镜像，不反向清除 `owner_merchant_id`。

本设计不会关闭 F-04、F-06 或 F-13。只有绑定组件测试、API 回归、MySQL 隔离复演和前端注册协议均通过后，F-02 才能标记为代码侧关闭；生产是否生效必须单独记录。

## 13. 实施边界与文件所有权

后续实施预计修改：

- `backend/internal/model/models.go`
- `backend/internal/common/errors.go`
- `backend/internal/dto/dto.go`
- `backend/internal/app/file_handlers.go`
- `backend/internal/app/file_binding.go`
- `backend/internal/app/auth_handlers.go`
- `backend/internal/app/merchant_handlers.go`
- `backend/internal/app/product_handlers.go`
- `backend/migrations/0006_file_binding_ownership.*.sql`
- `backend/tests/` 下文件上传、注册、商品和 MySQL 隔离测试
- `frontend/src/services/api.ts`
- `frontend/src/pages/auth/RegisterPage.tsx` 及其测试
- 对应 API、数据模型和 release-readiness 文档

实施前必须先确认 Grok 的 F-09 工作已经提交、`0005` 文件名稳定且工作区没有重叠修改。若 F-09 尚未提交，不得开始 F-02 代码实现。

## 14. 完成标准

1. 只凭 `file_id` 无法绑定匿名执照或其他商家的文件。
2. 商品图片和营业执照所有写入口统一校验存在性、类型、`PASS`、URL 与商家归属。
3. 注册认领、商家创建、账号创建、审核日志在一个事务内原子成功或失败。
4. 原始能力令牌不落库、不进日志、不进持久化前端存储。
5. 既有干净数据能由 `0006` 无损回填，冲突数据在 preflight 阶段失败关闭。
6. 后端全量测试、frontend 测试与构建、MySQL 隔离复演全部通过。
7. 库存和订单业务路径无行为变化。
8. 文档明确记录“未部署生产、未执行生产 migration”。
