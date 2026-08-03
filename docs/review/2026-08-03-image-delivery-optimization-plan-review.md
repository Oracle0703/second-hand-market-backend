# 《商品图片详情级压缩与回填实施计划》审核意见

- **审核对象**：[docs/superpowers/plans/2026-07-30-image-delivery-optimization.md](../superpowers/plans/2026-07-30-image-delivery-optimization.md)
- **关联设计文档**：docs/superpowers/specs/2026-07-30-image-delivery-optimization-design.md（commit e43d975）
- **审核日期**：2026-08-03
- **审核方式**：对照当前代码库逐条核实计划引用的现有契约（media 包、app handlers、model、scripts/migrate、Dockerfile、miniapp 页面、env 示例），并审查计划自身逻辑闭环。

## 总体评价

计划引用的现有契约（`handlePresign`/`handleUploadFile`/`localUploadPath`/`ValidateRuntime`/`migrationCatalog`/`MigrateSchema`/各测试文件/env 示例/Dockerfile 结构）**几乎全部属实**，错误码 10008、上传同步置 PASS、纯本地存储等前提均成立；版本化不可变键 + 账本 + 条件更新 + dry-run/canary 的总体方向正确，TDD 节奏与任务划分合理。

但存在 **3 个会导致数据丢失或线上故障的逻辑漏洞**、若干自相矛盾与无效设计，需在动工前修正。

---

## 一、逻辑漏洞（必须修复）

### 1. 回填候选查询不排除已迁移图片 —— 换 run-id 重跑会删除线上在用的图片【严重，数据丢失】

**位置**：计划 L496（候选条件）、L521（目标键与删除旧对象）。

候选条件是 `biz_type=PRODUCT_IMAGE` + `scan_status=PASS` + 对象存在 + 被商品引用，**没有 `object_key NOT LIKE 'product_image/detail-v1/%'`**。而目标键是确定性的 `product_image/detail-v1/F<file-id>.jpg`。于是任何**使用新 run-id 的重跑**：

1. 已回填图片重新成为候选（唯一索引是 `(run_id, file_id)`，新 run-id 允许建新条目）；
2. 读"旧对象" = 当前的 detail-v1 对象，重新压缩后**覆盖写入同一个版本化键**（本身已违反全局约束"不可变前缀"）；
3. 条件更新 `WHERE id=? AND object_key=? AND scan_status='PASS'` 恰好匹配当前行 → 影响 1 行 → 判定提交成功；
4. "只有该更新影响一行后才删除旧对象" → **删掉的就是记录当前指向的唯一对象**，`file_records` 指向已不存在的文件。

**触发场景非常现实**：计划自己的发布流程是"单张 canary → 分批 apply"（L581、L703），全文没有任何地方要求 canary 与批量共用同一 run-id；设计文档也只对"清理重试"要求同 run-id。几个月后第二个维护窗口用新 run-id 重跑，同样踩中。

**修复建议**：

- 候选查询加 `object_key NOT LIKE 'product_image/detail-v1/%'`（幂等排除）；
- 删除旧对象前加 `oldKey != targetKey` 防御；
- 发布文档明确 canary 与批量必须共用同一 run-id。

### 2. STAGED 状态的崩溃恢复未对账 —— 重跑可能误删新对象【严重】

**位置**：计划 L498-L521。

提交路径：写目标对象 → 标 STAGED → 条件更新 `file_records` → 标 COMMITTED → 删旧对象。计划没有说明 `file_records` 更新与账本标 COMMITTED 是否同事务。若进程在"DB 已更新、账本未标 COMMITTED"之间崩溃，条目停留在 STAGED；同 run-id 重跑时如果按 STAGED 重新处理：条件更新的 `object_key=?` 还是旧键，匹配 0 行 → 判失败 → "新目标对象仅在尚未提交时删除" → **把其实已经生效的新对象删掉**，同样是数据丢失。

**修复建议**：续跑逻辑必须先做对账——STAGED 条目重跑时先查 `file_records.object_key` 是否已是目标键，是则直接补标 COMMITTED 进入清理，不得重新处理。计划的恢复测试（L448-L455）只覆盖了清理失败一种情况，需补崩溃窗口用例。

### 3. 过渡期设计缺失：新校验上线到回填完成之间，存量商品全部无法编辑【严重，线上故障】

**位置**：校验函数 L232-L255，发布顺序 L581。

已核实现状：`product_handlers.go` 目前对图片 file id **不做任何校验**（不查 `file_records`），商户可自由编辑。新校验要求所有图片必须 `IsDetailProductImageKey` 且 ≤ 硬上限——**回填完成前，所有存量商品的编辑请求都会被 10008 拒绝**（编辑接口会带上既有图片 id）。

计划 Task 5 的发布步骤相比设计文档**倒退了三处**：

- 丢了"**再发布后端 API**"这一显式步骤（设计文档第 3 步），只写了"确认 API 配置"；
- 丢了维护窗口必须**阻止上传、商品编辑和商品删除**的写冻结范围（设计文档明确要求；商品删除流程会在事务外删物理对象，与回填并发有真实竞态）；
- 丢了 canary/批量的 run-id 连续性说明（见漏洞 1）。

**修复建议**（二选一）：

- (a) 编辑校验对旧格式键设豁免期，回填完成后（或下个大版本）再收紧；
- (b) 把顺序定死为"迁移 0004 → 回填全部完成 → 部署新 API"，并在计划中补回写冻结范围。

无论哪种，都要把"部署新 API"显式写回发布清单。

---

## 二、自相矛盾 / 规格缺失

### 4. 账本字段与状态机语义脱节

Task 3 定义了 `LeaseUntil`、`Attempts`、`ErrorCode`、`statusSkipped`（L349-L367、L508-L519），但 Task 4 全文**没有任何地方说明**：租约何时写入/过期后如何接管、Attempts 何时递增、重跑是否重试 FAILED 条目。`statusSkipped` 更是无人使用——dry-run 不建账本行、不合格候选只输出 JSON 行"不修改业务数据"，没有任何路径会把条目标成 SKIPPED。

### 5. `--workers` 与单并发约束互相矛盾，且无程序化互斥

全局约束要求"单并发"（L18），生产强制 `--workers=1`（L494），却仍暴露 `--workers` 参数并为此设计租约字段——一套永远不会启用的并发机制。同时**没有任何数据库级互斥**（无 run 行锁、无 advisory lock）防止两个 apply 并发；结合漏洞 1，误开两个不同 run-id 的 apply 是破坏性的。

**建议**：删掉 `--workers`（或内部固定 1），并加简单互斥（如对 run 表 `GET_LOCK`，或启动时检查存在未 Finished 的 run 即退出）。

### 6. `IMAGE_DETAIL_MAX_EDGE` 可配置，但候选网格写死

L121-L126：`Edges` 固定 `{1280,1120,960}`；L226：`server.go` 把配置的 MaxEdge 复制进 policy。若运维设 `IMAGE_DETAIL_MAX_EDGE=800`，候选仍会生成 1280/1120px，随后被 handler 的"最长边不超过配置值"检查全部拒绝 → **所有商品图上传失败**。

**建议**：要么 Edges 从 MaxEdge 派生，要么 MaxEdge 不可配置。

### 7. 全局约束"严格不超过 500KB"与可配置硬上限矛盾

L14 把 `500*1024` 写成硬性全局约束，L224 却做成 env 可调且校验只要求 `target <= hard <= FileUploadMaxBytes`——运维设 `IMAGE_DETAIL_HARD_LIMIT_KB=2000` 即合法突破"严格 500KB"。

**建议**：硬上限固化为常量（推荐，与目标陈述一致），或修改目标陈述承认它只是默认值。另外 `IMAGE_DETAIL_TARGET_KB` 单位是 KB、字段名是 `...Bytes`，命名也是隐患。

---

## 三、不合理设计

### 8. 详情页 Swiper 内的 `lazyLoad` 无效

已核实详情页图片是 Taro `Swiper/SwiperItem` 轮播（`miniapp/src/pages/product/detail/index.tsx:67-82`）。微信小程序官方明确：`Image` 的 `lazy-load` **只对 page 与 scroll-view 下的图片有效**，Swiper 内不生效。于是 L636-L643 的 `lazyLoad={idx > 0}` 编译通过、行为为零——而配套测试是源码正则断言（L620），**测试全绿但没有任何运行时效果**，正好掩盖这一点。首页/分类/卡片在页面滚动上下文里 `lazyLoad` 有效，只有详情页这部分无效。

**建议**：按 `Swiper` 的 `current` 只给当前 ±1 项赋 `src`（其余项渲染占位），切换时再水合。

### 9. 15 个候选全量生成，无短路

Task 1 要求生成 3 边 × 5 质量全部候选再交给 `Select`（L101、L145）。现有 `vips_cli_processor.go:108-110` 已有"首个达标即返回"的先例。按保真度降序迭代、记录首个 ≤ 硬上限的兜底、命中首个 ≤ 目标即停，结果与全量生成完全等价且确定性相同，典型图片 2~3 次编码即可，避免每张图固定 15 次 vips 调用——对单并发跑全量历史图的回填命令是数倍总时长的差异。

### 10. `PassthroughProcessor` 的 detail 重编码没有支撑

L147 要求它为 detail profile"重新编码小尺寸 JPEG"。现状：它只能解码 JPEG/PNG（WebP/HEIC 直接 fail closed），且**完全没有缩放能力**（go.mod 无 `golang.org/x/image`，现行实现原尺寸重编码）。计划既没提新依赖，也没写清"非生产驱动仅支持 JPEG/PNG 源"的限制，实现者会在这里卡住或被迫临时加依赖。

### 11. GORM 模型与手写 SQL 双份 schema，无一致性保障

测试库靠 `MigrateSchema`（AutoMigrate），生产靠手写 `0004_....sql`，两者靠人肉对齐，没有任何测试断言列类型/索引等价。

**建议**：在 `scripts/migrate` 测试里用 AutoMigrate 产物与 SQL 语句做关键列/索引比对，至少覆盖唯一索引 `uk_backfill_run_file`。

---

## 四、无效/多余设计（建议删除）

| 项 | 位置 | 原因 |
| --- | --- | --- |
| `statusSkipped` 常量 | L514 | 无任何写入路径（见 #4） |
| `--workers` 参数 | L489 | 生产恒为 1，纯死代码（见 #5） |
| "先部署能解析 `/uploads/...` 的小程序" | L581 | 已核实现行小程序 `resolveAssetURL` 早已支持 `/uploads` 相对路径，该步骤无实际工作；应改为"确认线上版本已含此能力"的检查项 |
| `writeJSON func(io.Writer, any)` 的 Writer 参数 | L412 | 默认绑定固定 `os.Stdout` 编码器，参数冗余 |
| `summary` 无清理计数 | L415-L421 | `CLEANUP_PENDING → DONE` 的续跑结果在汇总里不可见，与"核对 CLEANUP_PENDING 为零"的发布要求不匹配 |

---

## 五、小问题与需确认项

- **目标键命名与设计文档不一致**：设计文档写 `F<业务流水号>`，回填实际用 `F<file-id>`。`BuildBizNo` 是"F+14 位时间戳+4 位序号"（`backend/internal/common/idgen.go:11`），与自增 file-id 实际不可能碰撞，但两份文档应对齐口径。
- **Cache-Control 断言位置不明**：L201-L203 的测试片段出现在上传流程测试里，但该头是 `handlePublicUpload`（GET）设置的，应写明是对 GET 响应的断言。另外已核实当前 `/uploads` **完全没有 Cache-Control**，回填删除旧对象后旧 URL 即 404，浏览器启发式缓存可能短暂续命——影响可控，建议在发布文档写明。
- **已合规图片也被重编码**：没有"已达标则跳过"路径，每张历史图都经历一次 JPEG 代际损失。作为设计取舍可以接受，但未在任何地方声明。
- **商户多用户问题需确认**：校验用 `*file.UploaderID != actor.UserID`（L247），而 `Actor` 同时有 `UserID` 和 `MerchantID`。若同一商户允许多个账号上传/编辑，应按 `MerchantID` 校验，否则 A 传的图 B 不能用来建商品。
- **生产镜像 HEIC 支持已落实**（Dockerfile 运行阶段有 `libheif1 libvips-tools`），但 HEIC 集成测试只在开发机有 vips 时运行，镜像本身无 smoke；建议在 Task 5 加一条镜像内 `vips copy heic→jpg` 冒烟。

---

## 六、修复优先级建议

1. **漏洞 1 + 2**（回填删除线上对象）：改候选查询 + 删除防御 + STAGED 对账——回填命令上线的前置条件；
2. **漏洞 3**（过渡期编辑全拒 + 发布顺序倒退）：定死部署顺序并把写冻结范围补回 Task 5；
3. **#8**（Swiper 懒加载无效）：换成 current±1 水合方案，否则 Task 6 的一半价值不成立；
4. #4~#7 的规格矛盾在动工前对齐；
5. #9~#11 与第四节可在实现期顺手处理。

---

## 附：审核中已核实的代码事实（结论依据）

| 事实 | 证据 |
| --- | --- |
| `ProcessRequest` 当前无 `OutputProfile` 字段 | backend/internal/media/processor.go:58-62 |
| 现有 vips 处理器保持原格式、仅有 autorot/copy/resize，已有达标短路 | backend/internal/media/vips_cli_processor.go:62,80-110,152-158 |
| `PassthroughProcessor` 仅支持 JPEG/PNG、无缩放能力 | backend/internal/media/processor.go:90-135 |
| `common.ErrInvalidUpload` = 10008；`BuildBizNo` = 前缀+时间戳+4位序号 | backend/internal/common/errors.go:14,43；idgen.go:11 |
| 上传同步置 PASS，无异步扫描 | backend/internal/app/file_handlers.go:235-246 |
| `/uploads` 当前无 Cache-Control 头 | backend/internal/app/file_handlers.go:338-380 |
| 商品创建/编辑当前完全不校验图片 file id | backend/internal/app/product_handlers.go:62-74,162-176 |
| `FileRecord.URL`/`UploaderID *uint64`、`Product.CoverFileID`、软删除、`ProductImage` 均存在 | backend/internal/model/models.go:135-212 |
| `migrationCatalog`/`loadMigrationStatementsFromDir`、SHA-256 白名单存在（现有 0001-0003） | backend/scripts/migrate/main.go:27-43,137-169 |
| Dockerfile 当前只构建 server，运行阶段有 `libvips-tools`+`libheif1` | backend/Dockerfile:9,14,19 |
| 仅支持本地磁盘存储，无对象存储 | backend/internal/app/server.go:51-57 |
| 详情页为 Swiper 轮播；首页/分类封面 className 与计划一致 | miniapp/src/pages/product/detail/index.tsx:67-82；home/index.tsx:240；category/index.tsx:212 |
| 小程序 `resolveAssetURL` 已支持 `/uploads` 相对路径 | miniapp/src/utils/url.ts:14-29 |
| 设计文档候选查询同样未排除 detail-v1；维护窗口写冻结要求在设计文档中存在 | docs/superpowers/specs/2026-07-30-image-delivery-optimization-design.md |
