# 设计文档审核：商品售罄与可恢复销售状态

| 项目 | 内容 |
|---|---|
| 日期 | 2026-08-09 |
| 审核对象 | `docs/superpowers/specs/2026-08-09-product-sold-out-state-design.md` |
| 分支 | `hotfix/hy/0000_product_sale_status` |
| 结论 | 有条件批准；主方案正确，实现前需补齐语义缺口 |

## 1. 总体评价

| 维度 | 评价 |
|---|---|
| 问题定义 | 清晰：去掉商品关闭、SOLD=售罄、可补库存恢复 |
| 方案选择 | A 合理（保留 `SOLD` 码，避免迁移与契约大改） |
| 不变量 | I1–I6 基本扎实 |
| 与代码现状契合度 | 高；多数改动点文件定位准确 |
| 可实施性 | 中高；有几处实现语义未写死，容易实现分叉 |

当前代码对照要点：

- 订单完成/关闭已按库存预占联动（本分支未提交改动）
- 库存调整里 `MARK_SOLD`→0 已会进 `SOLD`；`SOLD` **尚不能**补库存
- `checkProductReadyForOnShelf` **只校验图片**，还没有库存/活动订单校验
- 小程序详情已能看到 `SOLD`（`buyerDetailVisibleStatuses` 含 `SOLD`），但展示的是原始 `product.status`
- 商家文案仍是 `SOLD=已成交`、`CLOSED=已关闭`

## 2. 必须澄清（实现前建议定稿）

### 2.1 状态机归属：`stateflow` vs 领域 handler

文档 §5 画了完整业务边（含 `LOCKED→ON_SHELF/OFF_SHELF`、`SOLD→OFF_SHELF`、`ON_SHELF/OFF_SHELF→SOLD`），但现状是：

| 路径 | 是否走 `CanTransitionProduct` |
|---|---|
| 上架/下架/关闭商品 API | 是 |
| 创建/完成/关闭订单 | 否（直接改商品状态） |
| 库存调整 `MARK_SOLD` / 补库存 | 否 |

本分支测试还明确写了：`LOCKED → OFF_SHELF` 在 stateflow 中应被拒绝（订单关闭绕过）。

文档 §12 只写「增加售罄补库存后的恢复语义」，没有给出**目标 `productTransitions` 全表**，也没说明哪些边只允许订单/库存路径触发。

**建议在文档中增加一小节**，例如：

```text
productTransitions（商品状态 API 允许的边）:
  DRAFT     -> ON_SHELF
  ON_SHELF  -> OFF_SHELF, LOCKED
  OFF_SHELF -> ON_SHELF
  LOCKED    -> （无；禁止商品 API 直接改 LOCKED）
  SOLD      -> （无；禁止商品 API 直接上架）

领域路径允许但不必进入 productTransitions 的边:
  订单完成: LOCKED -> ON_SHELF | SOLD
  订单关闭: LOCKED -> OFF_SHELF
  MARK_SOLD 归零: ON_SHELF|OFF_SHELF -> SOLD
  INCREASE 补货: SOLD -> OFF_SHELF
```

否则实现时很容易出现「补了 stateflow 边导致 LOCKED 可被商品 API 下架」或「只改 handler 不改测试/状态机文档」的分叉。

### 2.2 `DRAFT` 上的 `MARK_SOLD` 语义含糊

| 位置 | 说法 |
|---|---|
| §8.1 | `DRAFT`：`MARK_SOLD` =「不允许设为售罄」 |
| §6.4 | 「通用库存调整中的 `MARK_SOLD` 部分扣减能力继续保留」 |
| 现状代码 | `DRAFT` 可 `MARK_SOLD`，扣到 0 会进 `SOLD` |

需要二选一写死：

1. **DRAFT 完全禁止 `MARK_SOLD`**（只能 `INCREASE/DECREASE`；售出在上架后做）
2. **DRAFT 允许部分/全部 `MARK_SOLD`**，扣到 0 是否允许进 `SOLD`

「不允许设为售罄」容易被理解成「禁止整单快捷操作，但允许部分扣减」——实现会不一致。

### 2.3 「设为售罄」用页面缓存库存做 `quantity`，并发不安全

§6.4：快捷操作提交「页面最新的全部剩余库存」，前端以 `status_after = SOLD` 判成功。

并发场景：

- 页面显示 stock=5，同时另一人 `DECREASE` 2 → 提交 `MARK_SOLD 5` → 超库存失败（还好）
- 页面显示 stock=5，实际已是 3，前端仍可能被做成「部分售出」而非售罄（若错误地允许 quantity 截断）

更稳的后端语义建议写进设计：

```text
设为售罄（快捷）:
  在行锁内将 stock 直接归零并转 SOLD
  不信任客户端 quantity；quantity 仅用于流水展示或改为可选
```

或至少规定：`MARK_SOLD` 且 `quantity != 当前 stock` 时，快捷入口必须失败并提示刷新，而不是部分扣减。

### 2.4 历史 `CLOSED` → `OFF_SHELF` 会改变买家可见性

现状：

```go
buyerDetailVisibleStatuses = ON_SHELF | LOCKED | OFF_SHELF | SOLD
// 不含 CLOSED → 买家详情 404
```

迁移后原 `CLOSED` 商品变成 `OFF_SHELF`，历史链接会重新可打开（显示下架）。这不一定是坏事，但是**行为变化**，文档 §11 未写。

**建议补充：**

- 迁移后买家是否应可见这些商品
- 若不应可见，是继续保持不可见，还是迁移时打标/软删

### 2.5 `SOLD` 不可删除，退出路径偏绕

§5.1 / §9.2：`SOLD` 只能查看、补库存。
商家若只想废弃售罄商品，必须：`补库存 → 下架 → 删除`，或先补再盘亏。

请明确这是产品决策还是遗漏：

- 允许 `SOLD` 直接删除（库存已为 0）
- 或允许 `SOLD → OFF_SHELF` 的「放弃恢复」无库存动作
- 或文档写明「必须先补库存再删除」为有意约束

## 3. 中优先级（建议补进文档）

### 3.1 上架校验落点写清楚

§8.2 要求：状态允许 + 图片 + `stock - reserved > 0` + 无活动订单。
当前只校验图片。文档影响范围提到 `product_handlers.go`，建议直接写：

> 在 `doProductStatusChange` 当 `toStatus == ON_SHELF` 时，扩展 `checkProductReadyForOnShelf`（或等价逻辑）校验 stock/reserved/active_order。

并区分错误码：图片缺失现在是 `10001`，库存为 0 设计为 `10005`——可以，但实现时别混用。

### 3.2 前端库存调整入口矩阵需同步改

现状 `canAdjustProductStock`：仅 `DRAFT/ON_SHELF/OFF_SHELF`。
设计要求 `SOLD` 可 `INCREASE`。

§9.2 已写「只展示补充库存」，但测试清单可再加：

- `canAdjustProductStock('SOLD') === true`
- 类型选项仅 `INCREASE`
- 文案：`SOLD` 从「已成交」改为「售罄」；通用 `MARK_SOLD` 是继续叫「线下售出」还是「售出扣减」

「设为售罄」快捷按钮 vs 弹窗里的 `MARK_SOLD` 选项关系也建议画一下，避免两个入口文案打架。

### 3.3 Dashboard API 契约变更

`product_stats` 去掉 `closed` 后，前端 `DashboardPage` 仍有：

```ts
const PRODUCT_STATUS_ORDER = [..., 'sold', 'closed']
```

文档有提，建议在 §8 或 §12 明确：

- 响应字段删除 `closed`（或固定返回 0 做兼容一期）
- 前端同步删 product closed 统计项

订单 `closed` 统计保留——这点写得好。

### 3.4 操作日志历史兼容

移除 `product_close` 路由后，历史 `operation_logs` 仍有 `product_close`。
商家日志页映射应**保留展示文案**，只是不再产生新记录。建议在 §7 加一行，避免被当成「死代码」删掉。

### 3.5 发布顺序应把数据修复设为硬前置

§15 步骤 1 是「统计并处理」，但后端去掉 `CLOSED` 边后，未迁移的 `CLOSED` 商品会变成**无法上架/无法调整的僵尸数据**（取决于 `canAdjust` 是否仍认 CLOSED）。

建议改为：

1. **阻塞项**：迁移 `CLOSED→OFF_SHELF` + 修复 `SOLD stock>0`
2. 再发后端

并给出迁移 SQL 的校验查询（count 应为 0 才继续）。

### 3.6 测试缺口

已有 B1–B9 不错，建议至少补：

| 编号 | 用例 |
|---|---|
| B10 | `ON_SHELF` 经 `DECREASE` 到 0 → `OFF_SHELF`，不是 `SOLD` |
| B11 | 部分 `MARK_SOLD`（未扣到 0）状态不变 |
| B12 | `SOLD` 禁止 `DECREASE`/`MARK_SOLD` |
| B13 | `POST .../products/:id/close` 404 或移除 |
| B14 | 买家详情 `SOLD` 可见且 `can_submit_intent=false` |
| B15 | 迁移后不存在 `status=CLOSED` 商品 |
| F6 | Dashboard 无商品 closed |
| F7 | `SOLD` 文案为「售罄」且库存入口仅补货 |

`stateflow_test` 里「SOLD terminal」在引入 `SOLD→OFF_SHELF`（若走 stateflow）后必须改写，否则会误导。

## 4. 低优先级 / 文档质量

1. **§5.1 `OFF_SHELF`「库存大于 0 时设为售罄」** 与 §8.1 全量允许 `MARK_SOLD` 略不一致——用「stock=0 时快捷入口隐藏/禁用」即可对齐。
2. **I3** 可加强：`active_order_id` 指向 `CREATED` 订单，且 `reserved_stock` 与该订单 `quantity` 一致（单活动订单模型下）。
3. **小程序** 详情仍渲染原始 `product.status`（`SOLD` 英文码）；§10 要求显示「售罄」，应点名要加状态文案映射，并测详情页不是列表页。
4. **常量名 `ProductSold`/`SOLD`** 保留的技术债可在 §3 方案 A 的缺点里加一句「注释/文档统一称售罄」，减少后人再当成终态成交。
5. **与旧设计文档关系**：`2026-08-03-inventory-adjustment-design.md` 写明 `SOLD/CLOSED` 不可调库存；本文是对其的修订，建议在文首加「修订关系」避免两份 specs 冲突。
6. **自审 §16** 写「无歧义」，但 DRAFT/`MARK_SOLD`、stateflow 归属、SOLD 删除路径仍有歧义——建议修完后再勾。

## 5. 与现状对齐良好的部分（可保留）

- 保留 API 状态码 `SOLD`、展示改「售罄」——兼容成本最低
- 补库存 → `OFF_SHELF`，不自动上架——正确
- `DECREASE` 归零 ≠ 售罄——财务/业务语义清楚
- 订单 `CLOSED` 与商品售罄分离——和现有订单/意向模型一致
- 关闭订单不得改 `SOLD`、不得清零库存——与本分支 `order_handlers` 一致
- 买家列表仍只 `ON_SHELF`，`can_submit_intent` 继续后端计算——正确
- 不删 `closed_at` 列——范围控制得好
- 不引入 `SOLD_OUT`、不多活动订单——非目标克制

## 6. 建议的定稿 checklist

在标「用户已批准 / 可写 implementation plan」前，建议文档补上：

1. **stateflow 全表 + 领域旁路边**（§5 或新 §5.2）
2. **DRAFT + MARK_SOLD** 最终规则
3. **设为售罄** 后端是否「锁内直接归零」
4. **SOLD 删除/废弃** 路径
5. **CLOSED 迁移对买家可见性** 的产品结论
6. **上架校验** 的具体函数与错误码
7. **修订** 对 2026-08-03 库存调整设计的关系说明
8. **测试** B10–B15 / F6–F7

## 7. 审核结论

| 项 | 结果 |
|---|---|
| 是否批准直接开发 | **有条件批准** |
| 阻塞项 | 上文「必须澄清」2.1–2.5 |
| 主方案 A | **同意** |
| 风险等级 | 中（状态机分叉 + 数据迁移时序 + 前端契约） |

主业务叙事（售罄可恢复、先补货再上架、去掉商品关闭）是对的，也贴合现有库存/订单模型。把状态机归属、DRAFT/售罄快捷语义、迁移副作用写死后，就可以进入实现计划拆任务。
