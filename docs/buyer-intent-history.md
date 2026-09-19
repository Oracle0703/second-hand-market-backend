# 购买意向历史修复（Issue #23）

旧唯一索引 `(buyer_id, product_id, is_open)` 同时限制了未关闭和已关闭记录。
同一买家对同一商品第二次发起意向后，关闭操作会与第一条历史冲突并返回 500。

本次修复允许保留多条关闭历史，同时由数据库保证最多一条未关闭意向。
创建操作在事务中锁定商品并重读状态，竞争提交返回业务冲突；联系与关闭操作
锁定同一意向，防止并发联系覆盖关闭状态。重复关闭不会改写关闭时间、备注或重复记审计。

## 实现与兼容

- MySQL 8.0/8.4：增加数据库生成的可空 `open_marker`，未关闭为 `1`，关闭为 `NULL`；
  唯一索引为 `uk_buyer_intent_open(buyer_id, product_id, open_marker)`。
- SQLite：使用 `WHERE is_open = 1` 的 `(buyer_id, product_id)` 部分唯一索引。
- GORM 不再声明旧唯一索引，生成列只读且不由 AutoMigrate 管理。
  开发/测试的 `MigrateSchema` 会建立对应约束；API 正常启动仍不执行迁移。
- 参考旧本地 F-11 分支的索引检查和迁移方案，适配当前 `main` 的事务、商家范围和显式维护命令。
  当前主分支已有 `0009_category_scope_name_index`、`0010_merchant_initial_password`，
  因此本次使用 **0011**。Issue #23 中的旧 `master`、0004–0009 依赖说明不代表当前迁移链。
- 接口请求、响应及小程序入口保持兼容。此变更只针对本仓库，aircon 需另外评估、同步。

## 一次性迁移与发布

代码合并不等于数据库已迁移。现有 CD 只发布 API/Web，不执行数据库脚本。
本分支没有执行生产或 staging 迁移；生产发布前应在对应环境安排一次维护窗口。

1. 核对目标为主项目：生产 Docker 项目 `secondhand-market-cd-production`，
   数据库容器 `secondhand-market-mysql`。不要使用 aircon 的项目、数据库、卷或凭据。
2. 使用已确认的目标数据库与专用迁移凭据，将 `DB_DRIVER`、`DB_DSN` 放入进程环境，
   不写入仓库、终端命令参数或日志。暂停该环境的意向写入和其他维护程序。
3. 在 `backend/` 目录执行 MySQL 迁移：

   ```sh
   go run ./scripts/migrate --migration 0011_buyer_intent_open_uniqueness
   go run ./scripts/buyer_intent_schema --check
   ```

   命令按 SHA256 校验并依次执行 preflight、up、postflight。
   SQL 使用临时存储过程，因此迁移账号需要 CREATE/ALTER ROUTINE、EXECUTE、ALTER/INDEX 和 SELECT 权限。
   preflight/postflight 不改变业务表或记录，但会创建、调用、删除检查存储过程。
   遇到未知列/索引、重复未关闭记录、状态不一致时停止，不自动删除或修正业务数据。
4. SQLite staging 使用其自己的 `DB_DSN`，停止写入后执行：

   ```sh
   go run ./scripts/buyer_intent_schema --apply-sqlite
   go run ./scripts/buyer_intent_schema --check
   ```

   该命令只修改已有 `buyer_intents` 表的索引，DDL 放在一个事务中；不会初始化其他表或账号。
5. 检查成功后按既有 CD 发布前后端，恢复写入并验证三轮创建/关闭、历史列表和重复关闭。
   `/readyz` 仅验证数据库连通性，不能替代上述 schema 检查。

MySQL 先添加生成列和新索引，验证后才移除旧索引，整个过程中保留唯一约束。
每条 DDL 独立提交；中断在生成列或新索引阶段时，可检查后重跑同一迁移。
已完成状态可重跑，已有记录不变。失败时命令返回非零，必须检查失败原因再继续发布。

不提供恢复旧唯一索引的 down：多条关闭历史产生后，旧约束已无法表达数据。
回退 API/Web 镜像时保留新索引；旧 API 的显式字段写入与新生成列兼容，
不得运行旧版本 AutoMigrate 以免重建错误索引。

## 验证

- SQLite：旧索引迁移、历史保留、拒绝第二条未关闭记录、结构漂移拒绝、重复执行、维护命令。
- 实际 HTTP：三轮创建/关闭，重复关闭审计次数，关闭后禁止再次标记联系。
- CI MySQL 8.0 与 8.4：执行真实 0002 表定义和 0011 三阶段脚本，覆盖中断续跑、
  异常数据/索引拒绝、AutoMigrate 兼容、并发创建单赢家、并发关闭及联系/关闭竞争。
- MySQL 测试只接受 loopback 的 `buyer_intent_ci` 临时库，凭据来自 CI 合成配置。
  未设置 `BUYER_INTENT_MYSQL_TEST=1` 时，本地测试会跳过该套件。

```sh
cd backend
go test ./...
go vet ./...
go test -race ./internal/app ./tests -run 'BuyerIntent|MigrateBuyerIntent|Verify.*BuyerIntent' -count=1
```
