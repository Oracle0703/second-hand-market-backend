# 审核意见：商品图片详情级压缩与回填

| 项 | 内容 |
| --- | --- |
| 审核对象 | `docs/superpowers/plans/2026-07-30-image-delivery-optimization.md` |
| 对照规格 | `docs/superpowers/specs/2026-07-30-image-delivery-optimization-design.md` |
| 对照实现 | `backend/internal/media/*`、`backend/internal/app/file_handlers.go`、`product_handlers.go`、商家前端编辑页、小程序图片组件 |
| 审核日期 | 2026-07-30 |
| 结论 | **主线方向正确，但不可按现稿直接开工**；存在会直接导致上传失败与上线窗口锁死商家改商品的问题 |

---

## 总体评价

方案主干成立：

- 仅处理 `PRODUCT_IMAGE`，执照等业务文件保持原规则
- 服务端统一输出 JPEG，不落原图，超 500KB 失败
- 版本化对象键 + 仅对 `detail-v1/` 长期 `immutable` 缓存
- 回填不改 `file_id`，商品关联 / 收藏等无需迁移
- 条件更新 `WHERE id=? AND object_key=? AND scan_status=PASS`
- 账本 + dry-run / canary / 分批 + 回填二进制打入同镜像

但实现计划与现有上传契约、发布顺序之间有 **P0 级逻辑漏洞**，另有若干不合理设计与偏弱/无效设计。建议先修订计划与规格，再进入编码。

---

## 一、严重逻辑漏洞（P0）

### 1. 预登记 MIME=`image/jpeg` 与源图 claim 校验冲突（必现 bug）

**文档要求：**

- Presign 时 `PRODUCT_IMAGE` 预登记 `MimeType: "image/jpeg"`
- Upload 时把 `file.MimeType` 作为 `InputMIME` 传给 processor

**现有处理器行为：**

`VipsCLIProcessor` / `PassthroughProcessor` 均校验：

```text
ImageMIMEMatchesClaim(detected, req.InputMIME)
```

**失败路径（商家上传 PNG / WebP / HEIC）：**

| 步骤 | 实际值 |
| --- | --- |
| 客户端声明 | `image/heic` 等（presign 允许类型校验通过） |
| DB 预登记 | `image/jpeg`（按文档） |
| 处理时 InputMIME | `image/jpeg` |
| 字节检测 | `image/heic` |
| 结果 | claim 不匹配 → **上传失败** |

设计规格写的是「源 MIME 只用于前置允许类型校验」，但实现计划 **没有** 给出任一完整修法：

1. 单独保存 `source_mime`（或等价字段），处理时用源 MIME 做 claim；或
2. 在 `detail-v1` 路径明确 **跳过 InputMIME claim，只信字节检测**；或
3. multipart 上传时重新传入客户端源 MIME，且 handler 用该值而非预登记输出 MIME

**影响：** 按现稿实现后，非 JPEG 商品图基本传不上去。

**建议写入计划的明确契约：**

- Presign：仍可预登记最终 `mime_type=image/jpeg` 与 `.jpg` 键
- Upload：`InputMIME` / 源格式校验 **不得** 依赖预登记的输出 MIME
- `detail-v1`：以上传字节检测为准；输出强制 JPEG 并在 handler 侧做硬校验

---

### 2. 发布顺序会让「未回填商品」无法编辑

**冲突点：**

- Task 2 上线后，创建/编辑商品 **只接受** `detail-v1` + `PASS` 图片
- 商家端 `EditPage` **每次保存都会提交** 现有 `image_file_ids`（含历史非 detail-v1 图）
- 发布清单却是：先发带严格校验的 API → **之后** 维护窗口再回填

**中间窗口后果：**

- 买家仍可能看到旧大图（旧 URL 仍可访问，直到回填删除）
- 商家改标题 / 库存 / 描述也可能失败（因为仍提交旧 `file_id`）
- 只有「删光旧图 → 重新上传」才能保存

**可选修法（计划必须选一种并写进发布步骤）：**

1. **两阶段发布**：先启用新上传压缩 + 完成回填，再打开 `detail-v1` 关联校验
2. **过渡期 grandfather**：仍被商品引用的旧 `PASS` 非 `detail-v1` 暂时允许关联
3. **特性开关**：`REQUIRE_DETAIL_V1_PRODUCT_IMAGES=false` 直至回填清零后再打开

**结论：** 关联校验与回填时序在现稿中不成立，属于上线逻辑漏洞。

---

## 二、不合理设计（P1 / P2）

### 3. 列表/封面与详情共用一张 1280px / ~300KB 图（P2）

规格明确「不生成缩略图」，实现计划只做 `detail-v1`。

- 从平均约 4.39MB → 约 300KB：收益明显，作 MVP 可接受
- 列表一屏 10 张仍约 3MB；`lazyLoad` 只减并发，**不减单卡体积**

若长期目标是列表体验，更合理的是 `list-v1`（如最长边 400–640）+ `detail-v1`，或同对象多尺寸。
建议在文档中标明：**当前为 MVP 折中，非长期最优**，并可选记录后续演进。

---

### 4. 「300KB 优先」可能过度牺牲清晰度（P2）

选择规则：

1. 先在 ≤300KB 里选「边最长 + 质量最高」
2. 否则在 ≤500KB 里同样选

**反例：**

| 候选 | 结果 |
| --- | --- |
| 1280 / Q82 / 450KB | 落选 |
| 960 / Q66 / 290KB | **入选** |

二手商品图细节往往重要，450KB 高清可能优于 290KB 小图。
若产品更看重观感，可考虑：

- 硬上限内最大化边长/质量，300KB 仅作 soft target；或
- 对「目标内更小边」设最小可接受边长（如不低于 1120）

需产品确认，不是实现 bug，但是明确的质量取舍。

---

### 5. 同步上传路径最多 15 次 vips，未写 early-exit / 超时（P1）

- Edges: `1280, 1120, 960`
- Qualities: `82, 78, 74, 70, 66`
- 最坏 15 次 vips；原图可达 40MB HEIC，HTTP 内串行压力大

若按「边降序、质量降序」生成，**第一个 ≤300KB 的候选即为最优**，与 `Select` 一致，应允许 early-exit。
计划还应补充：处理超时、`context` 取消、上传耗时预期（SLO 或至少运维备注）。

---

### 6. 回填提交后立刻删旧对象 → 短暂裂图（P1）

流程：条件更新 `file_records` → 删除旧对象。

小程序/前端若仍缓存旧 `cover_url`（react-query、未刷新页面），旧键 404，直到重新拉商品。
维护窗口能降低概率，但读路径没有失效或双读策略。

**更稳妥选项：**

1. 延迟删除（如 24h TTL + 二次清理命令）
2. 旧键短时 302 / 回源到新对象
3. 明确接受「维护窗内可能裂图」并写入验收，而不是只写「旧对象被删除」

---

### 7. 维护窗口只靠文档，代码不强制（P1）

设计已承认：商品删除会在事务外删物理文件，不能与回填并发。
计划仅要求 `--workers=1` 与运维纪律，**没有**：

- 全局 maintenance flag
- 回填期间拒绝 upload / product image edit / delete

运维漏做时仍有丢图或条件更新失败风险。对「可安全恢复」的宣称偏乐观。
至少应在发布清单用粗体强调，或增加简单的维护开关。

---

### 8. 账本唯一键是 `(run_id, file_id)`，不是 `file_id` 全局唯一（P1）

两次 `--apply` 使用不同 `run-id` 时可并行处理同一文件，条件更新可能互相覆盖。
生产靠「人工只跑一个 run」约束，模型层未防呆。

**建议：**

- `COMMITTED` 维度对 `file_id` 唯一；或
- 候选查询跳过已是 `detail-v1` 的 `object_key`；或
- 命令级全局锁 / 单 flight 文件

---

## 三、无效或偏弱设计（P2 / P3）

### 9. Task 6 小程序 `lazyLoad` 与主目标耦合弱（P2）

主目标是体积与可恢复回填；`lazyLoad` 是请求时序优化：

- 微信 / 抖音对 `lazyLoad` 行为不一致
- 测试是读 TSX 源码正则，不验证运行时
- 对「数 MB → 300KB」的收益是次要的

建议降为可选 polish，或从本计划拆出，避免占满 Task 与验收比重。

---

### 10. 「confirm 不能把 PENDING 提成 PASS」基本是现状复述（P3）

当前 `handleConfirmUpload` 已要求 `scan_status == PASS`，confirm 不能抬升 PENDING。
写进验收可以；算「本需求新能力」则无效。

---

### 11. `PassthroughProcessor` 承担 `detail-v1` 语义含糊（P2）

「非生产测试下为详情 profile 重新编码小尺寸 JPEG」未说明：

- 是否真 resize 到 1280
- 是否执行与 vips 相同的 Select 阶梯
- 与生产 vips 结果是否允许不一致

测试若靠 fake processor，不必重造一套 passthrough detail；若要真测，应明确最小行为。

---

### 12. 打包测试用字符串匹配 Dockerfile（P3）

`TestImageDeliveryPackaging` 靠子串断言 Dockerfile，重构 `COPY` 行即挂，对正确性帮助有限。
镜像构建 smoke（Task 5 Step 4）更有价值；源码字符串测试可弱化。

---

### 13. dry-run 行为：设计 vs 计划不一致（P1）

| 来源 | dry-run 行为 |
| --- | --- |
| 设计规格 | 扫描、解码、**估算输出体积**、出清单 |
| 实现计划测试 | `Processed == 0`，不建账本、不写对象 |

若 dry-run 不做真实压缩估算，运维无法预判「有多少会因 >500KB 失败」。
存量约 11 张时影响有限；更大存量时 dry-run 会变成无效演练。

**应统一为以下之一：**

1. dry-run 真跑 process，只禁止写库/写最终对象/删旧对象；或
2. 明确 dry-run 仅候选枚举，不做体积估算，并改设计规格

---

### 14. `LeaseUntil` / 多 worker 状态机写了字段、没写完整协议（P3）

模型有 `LeaseUntil`、`Attempts`，正文几乎只说「单并发 + 过期租约接管」。
`workers>1` 的抢占、续约、崩溃恢复未定义 → 字段可能变成死设计。

若生产永不为 1：删多 worker 或把租约协议写全。

---

### 15. 对象键表述易误导（P2）

- 新上传：`product_image/detail-v1/F<BuildBizNo>.jpg`
- 回填：`product_image/detail-v1/F<file-id>.jpg`

`BuildBizNo("F")` 已是 `F2026...` 形式，再写 `F<BuildBizNo>` 容易造成 **双 F**。
回填用 `file-id`、上传用业务号其实合理（回填要稳定可重入），但应用明确伪代码写清，避免实现歧义。

**建议写法示例：**

```text
新上传: product_image/detail-v1/{BuildBizNo("F")}.jpg
回填:   product_image/detail-v1/F{file_id}.jpg
```

---

## 四、与现有代码的其它缺口（实现时易漏）

| 点 | 说明 |
| --- | --- |
| 商品创建/更新今天完全不校验图片归属与 PASS | 计划补校验正确，但必须与回填发布窗口一起设计 |
| Admin 也可传 `PRODUCT_IMAGE` | `validate...` 要求 `UploaderType=merchant` 且 `UploaderID=actor`，admin 代运营可能被误杀 |
| 已是 `detail-v1` 的文件 | 候选查询未要求 skip，回填可能重复压缩 |
| `FAILED` 是否可同 run 重试 | 只详写了 `CLEANUP_PENDING` 续跑，失败重试语义不清 |
| 仅有 `0004.up.sql` | 与仓库 `0001–0003` 带 down 的习惯不一致（若本来不用 down 应注明） |
| 前端预览仍用 `createObjectURL(原文件)` | 商家预览仍是原图，与落盘 JPEG 不一致；非 bug，但预期未写 |

---

## 五、建议保留的部分

以下设计合理，修订计划时应保留：

1. 只动 `PRODUCT_IMAGE`，执照等保持原格式
2. 不落原图；处理失败 / 超限不得标 `PASS`
3. 版本化前缀 + 长期缓存仅适用于该前缀
4. 回填不改 `file_id`
5. 条件更新同一条 `file_records`
6. 账本 + dry-run / canary / 分批 / 可中断续跑
7. 生产禁 passthrough、显式迁移、回填进同一镜像
8. 共享 `LocalObjectPath` / `AtomicWriteFile` 给 HTTP 与回填
9. 生产 API 不在启动时自动回填

---

## 六、修改优先级总表

| 优先级 | 项 | 建议动作 |
| --- | --- | --- |
| **P0** | MIME / claim 冲突 | 改 presign / upload / processor 契约，保证 HEIC/PNG/WebP 可转 JPEG |
| **P0** | 严格校验 vs 回填顺序 | 特性开关或 grandfather，消灭「不能改商品」窗口 |
| **P1** | dry-run 是否真估算 | 与设计对齐，否则 canary 前缺少失败预判 |
| **P1** | 删旧对象策略 | 延迟删，或接受裂图并写入验收 |
| **P1** | 候选 early-exit / 超时 | 避免上传被最多 15 次 vips 拖死 |
| **P1** | 维护窗口强制力 | 文档加粗或加维护开关；多 run 防重 |
| **P2** | 300KB 选择策略 | 产品确认是否接受「为流量牺牲清晰度」 |
| **P2** | 单图服务列表 | 标注 MVP，或规划 list 尺寸 |
| **P2** | lazyLoad 任务 | 降级或拆出 |
| **P2** | 对象键伪代码 | 消除双 F 歧义 |
| **P3** | 租约 / 多 worker / admin 上传 | 补协议或删死字段 |
| **P3** | confirm 条款 / Dockerfile 字符串测试 | 降为既有行为说明或弱化测试 |

---

## 七、一句话结论

文档作为「压缩 + 可恢复回填」主线成立，但 **不能按现稿直接实现**：

1. 先修 **源 MIME 与 detail 输出 MIME 的管道**
2. 再修 **关联校验与回填的发布耦合**

否则会出现两类硬伤：

- 新格式图（非 JPEG 源）上传失败
- 上线后商家无法保存仍引用历史图的商品编辑

其余多为产品取舍与运维强度问题，建议在改计划时一并写死，避免实现期各自发挥。

---

## 八、建议的计划修订清单（供改稿勾选）

- [ ] 明确 `detail-v1` 上传时源格式检测与 InputMIME 契约，覆盖 JPEG/PNG/WebP/HEIC/HEIF
- [ ] 明确关联校验的发布阶段（开关 / grandfather / 回填后再生效）
- [ ] 统一 dry-run：是否真实 process 与体积估算
- [ ] 写明候选生成顺序与 early-exit、超时
- [ ] 写明旧对象删除策略与客户端裂图预期
- [ ] 写明多 `run-id` / 已是 `detail-v1` 的跳过规则
- [ ] 澄清对象键伪代码（上传 bizno vs 回填 file_id）
- [ ] 澄清 admin 上传商品图是否允许
- [ ] 澄清 `FAILED` 重试与 `LeaseUntil` 是否保留完整语义
- [ ] 评估 Task 6 lazyLoad 是否留在本计划
- [ ] 产品确认 300KB 优先 vs 硬上限内保真优先

---

## 九、审核范围说明

本文件为文档/设计逻辑审核，**未**执行实现或跑测试。
对照代码以审核当日仓库状态为准，用于验证计划假设是否与现有契约冲突。
