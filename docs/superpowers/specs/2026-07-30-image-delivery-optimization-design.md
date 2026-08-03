# 商品图片详情级压缩与回填设计

## 背景

生产环境当前已落盘的商品图片大多是手机原图。已核验的 15 张有效商品图共约 65.87 MiB，平均约 4.39 MiB，最大约 7.44 MiB；其中多数为 `5712 x 4284`。现有图片处理器的目标仅为 20 MiB，因此这些图片不会继续缩放，小程序列表和详情页仍会下载数 MiB 的单张图片。

本次只处理商品图片（`PRODUCT_IMAGE`）：上传时不保存原图，只保存一份可同时用于列表、封面和详情的 JPEG 压缩图；生产已存在的有效商品图片通过独立运维命令一次性回填。

## 目标与边界

| 项目 | 确定规则 |
| --- | --- |
| 适用范围 | 仅 `PRODUCT_IMAGE`；不改变营业执照、头像及其他业务文件的处理规则 |
| 最终格式 | `image/jpeg`，对象键扩展名固定为 `.jpg` |
| 尺寸 | 等比缩小、不放大，最长边不超过 `1280px` |
| 体积目标 | 优先命中 `300 * 1024` bytes（下文称 300KB） |
| 硬上限 | 严格不超过 `500 * 1024` bytes（下文称 500KB） |
| Profile 稳定性 | `detail-v1` 的尺寸、体积和质量阶梯是代码常量，不能通过环境变量改变；任何语义变化必须发布新 profile |
| 原图 | 新上传成功后不保留原始字节；历史对象在数据切换后至少保留 24 小时，过期并通过安全校验后删除 |
| 存储变体 | 每个商品文件只保留一份 `detail-v1` JPEG，不生成额外封面或缩略图 |
| 存量处理 | 独立、可中断续跑的命令；绝不在 API 服务启动时自动回填 |

本次不引入 CDN、异步队列、水印、人工内容审核、客户端本地压缩或 `list-v1` 缩略图。原始上传大小上限继续保持现有 40 MiB；超过该限制或无法压到 500KB 的文件都拒绝上传，不允许回退保存原始字节。列表使用详情级单图是本期明确的 MVP 边界，后续是否增加多尺寸变体必须基于真实流量和清晰度数据另行设计。

## 方案选择

| 方案 | 结论 | 原因 |
| --- | --- | --- |
| 保持原格式并仅降低质量 | 不采用 | PNG、HEIC 等格式很难稳定满足 500KB，且文件扩展名与实际内容的一致性更复杂 |
| 前端或小程序本地压缩 | 不采用 | 设备能力和实现不一致，无法约束管理端、API 和历史文件 |
| 服务端统一 JPEG + 版本化对象键 | 采用 | 体积和格式可控，保留同一个 `file_id`，同时利用新 URL 避免客户端继续使用旧缓存 |

## 图片处理契约

### 输入、输出与 MIME 边界

服务端接受 JPEG、PNG、WebP、HEIC、HEIF 声明类型，但 `detail-v1` 分支只以实际文件字节检测和 libvips 解码结果判断源格式。文件名、表单 MIME 和扩展名都不能绕过内容校验；请求声明的源 MIME 只用于允许类型的前置校验，不作为最终格式判据。

预登记记录中的 `mime_type=image/jpeg` 表示预期输出格式，不是源文件的 MIME claim。`detail-v1` 处理器不得拿这个预登记输出 MIME 与源字节检测结果做一致性比较，否则 PNG、WebP、HEIC 和 HEIF 会被错误拒绝。处理完成后必须重新校验输出魔数和解码结果为 JPEG，再把最终 MIME 写回文件记录。该例外只属于 `PRODUCT_IMAGE/detail-v1`，其他业务文件继续保持现有源 MIME claim 匹配规则。

### 不可变 Profile

| 常量 | `detail-v1` 固定值 |
| --- | --- |
| 最长边 | `1280px` |
| 尺寸候选 | `1280`、`1120`、`960`，按降序 |
| JPEG 质量候选 | `82`、`78`、`74`、`70`、`66`，按降序 |
| 目标体积 | `300 * 1024` bytes |
| 硬上限 | `500 * 1024` bytes |
| 单图处理超时 | 60 秒，并继承上游 context 的更早取消时间 |

其中最长边、尺寸候选、质量候选、目标体积和硬上限属于输出常量，不得暴露为可改变 `detail-v1` 语义的运行时配置。处理超时属于资源保护边界，不改变成功输出的 profile；未来如需调整输出常量，必须使用 `detail-v2` 和新的不可变对象键前缀。

`PRODUCT_IMAGE` 固定执行以下步骤：

1. 解码并校验为允许的静态图片。
2. 应用 EXIF 方向（`autorot`），转为 sRGB，移除 EXIF 等元数据。
3. 对透明图片以白色背景合成。
4. 按候选最长边等比缩小，绝不放大。
5. 编码 JPEG，并校验输出魔数、MIME、尺寸和字节数。

候选生成必须有明确上界和短路规则，而不是先物化全部结果：

1. 在单图 60 秒截止时间内，按“边长降序、质量降序”依次生成候选，最多尝试 `3 x 5 = 15` 个。
2. 保留遇到的第一个不超过 500KB 的候选作为硬上限兜底。由于遍历顺序固定，它就是当前已生成结果中画面尺寸最大、质量最高的合格项。
3. 命中第一个不超过 300KB 的候选后立即返回，不再继续编码更小尺寸或更低质量候选。
4. 全部候选处理完仍未命中 300KB 时，返回已记录的 500KB 兜底；没有兜底则处理失败。
5. HTTP 请求取消、回填任务取消和显式截止时间必须一路传递到 `exec.CommandContext`，超时后终止 vips 子进程且不保存半成品。

这保持“画面尺寸优先、同尺寸质量优先”的确定性排序，同时让 300KB 继续作为明确的产品目标，并保证任何成功产物都绝不超过 500KB。

### 非生产 Passthrough 边界

`PassthroughProcessor` 只服务本地开发和窄范围单元测试，不承诺复刻 vips 的完整候选阶梯：

1. 仅接受实际字节可解码为 JPEG 或 PNG 的静态小图，不能以声明 MIME 代替字节检测。
2. 统一重新编码为 JPEG；透明 PNG 使用白底。
3. 不执行缩放；输入最长边超过 1280px 或输出超过 500KB 时直接失败。
4. WebP、HEIC、HEIF、候选选择和完整 profile 行为通过 fake processor 及真实 vips 集成测试覆盖。

生产环境必须使用 vips，配置为 passthrough 或未知处理驱动时拒绝启动，也不允许在 vips 失败后回退到 passthrough 或原图。

### 错误语义

| 情况 | 结果 |
| --- | --- |
| 伪装文件、损坏图片、无法解码、输出不是 JPEG | 返回现有非法上传错误；文件记录保持 `PENDING` |
| vips 缺失、超时、任务取消、临时文件 I/O 失败 | 返回内部处理错误；文件记录保持 `PENDING`，可重试 |
| 无候选满足 500KB | 返回非法上传错误；不保存任何原始或超限字节 |
| 新上传的数据库更新失败 | 删除刚写入且尚未被引用的新对象，保留 `PENDING` 记录；不得返回成功 |

`confirm` 不能把未成功处理的 `PENDING` 文件提升为 `PASS`。这是现有安全不变量而非本期新增能力，本次必须保留对应回归测试。

## 上传、关联与访问链路

### 新上传

`POST /files/presign` 在 `biz_type=PRODUCT_IMAGE` 时生成 `product_image/detail-v1/{BuildBizNo("F")}.jpg`。`BuildBizNo("F")` 的结果本身已经带 `F` 前缀，调用方不得再额外拼接一个 `F`。预登记 MIME 固定为最终输出 `image/jpeg`，源格式仍由上传字节检测。

`POST /files/upload` 将原始请求体读入受 40 MiB 上限约束的临时处理范围，调用 `detail-v1` 处理器，再以原子且不覆盖的方式发布对象：先写同目录临时文件并关闭、fsync、校验，再使用 Linux `renameat2(RENAME_NOREPLACE)` 或同文件系统的 hard-link publish 等 no-replace 原语创建最终键，最后 fsync 目录并清理临时文件。普通 `os.Rename` 可能覆盖目标，不能用于版本化对象发布。目标已存在时必须返回冲突且保持既有字节不变。成功后以一个数据库更新同步 `object_key`、`url`、`mime_type=image/jpeg`、`size_bytes` 和 `scan_status=PASS`。任何处理或校验失败都不留下本次新建的可访问对象。

`FileRecord.ID` 保持不变，因此 `product_images.file_id`、`products.cover_file_id`、收藏、浏览历史和意向记录都不需要迁移。API 返回的图片 URL 仍由同一文件记录计算。

### 商品关联校验与过渡开关

商品创建和编辑对请求提交的 `image_file_ids` 去重后批量校验文件存在、`biz_type=PRODUCT_IMAGE`、`scan_status=PASS` 和商户归属，任一文件不合格都使整个创建或更新事务失败。商户归属按 `MerchantID` 判断：通过文件上传账号关联的 `merchant_accounts.merchant_id` 与当前操作者所属商户比较，不能直接比较上传账号和编辑账号的 `UserID`，从而允许同一商户的 OWNER、STAFF 共同维护商品。历史归属解析必须包含软删除的商户账号；无法稳定解析 `MerchantID` 的记录作为阻断异常处理，不能降级成 `UserID` 相等。

管理账号上传的 `PRODUCT_IMAGE` 没有商户归属，不能直接关联到商家商品。本期不扩展 admin 代商家维护商品图片的业务流程。

商品删除及其文件回收使用相同的 `MerchantID` 归属规则，不能继续按当前操作者的上传账号 ID 筛选文件。删除文件记录前的引用计数必须同时覆盖 `product_images.file_id` 和其他商品的 `products.cover_file_id`；只有两类引用都为 0 才允许回收。该规则保证 OWNER 上传的图片可以由同商户 STAFF 正常维护和删除，同时不会误删被其他商品共用的封面。

新增发布开关 `REQUIRE_DETAIL_V1_PRODUCT_IMAGES`，只控制关联时是否强制图片已经位于 `product_image/detail-v1/`：

| 开关值 | 行为 |
| --- | --- |
| `false` | 仍执行存在性、业务类型、`PASS` 和 `MerchantID` 校验，但允许存量非 `detail-v1` 图片继续随商品保存；用于第一阶段部署和回填窗口 |
| `true` | 在上述校验基础上要求对象键位于 `detail-v1` 前缀、扩展名为 `.jpg`、记录 MIME 为 `image/jpeg` 且 `0 < size_bytes <= 500 * 1024`；仅在历史引用清零后启用 |

该开关不是覆盖全部 API 的全局 maintenance flag。上传、商品图片编辑和商品删除的写冻结仍由发布维护窗口明确执行，避免扩大一次性迁移对其他业务接口的行为面。

开关 false 和 true 使用同一套共享校验谓词，差异只能是是否要求 `detail-v1` 元数据。第一阶段部署前必须对全部 `product_images.file_id` 与 `products.cover_file_id` 做基础谓词预检；第二阶段切换前必须运行完整严格谓词，确保键、扩展名、MIME、体积、`PASS`、业务类型和 `MerchantID` 违规数以及未处置阻断异常数均为 0。生产环境必须显式配置该布尔值，缺失或非法值拒绝启动。

### 缓存与小程序加载

`product_image/detail-v1/` 是不可变对象键前缀。`GET /uploads/...` 对该前缀设置 `Cache-Control: public, max-age=31536000, immutable`；其他业务文件保留现有响应策略。以后变更图片内容必须创建新的版本化键，不能覆盖已有版本化对象。

小程序商品卡片、首页商品流和分类列表的 `Image` 组件启用 `lazyLoad`。详情轮播不依赖 Swiper 内 `Image.lazyLoad` 是否在微信、抖音两端具有一致语义，而是使用受控 `Swiper.current`：首图立即加载，任何时刻只为当前项及相邻项设置真实 `src`，其余项使用不触发远程请求的占位值。`current` 改变后同步更新真实 `src` 集合。

列表和详情仍使用同一份详情级 JPEG，不新增缩略图存储。上传前的本地 `createObjectURL` 只用于选图预览，不承诺与服务端最终 JPEG 逐像素一致；上传完成后的详情和再次进入编辑页必须读取服务端 URL。

## 生产存量回填

### 候选、排除与 dry-run

新增 `backend/scripts/backfill_product_images` 一次性命令，不通过 HTTP 上传接口，也不创建 API Server。候选仅包含同时满足下列条件的 `file_records`：

1. `biz_type = PRODUCT_IMAGE` 且 `scan_status = PASS`；
2. 文件被 `product_images.file_id` 或 `products.cover_file_id` 引用；
3. `object_key` 不位于 `product_image/detail-v1/`；
4. 实际对象存在且可读；
5. 按 `file_records.id` 去重。

位于 `detail-v1` 前缀的记录不进入回填；命令仍需校验其 MIME、尺寸、体积和对象是否一致，发现异常只写异常清单，不能覆盖不可变对象。旧图即使已经小于 1280px 和 500KB，只要尚未处于 `detail-v1`，仍需重编码，以统一 JPEG、sRGB、方向、白底、元数据和版本化键契约。

营业执照、未引用的孤儿文件、`PENDING` 文件、缺物理文件、悬空封面及跨 run 未解决冲突不作猜测性修复。它们只写入 JSON Lines 异常清单，不创建 `SKIPPED` 账本项。已核验的生产数据中预计有 11 条有效候选；该数量只是运行前基线，实际执行前必须重新 dry-run 复核。

`--dry-run` 默认执行与 apply 相同的候选读取、真实解码和候选压缩，但只输出源/预计输出尺寸、源/预计输出字节数、命中边长与质量、目标键和失败原因。dry-run 可以使用临时文件，结束后必须清理；不得写最终对象、`file_records`、商品关联、运行账本或条目账本。汇总字段使用“已评估数量”，不能把只枚举数据库记录误称为已处理。

### 账本与状态机

新增显式前向迁移，创建 `image_backfill_runs` 与 `image_backfill_items`。迁移通过现有显式迁移命令执行并使用 `CREATE TABLE IF NOT EXISTS`；本系统采用前向修复策略，不新增生产工具不会执行的 down migration。

账本至少记录：运行 ID、文件 ID、源/目标对象键、profile 版本、源/输出 SHA-256、源/输出大小、处理状态、尝试次数、错误码、提交时间、`cleanup_after` 和清理状态。保留 `(run_id, file_id)` 唯一约束，不增加 `file_id` 全局唯一约束，以允许未来同一文件迁移到 `detail-v2`。账本是独立审计记录，不得通过外键级联随 `file_records` 或商品删除，否则延迟清理将失去恢复依据。

单项处理状态如下：

| 状态 | 含义与允许转移 |
| --- | --- |
| `PENDING` | 已为当前 run 建立账本，等待处理 |
| `PROCESSING` | 本次尝试开始；每次进入都将 `attempts` 加一 |
| `STAGED` | 新对象已原子写入并通过输出哈希校验，业务记录尚待事务确认 |
| `COMMITTED` | `file_records` 已切换到目标对象，且清理计划已在同一事务落账 |
| `FAILED` | 处理、校验或冲突失败并记录 `error_code`；只能在相同 run 显式传入 `--retry-failed` 后回到 `PROCESSING` |

清理状态独立于主状态：

`NOT_SCHEDULED -> PENDING -> DONE | FAILED`；同一 run 再次执行清理时，`FAILED` 可以回到 `PENDING` 后重试。

`cleanup_after` 必须不早于 `committed_at + 24h`。主状态不包含 `SKIPPED`，也不使用租约字段；未入选记录由异常清单表达，单实例互斥由全局锁保证。

命令参数固定为：

| 参数 | 行为 |
| --- | --- |
| `--dry-run` | 默认模式；真实解码和压缩估算，只读业务数据且不写对象或账本 |
| `--apply` | 创建或继续账本并执行回填；必须取得全局变更锁 |
| `--cleanup` | 仅清理同一 run 中已到期旧对象；必须取得与 apply 相同的全局变更锁 |
| `--run-id` | apply 和 cleanup 必填；canary、后续批次、失败重试和清理必须复用同一个 ID |
| `--after-id`、`--limit` | 只用于 dry-run/apply 的单张 canary 和分段处理；cleanup 总是扫描指定 run 的全部到期项 |
| `--retry-failed` | 仅与 `--apply` 及原 run-id 一起使用，显式重试该 run 的 `FAILED` 项 |

`--dry-run`、`--apply`、`--cleanup` 三种模式必须且只能选择一种；都未显式指定时默认 dry-run。apply/cleanup 必须提供 run-id，`--retry-failed` 只能用于 apply，非法组合在连接数据库或访问对象存储前直接失败。

JSON Lines 输出保留可注入 Writer 的契约：`writeJSON func(io.Writer, any) error`，默认实现为 `func(w io.Writer, v any) error { return json.NewEncoder(w).Encode(v) }`。命令入口默认传入 stdout，测试传入 buffer；实现不能把序列化函数固定绑定到 `os.Stdout`。

命令不提供 `--workers`，内部固定单并发。由于生产显式迁移只支持 MySQL，apply/cleanup 也只允许 `DB_DRIVER=mysql`；SQLite 仅可执行只读 dry-run，变更模式必须 fail closed，除非未来另行设计等价迁移和跨进程锁。

所有 apply/cleanup 实例在命令开始时通过专用、固定的 MySQL 连接获取同一个数据库级全局 named lock，锁与 run-id 无关，并保持到汇总完成后显式释放。账本及业务数据库变更绑定该连接；独立监测协程持续检查连接，连接丢失必须取消整个任务，后续禁止写对象或数据库并以非零状态退出。目标发布仍使用 no-replace 原语，因此极短的失锁竞态最多留下可对账的未引用目标，不能覆盖或删除另一个实例的有效对象。条目状态不再使用租约模拟跨进程互斥。

### 写入、恢复与延迟清理

新上传键使用业务流水号，历史回填目标键固定为 `product_image/detail-v1/F{file_id}.jpg`。两种命名用途不同，但目标都必须稳定且不可覆盖。目标已存在时，只有同一 run 的遗留 `PROCESSING` 或 `STAGED` 在按下述恢复协议证明输出哈希完全一致后才能复用；其他情况记录冲突并停止，不得覆盖对象。

单项写入顺序如下：

1. 获取全局锁，创建或加载当前 run 的账本项；发现其他 run 对同一文件和 profile 留有未解决条目时停止并报告，不能换 run 绕过失败重试规则。
2. 读取旧对象并计算源哈希，在受超时约束的临时范围生成和验证 `detail-v1` JPEG。
3. 使用与新上传相同的 no-replace 原语原子发布目标键，完成文件及目录 fsync 和输出哈希校验，再把条目标记为 `STAGED`；普通 `os.Rename` 不满足该步骤。
4. 在同一个短数据库事务中，以 `WHERE id=? AND object_key=? AND scan_status='PASS'` 条件更新 `file_records` 的对象键、URL、MIME 和最终大小，同时把账本标为 `COMMITTED`，写入 `committed_at`、`cleanup_after` 和 `cleanup_status=PENDING`。
5. 事务提交后继续保留源对象，退出本次处理；不得在回填主流程中立即删除旧对象。

`file_records` 更新和账本 `COMMITTED` 不允许拆成两个事务。事务失败时两者一起回滚，条目保留为可对账的 `STAGED`；不能仅因条件更新影响 0 行就删除目标对象。

同一 run 重新取得全局锁后，账本中遗留的 `PROCESSING` 可以确定为上一次进程被中断，而不是仍有另一个 worker 在处理。恢复时将 `attempts` 加一并先检查业务记录和目标对象：业务记录仍指向源键且目标不存在时重新开始处理；目标存在时重新生成预期结果并比较哈希，完全一致才补齐输出元数据并进入 `STAGED`；业务记录已指向目标键时也必须先验证目标，再交给下述 `STAGED` 对账；其他键值或哈希冲突转为 `FAILED`，且不删除任何对象。

续跑遇到 `STAGED` 时必须先对账：

| `file_records.object_key` | 恢复动作 |
| --- | --- |
| 仍为 `source_key` | 校验目标对象及输出哈希后，重新执行同一短事务；不重新压缩 |
| 已为 `target_key` | 防御性校验目标对象、哈希和业务元数据，符合时按正常提交不变量补齐 `COMMITTED`、`committed_at`、`cleanup_after` 和 `cleanup_status=PENDING`；不得删除目标对象 |
| 指向其他键或对象/哈希不符 | 标记冲突并保留源、目标对象，等待人工处理 |

旧对象只能由 `--cleanup --run-id <同一 ID>` 在到期后处理。每项必须先确认 `source_key != target_key`、主状态为 `COMMITTED`、清理时间已到且没有业务记录仍指向源键；任何仍存在的源或目标对象都必须与账本哈希一致。随后只允许进入以下安全分支：

| 当前状态 | 清理动作与完成条件 |
| --- | --- |
| 文件记录仍存在并指向 `target_key` | 目标对象必须存在且输出哈希匹配；只删除源对象，源对象不存在后记为 `DONE` |
| 文件记录已被正常删除，且商品图片和封面引用均为 0 | 分别删除仍存在且哈希匹配的源对象和目标对象；两者均不存在后记为 `DONE`，从而收敛商品删除时失败的物理目标清理 |
| 文件记录指向其他键、目标缺失但记录仍存在、仍有业务引用或任一哈希不符 | 不删除任何对象，记为 `FAILED` 并保留错误码供同一 run 重试或人工处理 |

源/目标对象删除与更新清理状态无法跨文件系统和数据库组成一个原子事务，因此清理必须幂等。安全分支的完成条件已经满足时直接对账为 `DONE`；实际删除成功但状态落库失败时保持非零退出，后续使用同一 run-id 重跑并通过相同条件收敛。账本不得因文件记录先行删除而消失。

汇总必须分别输出已评估、已提交、处理失败、待清理、清理完成、清理失败和异常清单数量。数据切换后的 24 小时保留期用于覆盖客户端仍持有旧 URL 的窗口；它不是长期保留高清原图的策略。

### 写冻结

apply 期间必须安排明确的写冻结。网关或既有运维入口至少阻断 `POST /api/v1/files/presign`、`POST /api/v1/files/upload`、`POST /api/v1/files/confirm`、`POST /api/v1/merchant/products`、`PUT /api/v1/merchant/products/:id` 和 `DELETE /api/v1/merchant/products/:id`；当前商户编辑页每次保存都会提交图片 ID，因此更新路由需要整体冻结。商品查询和图片读取继续提供服务。

冻结开启后必须等待在途写请求归零，再运行探针确认上述写接口返回维护状态、商品读接口和 `/uploads/...` 仍可访问。现有商品删除会在事务外删除物理对象，不能与回填并发运行。解除冻结前也必须确认所有 API 实例使用同一配置版本。

本期不增加覆盖所有写 API 的应用级 maintenance flag。发布负责人需要在网关或既有运维流程中实施上述范围的冻结，并在执行前确认冻结生效；全局回填锁只解决多个回填进程之间的互斥，不能替代 API 写冻结。

当前流程只适用于一次性小批量回填。若 dry-run 候选超过 100 条，或按单并发实测预计 apply 超过 30 分钟，必须停止本次发布并重新评审应用级维护模式、批处理窗口或异步方案，不能直接扩大当前人工维护窗口。

## 配置、部署与发布顺序

生产容器已安装 `vips 8.14.1`，实际上传卷挂载为宿主机 `/home/yu/services/secondhand-market/runtime/uploads` 到容器 `/data/uploads`，Web 的 `/uploads/` 已反向代理给 API。回填命令必须通过同一镜像、同一容器网络和同一上传卷执行，不能在宿主机硬编码不存在的 `/data/uploads` 路径。

`detail-v1` 不新增可改变最大边、目标体积、硬上限或质量阶梯的环境变量。生产启动时仍必须校验图片处理驱动为 vips；未知驱动或 passthrough 直接拒绝启动。发布相关配置如下：

| 配置 | 第一阶段 | 第二阶段 |
| --- | --- | --- |
| `REQUIRE_DETAIL_V1_PRODUCT_IMAGES` | 显式设为 `false`，允许存量图片随商品保存 | 完整严格谓词清零后显式改为 `true` |
| `DB_DRIVER` | 回填 apply 固定为 `mysql` | cleanup 仍固定为 `mysql` |
| `AUTO_MIGRATE` | `false` | `false` |
| `SEED_DEFAULTS` | `false` | `false` |
| `FILE_PUBLIC_BASE_URL` | 空值，由 API 的相对 `/uploads/...` 路径提供访问 | 保持不变 |

发布步骤：

1. 记录并确认线上小程序版本号或 commit 已包含相对 `/uploads/...` 解析能力；仓库当前支持不等于线上版本已经发布。
2. 构建包含 API、显式迁移命令、回填命令和 migrations 的镜像，执行镜像内 vips codec smoke，但不自动运行回填。
3. 使用新镜像执行只读预检：运行开关 false 对应的基础关联谓词与真实 dry-run，提前识别 PENDING、缺记录、跨商户、无法解析归属等阻断异常，并检查候选数量和预计耗时没有触发规模停止条件。
4. 在网关开启规定范围的写冻结，等待在途写归零并执行读写探针；在冻结快照下重新运行基础谓词和 dry-run。基础违规或阻断异常不为 0 时先修复并复检，无法修复则不进入迁移和部署。
5. 显式执行账本前向迁移并核验表结构；以 `REQUIRE_DETAIL_V1_PRODUCT_IMAGES=false` 滚动发布全部 API 实例。确认实例版本和配置修订一致后，通过维护绕行入口验证 JPEG/PNG/WebP/HEIC/HEIF 新上传、旧商品保存、图片展示和 GET 缓存头，普通外部写流量仍保持冻结。
6. 创建唯一 run-id，以 `--apply --run-id <ID> --limit 1` 做单张 canary；验证详情、封面、列表、新旧 URL、对象哈希和账本后，使用同一 run-id 分批完成其余候选。
7. 对账并确保没有 `PROCESSING` 或 `STAGED` 活跃项，再对全部有效商品引用执行与开关 true 完全相同的严格谓词。只有严格违规、阻断异常和未处置的相关 `PENDING/FAILED` 均为 0，才能把全部 API 实例滚动到 `REQUIRE_DETAIL_V1_PRODUCT_IMAGES=true`。
8. 在网关仍冻结时确认所有实例配置一致，并通过维护绕行入口验证商品编辑；随后解除写冻结。旧对象继续保留，不要求维护窗口持续 24 小时。
9. 最早在各条目 `cleanup_after` 到期后，使用同一 run-id 执行 `--cleanup`；核对待清理、完成和失败汇总，失败项保留对象并重试或人工处置。

若 canary、批次或严格谓词检查失败，立即停止继续 apply，在冻结状态下把遗留 `PROCESSING/STAGED` 对账为 `COMMITTED` 或 `FAILED`。确认全部实例仍为开关 false、基础谓词可通过且读链路正常后，可以解除冻结；已提交图片与未迁移旧图可在兼容模式共存，后续必须复用原 run-id 继续。若基础谓词无法恢复，则回滚 API 版本并保持对象和账本，不得带着不兼容校验恢复写流量。

## 测试与验收

| 层级 | 必测内容 |
| --- | --- |
| media 单元测试 | JPEG 输出、最长边不超过 1280、EXIF 旋转、透明 PNG 白底、去元数据、300KB 早停、500KB 兜底与硬拒绝、最多 15 候选、超时/context 取消、vips 失败不回退原图、passthrough 窄契约 |
| vips/镜像集成 | JPEG、PNG、WebP、HEIC、HEIF 实际字节均输出 JPEG；生产镜像内验证 HEIC/HEIF codec、API/迁移/回填二进制和 migrations 存在 |
| 上传集成测试 | 预签名键为 `{BuildBizNo("F")}.jpg`；预登记输出 MIME 不作为源 claim；成功后字节、MIME、对象键、URL、数据库大小一致；目标已存在时 no-replace 冲突且既有字节不变；失败无本次新建的可访问文件；confirm 不能提升 `PENDING` |
| 关联与删除测试 | 批量校验保持事务原子性；开关 false 时允许同商户存量图保存，true 时拒绝键、扩展名、MIME 或体积不合规的图片；共享谓词覆盖图片和封面；按 `MerchantID` 允许同商户 STAFF 并能解析软删除上传账号，拒绝跨商户和 admin 图片；覆盖 OWNER 上传、STAFF 删除及跨商品封面引用 |
| 静态访问测试 | 通过 `GET /uploads/...` 断言 `detail-v1` 的 Cache-Control；其他业务文件策略不变；旧 URL 在清理期限前仍可访问 |
| CLI 与 dry-run | 三种模式组合校验先于外部访问；dry-run 必须调用真实/fake processor 并输出预计结果，同时断言对象、业务表、run 表和 item 表均零写入；可注入 Writer 能把 JSON Lines 捕获到 buffer |
| 回填恢复测试 | canary/批次复用 run-id、跨 run 已提交项排除、`source_key == target_key` 删除防御、目标键冲突时既有字节不变、遗留 `PROCESSING` 恢复、事务回滚、各类 `STAGED` 对账、条件更新 0 行不删目标、显式 `--retry-failed` 与 attempts 递增 |
| 全局锁测试 | MySQL 两个不同 run 只能有一个 apply；SQLite apply/cleanup fail closed；锁连接丢失会取消处理且不再写对象或数据库 |
| 延迟清理测试 | 提交后 24 小时内不删除、到期安全校验、目标缺失时保留源、文件记录删除且引用为 0 时同时收敛源/目标、删除成功但状态更新失败后的幂等对账、清理失败可重试，以及待清理/完成/失败汇总 |
| 迁移测试 | SQLite AutoMigrate 与 MySQL 显式迁移分别运行同一组必需列、状态长度和 `(run_id,file_id)` 唯一行为断言；MySQL 集成环境执行真实 up migration，账本不随文件级联删除；不做跨数据库 DDL 文本比较，不要求 down migration |
| 小程序测试 | 列表卡片启用 `lazyLoad`；详情受控 current 的初始和切换状态只为当前及相邻项提供真实 `src`，不以源码正则代替行为断言 |
| 发布门禁测试 | 基础/严格谓词共享实现；阻断异常非零不能切换开关；冻结探针、全实例配置一致性和失败后保持 false 的恢复路径可演练 |
| 端到端验收 | 新上传和回填后商品封面、列表、收藏、浏览历史和详情均展示同一 JPEG；每个成功商品图片都不超过 500KB |

Dockerfile 的快速合约测试继续保留，用于检查必要运维二进制和 migrations 是否被复制，但不绑定无关空白或完整 `COPY` 行文本；镜像构建与容器内 smoke 负责验证真实产物和 codec 能力。

## 明确验收标准

1. 任意成功上传的 `PRODUCT_IMAGE` 都是最长边不超过 1280px 的 JPEG，且文件大小不超过 500KB。
2. 对可压至 300KB 的源图，系统按固定候选顺序保存满足该目标的最高优先级候选；真实样本需做可读性目视验收，但不改变本期 300KB 产品目标。
3. 无法满足 500KB 的图片上传失败，绝不保存或返回原始高清图。
4. 每个商品图片只存一份 `detail-v1` JPEG；不会产生封面或缩略图副本。
5. 有效历史商品图片可以真实 dry-run、单张 canary、分段执行和中断续跑，且不会改变关联的 `file_id`。
6. 已提交项不会因新 run 重复压缩、覆盖或删除当前对象；no-replace 冲突保持既有字节不变，异常 `PROCESSING/STAGED` 可通过对账恢复，数据库切换和账本提交保持原子性。
7. 回填切换后旧对象至少保留 24 小时，只有通过删除防御才会清理；商品已删除时账本仍能收敛遗留源、目标对象，失败对象及原因始终可追踪。
8. 历史引用清零前商户仍可保存通过基础谓词的旧商品；只有完整严格谓词和阻断异常均清零后才能启用严格开关，此后只能关联本商户的 `detail-v1`、`PASS` 图片。
9. 小程序列表不主动加载不可见商品图，详情轮播只为当前及相邻图片提供真实 URL，版本化图片 URL 可长期缓存。
10. apply/cleanup 仅在 MySQL 上运行，并由跨 run 全局锁串行化；锁连接丢失后命令停止一切后续变更。
