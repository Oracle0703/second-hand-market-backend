# F-06 匿名上传资源治理设计

**日期：** 2026-07-26

**分支：** `codex/reconcile-code-reviews`

**状态：** 设计与书面规格已批准；实施未开始

**问题：** F-06 - 匿名上传缺少持久化频率限制、存储配额和孤儿文件清理，应用、代理与前端的上传大小契约不一致

**前置依赖：**

- F-02 已建立一次性 `file_token`、15 分钟 capability、文件 owner 和绑定校验。
- F-04/F-13 已将营业执照改为私有文件，并将公开 `/uploads` 收窄为商品图片 allowlist。
- `0005_file_records_table`、`0006_file_binding_ownership`、`0007_license_file_privacy` 是本设计迁移的结构前置条件。
- 上述变更均只表示本分支代码状态；生产尚未执行 `0005..0007` 或部署对应应用。

## 1. 目标

本设计关闭 F-06，并同时消除 D-03 所指出的上传大小契约漂移：

1. 所有客户端可见的单文件业务上限统一为 **10 MiB（10,485,760 bytes）**。
2. multipart HTTP 请求体上限统一为 **11 MiB（11,534,336 bytes）**，只为字段和 boundary 预留传输开销；它不是第二个业务文件上限。
3. 在 multipart 解析和图片处理前拒绝过大请求，避免应用先分配或读取无界内存。
4. 对匿名 presign 实施跨进程、跨重启有效的按 IP 频率和活跃资源限制。
5. 对商家和全局文件记录实施数据库串行化的字节配额，不能因并发 presign 超额。
6. 只自动清理迁移后创建、capability 已过期且仍未绑定的匿名文件；历史记录和已绑定文件永不进入自动删除集合。
7. 原始 IP、HMAC 密钥、capability、清理 claim、object key 和本地路径不得进入响应、普通日志或验收证据。
8. 使用 SQLite 完成功能回归，在隔离 MySQL 8.4 环境证明并发配额、迁移和清理竞态。
9. 本阶段不执行生产 SQL、不部署生产代码或代理配置、不读取或修改生产上传目录和业务数据。

本设计取代当前代码和现状文档中的 40 MB 上传承诺。`2026-04-02-apple-image-upload-design.md` 保留为历史设计快照，不回写其原始结论；当前 README、API 清单、配置示例和用户界面在实施时更新为 10 MiB。

## 2. 非目标与保护边界

- 不替换本地文件存储，不引入 OSS、S3、MinIO、Redis 或外部分布式锁。
- 不改变允许的 JPG、PNG、WebP、HEIC、HEIF 类型和现有图片处理流程，除非为执行 10 MiB 边界所必需。
- 不移除或弱化 F-02 的一次性 `file_token`、过期时间、owner 校验或恒定时间 token 比较。
- 不重新开放匿名商品图片上传；匿名 actor 仍只可上传 `MERCHANT_LICENSE`。
- 不为商家遗留未绑定上传增加自动删除策略。自动删除严格限于本设计标记的新匿名记录。
- 不自动删除、认领、补 token、改 owner 或改写审查时观察到的 13 条生产未绑定记录，也不假设该聚合数量在正式维护窗仍保持不变。
- 不把磁盘可用空间监控、备份、对象生命周期管理或生产 Nginx 维护窗并入代码侧 F-06 完成条件。
- 不修改、提交或传输三份受保护审查文档、`backend/app.db`、生产数据、上传文件、`.env`、密钥或证据目录。
- 不把“本分支修复”或“隔离测试服务器通过”描述成“生产已生效”。

## 3. 方案选择

### 3.1 采用：单行数据库配额锁 + `file_records` 实时聚合

新增只有一行的 `file_quota_guards` 表。每次 presign 在同一数据库事务中先对固定 guard 行执行 `SELECT ... FOR UPDATE`，再统计当前 `file_records`、校验适用限制并创建新记录。匿名文件绑定到新商家时也在同一 guard 下校验商家配额。

该方案把所有会增加配额占用的入口串行化。吞吐量低于分片计数器，但上传 presign 不是高频路径，当前规模下简单、可证明的正确性优先于提前优化。删除和图片压缩只会降低现有占用，因此不需要持有 guard；任何未来可能增加已存在记录 `size_bytes` 的路径必须先取得同一 guard。

### 3.2 不采用：现有进程内 `memoryRateLimiter`

内存限流在进程重启后清空，多副本之间不共享，也不能与文件记录创建形成原子操作，无法证明并发下的全局或存储配额。本设计不复用它承担 F-06。

### 3.3 不采用：独立计数器/事件账本或 Redis

独立计数器需要处理文件创建、绑定、删除、迁移和失败回滚的双写一致性；Redis 还引入当前部署不存在的新基础设施。若未来 presign 锁竞争成为可测量瓶颈，可以在保持同一事务不变量的前提下演进为分片账本，本轮不预先增加复杂度。

## 4. 大小与错误契约

### 4.1 固定字节语义

所有代码、测试和文档使用二进制单位：

```text
file limit       = 10 * 1024 * 1024 = 10,485,760 bytes
multipart limit  = 11 * 1024 * 1024 = 11,534,336 bytes
```

后端不使用十进制 MB 进行比较。前端文案显示 `10 MiB` 或“10 MiB（约 10.5 MB）”，不能继续显示 40 MB。配置名为兼容现状可以保留 `_MB`，但其实现和文档必须明确按 MiB 乘以 `1024 * 1024`。

### 4.2 Presign 边界

`POST /api/v1/files/presign` 在创建任何数据库记录前验证：

- `file_size` 必须大于 0 且不超过 10 MiB；
- MIME 和 biz type 继续走现有 allowlist；
- actor 必须满足现有业务权限；
- 适用的频率和配额必须全部有余量。

JSON 中声明的 `file_size > 10 MiB` 返回 HTTP 400 / code `10008`，因为实际 JSON 请求体本身没有超限；不得创建 `file_records` 行。

### 4.3 Multipart 解析边界

`POST /api/v1/files/upload` 的第一段逻辑在调用 `PostForm`、`FormFile`、`ParseMultipartForm` 或读取字段前：

1. 使用 `http.MaxBytesReader` 将 request body 限制为 11 MiB。
2. 若非负 `Content-Length` 已超过 11 MiB，立即拒绝。
3. 显式解析 multipart，并把 `*http.MaxBytesError` 和超过 10 MiB 的文件统一映射为 HTTP 413 / code `10008`。
4. 只有解析成功后才读取 `file_id`、`object_key`、`file_token` 和文件头。

新增窄错误 `ErrUploadTooLarge`，复用业务 code `10008`，HTTP status 为 413，稳定 message 为 `upload file too large`。其他非法 MIME、空文件、损坏图片、字段不匹配仍使用 HTTP 400 / code `10008`，避免把所有无效上传误报为大小问题。

实际读取继续使用 `io.LimitReader(limit + 1)` 作为纵深防御。读取后的真实原图字节数必须大于 0、不超过 10 MiB，且与 presign 保存的 `size_bytes` 完全一致；少报或多报大小都返回 HTTP 400 / code `10008`。现有处理器只保留原图或更小的处理结果，最终 `size_bytes` 因而不能高于已预留值。

### 4.4 代理边界

仓库控制的 acceptance Nginx 将 `client_max_body_size` 设为 11 MiB，并将代理侧 413 转成 JSON API envelope：

```json
{"code":10008,"message":"upload file too large","request_id":"<proxy request id>"}
```

该响应使用 `application/json`，并与应用的失败 envelope 一样省略 `data`。正式生产的宿主 Nginx 和 Web Nginx 变更属于单独维护窗。两层都应使用 11 MiB request-body 上限和相同 413 envelope；浏览器、前端和 API 文档仍只承诺 10 MiB 文件，不得把 11 MiB 展示给用户。

## 5. 配置合同

新增或收紧以下配置：

| 配置 | 非生产默认值 | 含义 |
| --- | ---: | --- |
| `FILE_UPLOAD_MAX_MB` | `10` | 单文件业务上限，生产必须恰好为 10 |
| `FILE_UPLOAD_MULTIPART_MAX_MB` | `11` | multipart request-body 上限，生产必须恰好为 11 |
| `FILE_UPLOAD_IP_HASH_SECRET` | 仅测试/开发显式假值 | HMAC-SHA256 密钥，生产至少 32 bytes 且不得使用示例值 |
| `FILE_UPLOAD_ANON_PRESIGN_PER_HOUR` | `20` | 每个匿名来源每滚动一小时成功 presign 数 |
| `FILE_UPLOAD_ANON_ACTIVE_FILES` | `5` | 每个匿名来源当前占用的未绑定治理记录数 |
| `FILE_UPLOAD_ANON_ACTIVE_MB` | `50` | 每个匿名来源当前占用的未绑定治理字节 |
| `FILE_UPLOAD_MERCHANT_QUOTA_MB` | `2048` | 每个 merchant owner 的记录字节总量 |
| `FILE_UPLOAD_GLOBAL_QUOTA_MB` | `20480` | 全部 `file_records` 的记录字节总量 |
| `FILE_UPLOAD_CLEANUP_INTERVAL_SECONDS` | `300` | 周期扫描间隔 |
| `FILE_UPLOAD_CLEANUP_BATCH_SIZE` | `50` | 每批最多 claim 的记录数 |
| `FILE_UPLOAD_CLEANUP_CLAIM_TTL_SECONDS` | `600` | 崩溃 claim 可被重试前的租约时间 |
| `FILE_UPLOAD_CLEANUP_GRACE_SECONDS` | `1800` | capability 过期后的清理宽限期 |
| `TRUSTED_PROXY_CIDRS` | `none` | 明确可信的代理 CIDR，`none` 表示只信任直连 peer |

生产启动时，上表所有数值必须由环境显式提供、可解析且为正数；前两个还必须等于批准的 10/11。`FILE_UPLOAD_IP_HASH_SECRET` 和 `TRUSTED_PROXY_CIDRS` 也必须显式提供。缺项、零值、负值、溢出、无效 CIDR、弱/示例 secret 或不一致上限均使 `Config.Validate` fail closed，且错误不得回显 secret。

非生产可以使用表中数值默认值，但 HMAC secret 仍不能由固定仓库常量静默生成；测试使用每个测试独立的合成 secret。生产配置示例只放占位符，不提交真实值。

## 6. 可信来源 IP 与隐私

应用启动时把 `TRUSTED_PROXY_CIDRS` 解析后传给 Gin 的 trusted-proxy 配置。`none` 使用直接 TCP peer；只有请求来自明确可信代理时才接受代理转发的客户端地址。不得依赖 Gin 的“信任所有代理”默认行为。

匿名 presign 将规范化 IP 计算为：

```text
lower_hex(HMAC-SHA256(FILE_UPLOAD_IP_HASH_SECRET, canonical_ip_bytes))
```

IPv4 使用 4-byte 表示，IPv6 使用 16-byte 表示，使不同文本写法得到同一 hash。数据库只保存 64 字符小写 HMAC；原始 IP 不写入 `file_records`、操作日志、错误、普通日志或验收证据。代码不得回退到无密钥 SHA-256。

`source_ip_hash` 只用于匿名频率和活跃资源治理，不用于认证、账号关联或公开分析。即使匿名执照后来绑定商家，该 hash 仍保持不变，使一小时滚动频率不能通过快速绑定绕过；任何日志只记录“命中匿名限制”及限制类别，不记录 hash。

## 7. 数据模型与迁移

新增不可逆三段迁移：

```text
0008_anonymous_upload_governance.preflight.sql
0008_anonymous_upload_governance.up.sql
0008_anonymous_upload_governance.postflight.sql
```

### 7.1 `file_records` 新字段

```text
source_ip_hash       CHAR(64) NULL
cleanup_after        DATETIME(3) NULL
cleanup_claimed_at   DATETIME(3) NULL
cleanup_claim_token  CHAR(64) NULL
cleanup_attempts     INT UNSIGNED NOT NULL DEFAULT 0
```

索引：

```text
idx_file_source_created
  (source_ip_hash, created_at)

idx_file_cleanup_candidate
  (uploader_type, owner_merchant_id, cleanup_after, cleanup_claimed_at)
```

只有迁移后成功创建的匿名 presign 同时写入：

- `source_ip_hash = HMAC(client IP)`；
- `cleanup_after = capability_expires_at + cleanup grace`；
- claim 字段为 `NULL`，attempts 为 0。

认证商家、管理员和所有迁移前记录的 `source_ip_hash`、`cleanup_after`、claim 字段均为 `NULL`。这个 `NULL` 是不可跨越的历史保护标记，不由 migration 回填。

### 7.2 配额 guard

新增 `file_quota_guards`：

```text
id          TINYINT UNSIGNED PRIMARY KEY
guard_name  VARCHAR(32) NOT NULL UNIQUE
created_at  DATETIME(3) NOT NULL
```

迁移只插入固定行 `(1, 'file_records')`。运行时若 guard 缺失，presign 和匿名绑定返回内部错误并记录不含敏感信息的结构错误；不得绕过配额继续创建记录。

### 7.3 Preflight / up / postflight

Preflight 在任何 DDL/DML 前验证：

- 只有权威表 `file_records`，不存在旧 `files`；
- `0006` ownership/capability 和 `0007` privacy 结构、索引均为预期形态；
- `0008` 字段、索引和 guard 表不存在，或按明确的可重入策略已经完整存在；部分漂移一律 SQLSTATE `45000`；
- `size_bytes` 不为负，owner/uploader 基础约束没有新异常。

Up 只增加 nullable 治理字段、两个索引、guard 表和固定 guard 行。它不更新任何现有文件的 biz type、object key、URL、size、uploader、owner、scan status、capability 或创建时间，不回填来源 hash 或清理时间，也不删除记录/文件。

Postflight 验证准确列型、索引顺序、唯一 guard 行，以及所有迁移前行的治理字段仍为 `NULL`。验收 harness 额外比较迁移前后行数、ID 集合、聚合和非治理字段指纹，证明包括审查时观察到的 13 条未绑定记录在内的历史数据没有被加入自动清理集合。

不提供 destructive down migration；回滚采用前向修复。正式生产执行 `0008` 需要单独书面批准和维护窗，本任务不会执行。

## 8. Presign 配额事务

所有权限、MIME、biz type 和基本大小校验先于事务完成，避免无效请求占用全局锁。随后：

```text
BEGIN
  SELECT id FROM file_quota_guards WHERE id=1 FOR UPDATE
  -> 读取同一个 now
  -> 计算适用计数/字节
  -> 任一限制达到上限则 ROLLBACK
  -> 创建 file_records reservation
COMMIT
```

`size_bytes` 在 presign 时就是申报的保守预留量；上传成功后只能保持或减小。所有统计使用非负 `size_bytes` 的和，并使用数据库 `BIGINT`，Go 侧做溢出检查。

所有可能增加配额占用的事务使用 `sql.LevelReadCommitted`，且在 guard 锁之前不得读取或缓存配额统计。取得 guard 后才执行计数/求和，保证等待中的第二个事务能看到第一个事务刚提交的 reservation。SQLite 只承担功能测试，不作为隔离级别证明；MySQL 隔离并发测试必须覆盖“第二个事务先开始、等待第一个事务提交、随后看到第一个 reservation”的时序。

### 8.1 匿名限制

同一 `source_ip_hash`：

- 滚动一小时内成功创建的匿名 presign `< 20`；统计 `created_at > now - 1 hour`，包括随后已绑定的记录。
- 当前活跃匿名治理记录 `< 5`。
- 当前活跃匿名治理字节加新 `file_size <= 50 MiB`。

“活跃匿名治理记录”精确定义为：

```text
uploader_type = 'PUBLIC'
AND owner_merchant_id IS NULL
AND source_ip_hash = ?
AND cleanup_after IS NOT NULL
```

即 capability 过期但尚未成功清理的记录仍占用文件数和字节，攻击者不能利用清理延迟继续扩张存储。

第 21 个一小时 presign 返回 HTTP 429 / code `10009`。活跃文件数或匿名字节超限返回 HTTP 409 / 新 code `10013`，message 为 `upload quota exceeded`。响应不包含当前使用量、IP hash 或限制配置。

### 8.2 商家限制

商家 presign 按 `owner_merchant_id` 统计全部现有记录，新预留后必须 `<= 2 GiB`。匿名营业执照被注册事务绑定到新商家前，也在同一 guard 下把该文件纳入目标 merchant 的 2 GiB 校验；超限则整个注册事务回滚。

商家配额不按上传账号拆分，同一商家的 owner 和其他账号共享 2 GiB。管理员上传没有 merchant owner，只受全局配额。本设计不增加 quota 查询 API，也不向用户暴露其他租户或全局使用量。

### 8.3 全局限制

全局统计所有 `file_records.size_bytes`，包括历史记录、PENDING 预留、已绑定文件和尚未清理的过期匿名记录；新预留后必须 `<= 20 GiB`。全局和商家超限均返回 HTTP 409 / code `10013`。

删除数据库记录会自然释放配额。数据库行已删除但物理文件删除失败属于现有文件一致性风险，不得通过虚减记录外的计数器掩盖；F-06 验收需记录这种错误但不扩大为全盘文件盘点项目。

## 9. 匿名孤儿清理

### 9.1 唯一候选集合

清理 worker 只允许 claim 同时满足以下条件的行：

```text
uploader_type = 'PUBLIC'
AND owner_merchant_id IS NULL
AND cleanup_after IS NOT NULL
AND cleanup_after <= now
AND (
  cleanup_claimed_at IS NULL
  OR cleanup_claimed_at <= now - claim_ttl
)
```

`cleanup_after IS NOT NULL` 保证所有迁移前记录、所有认证上传和没有明确治理标记的记录永远不匹配。候选必须通过专用的安全删除路径校验：规范化 object key，解析真实上传根目录，从根开始以 `Lstat` 逐段检查父目录，拒绝任一符号链接或非目录节点，并以 `Lstat` 拒绝目标目录、符号链接和非普通文件。如果某级父目录或目标文件不存在，则在已验证的根内将其视为“文件从未落盘/已删除”的幂等成功。路径异常不会尝试删除任意文件。不能只依赖 `localUploadPath` 的字符串前缀检查，因为父目录中的符号链接可能跨出上传根目录。

当前 `local` provider 执行上述文件删除。任何其他 provider 在没有相应安全删除实现时 fail closed：保留数据库行、释放 claim 供后续重试并记录归一化的 `unsupported_provider`，不得只删数据库记录来伪装清理成功。

### 9.2 有界 claim 和执行

每 5 分钟运行一批，每批最多 50 行：

1. 在短事务中按 `cleanup_after,id` 排序，以行锁/`SKIP LOCKED` 选择候选。
2. 写入随机 `cleanup_claim_token`、`cleanup_claimed_at=now`，并原子递增 `cleanup_attempts`。
3. 提交 claim 事务后逐条处理，避免持有数据库事务执行文件系统 I/O。
4. 再次按 `id + claim token + PUBLIC + owner NULL` 验证记录；验证失败只放弃，不删除文件。
5. 本地文件存在时只删除该 object key 对应的普通文件；文件已不存在视为幂等成功。
6. 物理删除成功或文件不存在后，按同一 claim 条件删除数据库行。
7. 物理删除失败时清空本次 claim 供后续周期重试；进程崩溃留下的 claim 在 10 分钟后可重新获取。

注册 claim 的条件增加 `cleanup_claim_token IS NULL`。由于 cleanup 只在 capability 过期并经过 30 分钟宽限后开始，正常有效注册不受影响；一旦 cleanup 已 claim，旧 capability 不能与删除竞态绑定。worker 在任何 owner 非空或 claim 不匹配情况下都 fail closed。

### 9.3 调度和日志

`Server.Run` 为 worker 创建可取消 context，在 HTTP server 返回时取消 ticker。单次 worker 也暴露为可注入 `now` 的窄函数，测试不依赖真实等待。

日志只允许包含：批次开始/结束、claim 数、成功数、失败数、attempt 分组和归一化错误类别。不得记录原始 IP、IP hash、HMAC secret、capability、claim token、object key、URL、本地绝对路径、文件内容或商家资料。

清理失败不会使 API 进程退出；guard 表/列缺失等结构错误需要显著日志并停止本轮。重试始终有 batch 上限，避免故障后一次扫描占满数据库或磁盘 I/O。

## 10. 前端行为

抽取一个共享 `MAX_UPLOAD_FILE_BYTES = 10 * 1024 * 1024` 和本地校验函数，供以下入口复用：

- 商家注册营业执照；
- 商品新建图片；
- 商品编辑图片。

选择文件后、调用 presign 前校验 `file.size > 0 && file.size <= MAX_UPLOAD_FILE_BYTES`。超过上限显示“图片不能超过 10 MiB”，不发送 presign 或 upload；空文件显示稳定的无效文件错误。三处说明文案统一为“原图最大 10 MiB，服务端自动压缩”。

前端检查只改善体验，不能替代后端 presign、multipart 和真实读取三层校验。现有注册 `file_token` 仍只保存在组件内存，不能因共享上传校验写入 storage。

## 11. TDD 与本地验证

所有行为修改按 RED -> GREEN 实施，并在每个任务提交中记录先失败的目标测试及失败原因。

### 11.1 后端功能测试（SQLite）

至少覆盖：

1. presign 在 10 MiB 精确边界成功，10 MiB + 1 byte 在写库前失败。
2. multipart 10 MiB 文件可进入受控处理；文件或 request body 超限返回 HTTP 413 / `10008`，处理器和文件落盘未执行。
3. 实际文件字节与 presign `size_bytes` 不一致时失败且不覆盖目标文件。
4. 可信代理和直连 IP 规范化正确，数据库只出现 HMAC，不出现原始 IP。
5. 第 20 个匿名 presign 成功，第 21 个返回 429；重建 `Server` 后限制仍存在。
6. 5 个活跃匿名记录和 50 MiB 字节边界分别生效，过期但未清理记录继续占用。
7. 商家 2 GiB、全局 20 GiB 边界及 overflow fail closed。
8. quota 拒绝不创建 `file_records`，数据库错误不降级放行。
9. 匿名绑定在 guard 下检查商家配额；失败时 merchant/account/owner 变更全部回滚。
10. 历史 `cleanup_after=NULL`、已绑定记录、认证上传均永不成为候选。
11. 新匿名过期记录被有界 claim；物理文件、记录最终删除，缺失文件幂等完成。
12. 文件删除失败释放 claim并递增 attempt；崩溃 claim 只有 TTL 后可重试。
13. cleanup 与注册/owner 改变竞态时绝不删除已绑定文件。
14. 清理日志不含 IP/hash/token/object key/path。
15. F-02 capability、F-04/F-13 私有执照和商品图公开行为全部回归。

### 11.2 配置、迁移和前端测试

- 生产配置缺少任一显式限制、secret 或 trusted proxy 声明时启动失败；10/11 之外的生产大小配置失败。
- 开发默认值和显式测试值可启动，错误信息不含 secret。
- Migration artifact 测试固定 `0008` 的列型、索引、guard、历史 NULL 保护和无 destructive down。
- 三个前端入口分别证明 `>10 MiB` 不调用 presign，精确 10 MiB 仍调用；文案和共享常量一致。
- frontend 全量 Vitest 和 production build 通过。

本地完整门禁：

```bash
make test
cd frontend && npm test && npm run build
```

若变更触及 miniapp 共享契约，才追加 miniapp 测试和两平台构建；本设计预期不修改 miniapp。

## 12. 隔离 MySQL 8.4 测试服务器验收

F-06 使用新的独立目录和 Compose project，不复用 F-02、F-04/F-13 或 F-05 授权：

```text
remote path:    /home/yu/services/secondhand-upload-governance-acceptance-20260726
compose project: secondhand-upload-governance-acceptance
```

任何传输前必须取得包含准确远程路径和白名单的单独授权。白名单只包含实施计划列出的源码、migration、测试、acceptance harness、Makefile 和依赖清单。必须排除 `.env`、密钥、数据库、上传文件、证据目录、`.git`、缓存、`node_modules`、构建产物、`backend/app.db`、miniapp 私有配置和三份受保护审查文档。

隔离验收至少证明：

1. MySQL 版本为 8.4.x，Compose 名称、网络、volume 和端口均与生产隔离。
2. `0001..0008` 正常链和每类结构漂移按预期通过/失败；重复 `AUTO_MIGRATE=true` 不制造重复列或索引。
3. migration 前 fixture 的 ID、行数、非治理字段指纹和物理 fixture 文件在 migration 后完全不变，治理字段为 NULL，cleanup 运行后仍存在。
4. 多个独立 API 实例/数据库连接在接近匿名、商家和全局边界时并发 presign，成功总量绝不超过限制，失败没有残留 reservation。
5. 并发匿名绑定与 cleanup 只有允许的一方成功，已绑定文件从不被删除。
6. stale claim、物理删除失败、物理文件缺失和 worker 重启均按重试协议收敛。
7. 经 acceptance Nginx 的 10 MiB 业务文件边界、11 MiB transport 边界和 JSON 413 envelope 可复演。
8. backend 全量测试、frontend 全量测试和 build 使用同一已审 commit 通过。
9. 本地与远端白名单源码 SHA-256 一致；证据文件只含合成数据和脱敏聚合。
10. 验收前后只读记录生产容器 ID、状态和 restart count 一致；不连接生产数据库，不读取生产上传目录，不执行生产 SQL 或部署。

成功后保留隔离资源供审核，不擅自删除。验收报告提交到 `docs/superpowers/reviews/`，记录 commit SHA、命令、测试计数、脱敏证据 SHA-256 和明确的生产未变更声明。

## 13. 文档、状态与可追溯性

实施完成后更新当前事实文档和跟踪表：

- `README.md`
- `docs/backend-api-checklist.md`
- `docs/data-model.md`
- `docs/release-readiness.md`
- `docs/full-project-code-review-2026-07-24.md`
- `docs/production-hardening-repair-plan-2026-07-24.md`
- `backend/configs/*.example`
- `deploy/acceptance/README.md`

三份受保护且未跟踪的审查文档保持原样，不修改、不暂存、不提交。对于历史 review，只在受版本控制的当前跟踪文档追加带日期的后续核验，不改写原始发现。

F-06 状态使用三个独立字段：

```text
代码侧状态：开放 / 已修复
测试服务器状态：未审核 / 审核通过
生产状态：未迁移未部署 / 已在另行批准的维护窗生效
```

设计或计划提交不能标记为“已修复”。只有实现、RED/GREEN 证据、本地全量门禁和代码复核全部通过后，才标记“代码侧已修复”；只有隔离 MySQL 8.4 验收报告完成后，才标记“测试服务器审核通过”。本任务始终保留“生产未迁移、未部署、未修改数据/文件”。

## 14. 风险与失败策略

| 风险 | 控制 |
| --- | --- |
| 全局 guard 降低 presign 并发 | 当前低频路径优先正确性；MySQL 验收记录锁等待，只有出现可测瓶颈才另案分片 |
| multipart 在字段读取时已被隐式解析 | handler 第一行安装 body cap，并显式解析后再访问任何 PostForm/FormFile |
| 伪造 `X-Forwarded-For` 绕过限制 | 生产显式 trusted proxy CIDR；`none` 只信任直连 peer；启动时拒绝默认全信任 |
| 申报小文件、实际上传大文件绕过配额 | 实际读取字节必须与 presign reservation 完全一致且不超过 10 MiB |
| cleanup 与注册竞态删除有效执照 | capability 过期 + grace、claim token、绑定要求无 cleanup claim、删除前二次 owner/claim 校验 |
| 历史记录被误清理 | migration 不回填 `cleanup_after`；查询硬要求非 NULL；MySQL fixture 指纹和负向测试 |
| 文件删除成功但删行失败 | 重试把“文件不存在”视为成功，再按 claim 删除行 |
| 文件删除失败导致无限热点 | 有界 batch、attempt 计数、claim TTL、脱敏错误分类；不阻断 API |
| 代理和应用仍给出不同上限 | 10 MiB 文件合同 + 11 MiB transport 合同分别测试；生产代理变更独立门禁 |
| HMAC 伪匿名被泄露或日志化 | 强 secret、仅存 hash、日志/响应/证据禁入、生产配置不入库 |

## 15. 验收标准

只有以下条件全部有可追溯证据，F-06 才能标记为代码侧已修复并通过测试服务器审核：

1. 前端、presign、真实文件读取和现状文档统一执行 10 MiB 文件上限。
2. 应用和 acceptance proxy 对 11 MiB multipart request body 有前置限制；过大上传得到 HTTP 413 / code `10008`，不会进入图片处理或落盘。
3. 匿名 IP 仅以可信来源的 HMAC 存储，原始 IP 和所有 secret/token/path 不进入治理日志或证据。
4. 20/hour、5 files、50 MiB、2 GiB merchant、20 GiB global 五项限制在数据库事务中生效。
5. MySQL 并发测试证明任何 presign 竞态都不能超过限制，拒绝路径没有残留记录。
6. 只有迁移后、匿名、未绑定、过期并经过 grace 的记录可被清理；历史和已绑定 fixture 在并发与重启测试后保持不变。
7. cleanup 具有有界批次、claim TTL、幂等文件缺失处理、失败重试和脱敏日志。
8. `0008` 的 preflight/up/postflight、完整迁移链和 AutoMigrate 兼容在 MySQL 8.4.x 隔离环境通过。
9. backend 全量测试、frontend 全量测试和 build 通过，代码复核没有未解决的 High/Medium 问题。
10. 验收报告记录准确 commit 和证据哈希，并明确说明未执行生产 SQL、未部署、未读取或修改生产数据及上传文件。

在生产两层 Nginx、`0008` migration 和新应用经过另行批准的维护窗之前，F-06 的生产状态必须继续标记为“未生效”。
