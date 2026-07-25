# 发布前问题清单（release-readiness）

更新时间：2026-07-26

## 1. 当前版本状态

- 管理员安全加固已在本地实现：移除固定口令 seed、增加安全 bootstrap、生产默认值保护、自助改密及管理员 session 即时吊销。
- 商家后台多库存已在本地实现：订单数量、单件成交价、派生总价、库存预占、多笔 active 订单及新完成/关闭语义。
- 小程序没有增加下单入口；买家只读 API 的兼容字段 `stock` 返回可售库存。
- 本地实现尚未部署到生产，`0004_merchant_multi_stock` 尚未在生产数据库执行，管理员生产口令也尚未轮换。
- `yaner`、`admin`、`superadmin` 必须保留；本地实现和测试不修改任何现网账号或数据。
- 商家/管理端退出登录已调用服务端 `POST /auth/logout`（F-08，2026-07-26）；本分支已提交，随下次 frontend 发布生效。商家 access token 即时吊销仍属 F-14，不在本发布范围。

## 2. 本地验证结果

- 后端：`go test ./...` 通过。
- 前端：Node `22.22.2` 下 `npm run build` 通过，`npm test`（Vitest）为 6 files / 8 tests 全量通过（含 Layout logout、登录/安全页等；Pro Components 在测试中通过 stub 隔离）。历史记录：2026-07-24 前后曾因 Ant Design 模块初始化挂起未取得绿证，已在后续用 test-only stub 修复，不再作为当前阻断或开放项。
- 小程序：Node `22.22.2` 下 Vitest 为 11 files / 17 tests 全量通过。
- 三条 smoke 脚本通过 `node --check`，已移除固定管理员/测试商家口令；管理员凭据改为环境注入，临时商家密码每次随机生成。
- 主 smoke 已同步新库存断言：数量与总价、剩余库存不售罄、库存归零才 `SOLD`、关单释放预占且不改变上下架状态。

生产数据克隆隔离验收已通过：MySQL 8.4.8 迁移、索引、CHECK、AutoMigrate 兼容性、并发、管理员安全以及桌面/移动浏览器验收均已完成。生产迁移、部署、管理员轮换和真实生产写验证仍未执行。三条 smoke 会创建测试业务数据，本轮没有在生产执行。

`0004` 正式门禁文件（preflight / up / postflight）已入库，维护窗顺序见下文；隔离验收时的 acceptance SQL 与正式门禁同构，生产必须跑 `backend/migrations/` 下三份文件，而不是只发镜像 + AutoMigrate。

Production migration, deployment, administrator rotation, and real production write verification remain undone.

## 3. 生产发布前置条件

生产维护窗只按以下顺序进行；任何一步不满足即停止，不跳过或重排：

```text
recoverable backup evidence
-> protected yaner fingerprint
-> 0004 preflight
-> 0004 up migration exactly once
-> 0004 postflight
-> deploy API and admin frontend together
-> health/auth/read checks
-> controlled dedicated test product create/close/complete
-> protected yaner fingerprint comparison
-> 30-60 minute observation
```

三份唯一的 `0004` 门禁文件为：

- `backend/migrations/0004_merchant_multi_stock.preflight.sql`
- `backend/migrations/0004_merchant_multi_stock.up.sql`
- `backend/migrations/0004_merchant_multi_stock.postflight.sql`

迁移前的停止条件：active 订单非零；`LOCKED` 商品非零；旧索引形态不是预期的唯一 `(product_id,is_active)`；缺少可恢复备份证据；或 `yaner` 缺失、重复、或无法采集发布前指纹。`LOCKED > 0` 时，先报告受影响行 ID 及其 active-order 计数，取得明确业务批准后才可逐行处置；不得批量改写所有受影响商品状态。

生产不得运行 `smoke-mysql-concurrency.mjs`。生产写验证只使用小数量的专用测试商家/商品，依次执行创建 -> 关闭及创建 -> 完成；不得使用 `yaner` 数据，也不得仅为测试轮换现有管理员密码。

详细预检、迁移、发布和回滚步骤以 [生产加固与商家多库存修复方案](./production-hardening-repair-plan-2026-07-24.md) 为准。

## 4. 当前阻断项

- 生产数据库备份与迁移证据未完成。
- 新后端与商家前端尚未部署，生产管理员尚未逐个改密。
- 真实生产写验证、`yaner` 发布前后指纹比较和 30-60 分钟观察尚未执行。

这些事项阻断多库存版本正式上线。F-12、F-13、license governance、miniapp ordering、MySQL root rotation 均在本次发布范围外，不得借此扩大生产窗口。

frontend Vitest 与 F-08 logout 已在代码侧关闭，**不再**列为发布阻断；生产 frontend bundle 仍须随维护窗一并部署后，logout 服务端吊销才对线上用户生效。

## 5. 回滚边界

- 新库存语义开放前可回滚应用并保留新增列。
- 一旦产生多笔 active 或多笔 inactive 历史订单，不得恢复旧唯一索引或直接回滚旧订单代码，应关闭创建入口并向前修复；生产回滚只允许前向修复。
- 管理员口令轮换后不得恢复仓库历史公开口令。
- 任何回滚都不得删除、禁用、改名或重置 `yaner`，也不得把其多库存商品机械改为 1。
