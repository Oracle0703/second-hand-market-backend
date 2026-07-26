# F-04 / F-13 营业执照私有访问与管理员预览设计

**日期：** 2026-07-26

**分支：** `codex/reconcile-code-reviews`

**状态：** 设计已批准，实施未开始

**问题：**

- F-04：管理员审核详情只能看到 `license_file_id`，无法查看营业执照内容。
- F-13：本地存储通过无鉴权 `/uploads` 静态目录公开营业执照。

**前置依赖：** F-02 文件绑定授权和 `0006_file_binding_ownership` 已在代码侧关闭，并通过隔离 MySQL 8.4.8 测试服务器审核；生产尚未执行 `0006`。

## 1. 目标

本设计在不改变商品图片公开读取契约的前提下完成两个不可拆分的安全目标：

1. 匿名、买家和商家不能通过静态路径或管理员接口读取营业执照。
2. 具有有效 session 的管理员可以在审核详情页预览有效营业执照。
3. 每次成功授权读取都必须写入 `admin_file_read` 操作日志，日志写入失败时不得返回文件内容。
4. 新营业执照不再保存或返回可匿名访问的 URL；已有营业执照 URL 由正式 SQL migration 在未来维护窗清空。
5. 所有实现、迁移和验收只在本地及隔离测试环境执行，本阶段不部署应用、不执行生产 SQL、不修改生产数据。

## 2. 非目标

- 不将商品图片改为私有文件，不改变买家和商家现有商品图片 URL。
- 不引入对象存储、CDN 签名 URL 或新的存储供应商抽象。
- 不处理 F-06 的匿名上传限流、配额或孤儿文件清理。
- 不重新实现 F-02 的 ownership/capability 协议，只调整营业执照“上传完成”的判定依据。
- 不在本阶段读取、移动、删除或改写生产营业执照文件和记录。
- 不把生产未部署描述为线上已修复。

## 3. 方案选择

### 3.1 采用：鉴权内容代理 + 公开前缀 allowlist

营业执照通过管理员鉴权内容接口读取；公开 `/uploads` 只允许商品图片前缀。前端使用带管理员 access token 的 Blob 请求，再用内存 object URL 预览。

采用原因：

- access token 不进入 URL、代理日志、浏览器历史或 Referer。
- 管理员 session 吊销继续由现有 `RequireActiveAdminSession` 生效。
- 可以在发送任何文件字节前完成类型、扫描状态、路径和审计日志校验。
- 商品图片继续使用原 URL，不扩大到 miniapp 和买家页面改造。

### 3.2 不采用：短期签名 URL

签名 URL 可以直接用于 `<img src>`，但会把敏感 token 放入 URL，容易进入访问日志、浏览器历史和诊断截图，并需要额外的签名轮换、过期和重放策略。本阶段不引入。

### 3.3 不采用：所有文件统一鉴权代理

该方案会改变商品图片的公开缓存、买家 API 和 miniapp 展示契约，影响范围远超 F-04/F-13。本阶段只建立营业执照私有边界。

## 4. 后端架构

### 4.1 受控公开文件处理器

移除本地存储模式下的全目录 `r.Static("/uploads", ...)`，改为受控的 `GET /uploads/*path` 处理器。

处理规则：

1. 规范化请求路径并复用 `localUploadPath` 的根目录约束，拒绝目录穿越、空路径、目录和符号链接逃逸。
2. 只 allowlist `product_image/` object-key 前缀；`merchant_license/`、未知前缀和大小写变体统一返回 HTTP 404。
3. 商品图片文件不存在时返回 404；不得把本地绝对路径或 object key 写入响应正文。
4. 商品图片保持现有 URL 和 MIME 行为，并增加 `X-Content-Type-Options: nosniff`。
5. 营业执照无论数据库中是否还保留历史 URL，都不能通过 `/uploads` 返回内容。

使用 404 而不是 403，避免通过公开路径枚举私有文件是否存在。

### 4.2 管理员文件内容接口

新增：

```text
GET /api/v1/admin/files/:id/content
Authorization: Bearer <admin access token>
```

该路由放入现有 admin route group，因此同时要求：

- access JWT 有效；
- actor 为 `ADMIN`；
- `auth_sessions` 中对应管理员 session 存在、未撤销且未过期。

处理顺序固定为：

```text
解析 file id
-> 查询 FileRecord
-> 校验 MERCHANT_LICENSE + PASS + object_key 非空
-> 校验文件已绑定且被同一 owner merchant 的 license_file_id 引用
-> 使用根目录约束解析本地路径
-> 打开并 stat 为普通文件
-> 写入 admin_file_read 操作日志
-> 设置私有响应头
-> 流式返回文件
```

只有全部前置条件成功后才能发送响应头或文件字节。

响应头：

```text
Content-Type: 记录中的允许图片 MIME
Content-Disposition: inline
Cache-Control: private, no-store
Pragma: no-cache
X-Content-Type-Options: nosniff
```

错误契约：

- 未认证：现有 HTTP 401 / `10002`。
- 已认证但非管理员：HTTP 403 / `10003`。
- 文件不存在、类型错误、非 `PASS`、object key 非法、物理文件不存在：HTTP 404 / `10004`，不区分具体原因。
- owner 为空、owner 与引用商家不一致、没有商家引用该执照：同样返回 HTTP 404 / `10004`。
- 数据库、文件系统或审计日志写入失败：HTTP 500 / `20001`，且不得发送文件字节。
- 非本地存储供应商：fail closed 为 `20001`，不回退到公开 URL。

### 4.3 强制操作日志

现有 `writeOperationLog` 会忽略写入错误，不适用于敏感文件读取。新增一个返回 `error` 的窄接口，供管理员文件读取路径使用。

成功日志字段：

```text
action = admin_file_read
resource_type = file
resource_id = <file id>
operator_type = ADMIN
operator_id = <admin id>
merchant_id = <validated owner merchant id>
result_code = 0
```

`merchant_id` 必须来自已验证的 owner/reference 关系，不允许为空。`detail_json` 只记录 `biz_type=MERCHANT_LICENSE` 和 `scan_status=PASS`，不得记录 URL、object key、本地路径、token、文件内容或商家敏感资料。

日志在文件打开和校验成功后、发送响应前写入。日志写入失败时关闭文件并返回内部错误。若后续客户端中断传输，日志表示“服务器已授权并开始读取”，而不是证明客户端完整下载。

## 5. F-02 兼容调整

F-02 当前把 `url <> ''` 作为所有业务类型的上传完成条件。F-13 要求营业执照不再具有公开 URL，因此改为按业务类型判断：

- `PRODUCT_IMAGE`：继续要求 `scan_status=PASS`、`url` 非空、owner 匹配。
- `MERCHANT_LICENSE`：要求 `scan_status=PASS`、`object_key` 非空、owner 匹配；不要求 URL。
- PUBLIC 注册执照 claim：把 SQL 条件从 `url <> ''` 改为 `object_key <> ''`，其余一次性 token、过期、owner 和并发条件不变。

本地上传和 confirm 行为：

- 商品图片继续保存并返回公开 URL。
- 营业执照保存 `object_key`，将 `url` 写为空字符串，成功响应不包含 `url` 字段。
- 现有前端注册流程只依赖 `file_id`、`file_token` 和 `object_key`，无需持久化 URL。

该调整必须补 F-02 回归测试，证明商品图片规则没有放宽，营业执照仍不能在缺少 object key、非 PASS、错误 owner 或 capability 无效时绑定。

## 6. 数据迁移

新增不可逆三段门禁：

```text
0007_license_file_privacy.preflight.sql
0007_license_file_privacy.up.sql
0007_license_file_privacy.postflight.sql
```

### 6.1 Preflight

在任何 UPDATE 前 fail closed：

- `file_records` 必须存在，旧 `files` 必须不存在。
- `0006` 的 ownership/capability 三列和索引形态必须存在。
- 每条 `MERCHANT_LICENSE` 必须有非空 object key、允许的图片 MIME 和合法扫描状态。
- `PASS` 且已绑定商家的营业执照必须满足 owner/uploader 契约。
- 任何异常使用 SQLSTATE `45000` 中止，不修改 URL。

Preflight 不检查本地物理文件是否存在，因为 SQL migration 无法安全访问容器文件系统；该检查属于部署前文件清单门禁和应用验收。

### 6.2 Up

只执行：

```sql
UPDATE file_records
SET url = ''
WHERE biz_type = 'MERCHANT_LICENSE' AND url <> '';
```

不得更新商品图片、owner、capability、object key、扫描状态或时间字段。

### 6.3 Postflight

验证：

- 所有营业执照 URL 均为空。
- `PRODUCT_IMAGE` 的非空 URL 契约仍成立。
- ownership/capability 列和索引仍恰好一份。
- SQL postflight 输出当前总行数和营业执照数量；验收 harness 将其与执行前快照比较，证明两者未改变。

正式维护窗顺序必须是 `0006 preflight/up/postflight -> 0007 preflight/up/postflight -> 部署新 API 与 frontend`。`0007` 执行后不得单独启动仍要求执照 URL 非空的旧 API；回滚采用前向修复，不提供 down migration。

本阶段只提交 migration 文件并在隔离 MySQL 8.4 环境测试，不执行生产 migration。

## 7. 前端设计

审核详情页新增“营业执照”区域，不把说明性帮助文本放入页面。

数据流：

1. 审核详情仍返回 `license_file_id`，不返回 URL。
2. 页面存在 file ID 时，通过 `api.adminLicenseContent(fileID)` 发起 `responseType: 'blob'` 请求。
3. HTTP success interceptor 对 Blob 响应跳过统一 `APIResponse` envelope 校验；401 refresh/retry 逻辑保持有效。
4. 成功后使用 `URL.createObjectURL(blob)` 生成仅内存可见的预览地址。
5. file ID 改变、请求失败、组件卸载时立即 `URL.revokeObjectURL`。

界面状态：

- 加载：稳定尺寸的图片占位/Spin，不推动审核动作区域跳动。
- 成功：Ant Design `Image`，支持内置放大预览，显示文件 ID 供审计排查。
- 无 file ID：显示“暂无营业执照”。
- 401/403：显示鉴权失败状态，不回退到公开 URL。
- 404/损坏 Blob：显示文件不可用状态，审核按钮仍保持原有业务规则，不伪装为图片成功。

Blob 不进入 Zustand、localStorage、sessionStorage、URL 查询参数或日志。

## 8. 测试策略

所有行为变更使用 RED -> GREEN TDD。

### 8.1 后端测试

- 匿名读取 `/uploads/product_image/...` 返回原始商品图片和 `nosniff`。
- 匿名读取 `/uploads/merchant_license/...` 返回 404，即使数据库仍有历史 URL。
- 未知前缀、大小写变体和路径穿越返回 404。
- 管理员有效 session 可读取 PASS 营业执照，内容与 MIME 正确，响应为 `private, no-store`。
- 无 token、商家 token、买家 token、已吊销管理员 token 分别得到预期 401/403。
- 缺失记录、错误 biz type、非 PASS、空 object key、文件缺失、owner 为空或商家引用不一致均返回统一 404。
- `admin_file_read` 日志字段正确且不含敏感路径；日志写入失败时响应无文件字节并返回内部错误。
- 营业执照上传/confirm 不返回 URL；商品图片仍返回 URL。
- F-02 注册 claim、reapply 和商品绑定回归继续通过。

### 8.2 前端测试

- 有 file ID 时发送带认证协议的 Blob 请求并展示真实 `Image`。
- loading、无文件、403、404、损坏 Blob 状态可见且不重叠审核动作。
- 创建的新 object URL 在 file ID 变化和卸载时被释放。
- Blob/URL 不进入持久化 auth store 或浏览器 storage。
- 非 Blob API 继续执行统一业务码校验，Blob 401 仍只刷新一次并重放。

### 8.3 Migration 与隔离测试服务器

新增独立 Compose 项目和明确确认变量，覆盖：

1. 合法历史执照 URL 被清空，商品 URL 保持原值。
2. 缺表、双表、缺少 `0006` 结构、空 object key、错误 MIME/状态/ownership 分别在 UPDATE 前失败。
3. 完整 `0001..0007` 后，以 `AUTO_MIGRATE=false` 启动 API，验证匿名执照 404、商品图 200、管理员 Blob 200 和审计日志。
4. `AUTO_MIGRATE=true` 重启不恢复公开 URL、不创建重复列或索引。
5. 生产容器 ID、状态和重启次数在隔离验收前后只读一致。

证据必须脱敏、记录 SHA-256，并提交 `docs/superpowers/reviews/` 下的验收报告。隔离资源成功后保留供审核；不得在生产运行写入型 smoke。

## 9. 文档与状态

实施完成后更新：

- `docs/backend-api-checklist.md`
- `docs/data-model.md`
- `docs/release-readiness.md`
- `docs/full-project-code-review-2026-07-24.md`
- `docs/production-hardening-repair-plan-2026-07-24.md`

历史发现正文保持审查时点快照，只追加有日期的后续核验。状态必须区分：

- 本分支代码侧关闭；
- 隔离 MySQL 8.4 测试服务器审核通过；
- 生产 `0007` 未执行、应用未部署、现有生产文件未改动。

F-04/F-13 完成不能扩写为 F-06 或其他文件治理项已关闭。

## 10. 验收标准

只有以下证据全部存在，才能把 F-04/F-13 标记为代码侧关闭并通过测试服务器审核：

1. 公开处理器只允许商品图片，所有营业执照静态路径稳定返回 404。
2. 有效管理员通过 Blob API 预览营业执照，非管理员和失效 session 被拒绝。
3. 成功读取有 `admin_file_read` 日志，日志失败时不泄露任何文件字节。
4. 新营业执照 URL 为空且 F-02 绑定/claim 继续安全；商品图片 URL 契约不变。
5. `0007` fail-closed 矩阵、完整迁移链、API 文件流和 AutoMigrate 兼容在 MySQL 8.4 通过。
6. 后端、frontend、miniapp 相关回归和 frontend production build 通过。
7. 脱敏证据、SHA-256、审查结果和提交范围可追溯。
8. 生产容器、数据库、文件和部署均未被修改。
