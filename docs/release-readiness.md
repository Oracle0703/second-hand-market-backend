# 发布前问题清单（release-readiness）

更新时间：2026-07-28

## 1. 当前版本状态

- 管理员安全加固已在本地实现：移除固定口令 seed、增加安全 bootstrap、生产默认值保护、自助改密及管理员 session 即时吊销。
- 商家后台多库存已在本地实现：订单数量、单件成交价、派生总价、库存预占、多笔 active 订单及新完成/关闭语义。
- 小程序没有增加下单入口；买家只读 API 的兼容字段 `stock` 返回可售库存。
- 本地实现尚未部署到生产，`0004_merchant_multi_stock` 与 `0005_file_records_table` 尚未在生产数据库执行，管理员生产口令也尚未轮换。
- `yaner`、`admin`、`superadmin` 必须保留；本地实现和测试不修改任何现网账号或数据。
- 商家/管理端退出登录已调用服务端 `POST /auth/logout`（F-08，2026-07-26）；本分支已提交，随下次 frontend 发布生效。F-14 已在代码侧要求每个已认证请求校验当前 session 和账号状态，隔离测试服务器审核仍待单独授权，生产尚未部署。
- 文件元数据表名以 `file_records` 为唯一契约（F-09）：**本地修复并通过隔离 MySQL 8.4 测试服务器审核；生产未执行 0005**。`FileRecord.TableName()` 已固定；`0005` preflight/up/postflight 已入库（up 重复形态校验）。
- 分类唯一性以 `(parent_id,name)` 为唯一契约（F-16）：**本地修复并通过隔离 MySQL 8.4 测试服务器审核；生产未部署**。GORM 模型和两条 seed 路径已对齐历史 SQL migration。
- F-02 code-side closed on branch, pending frontend/backend deployment and `0006` production migration. 文件归属、类型、扫描状态、URL 与一次性 capability 已在事务内强制校验，并通过独立 MySQL 8.4.8 矩阵；本轮未部署、未执行生产迁移。
- F-06 代码侧已修复：0008 HAVING 兼容性已在 F-11 的隔离 MySQL 8.4.8 完整迁移链中通过，前端 code `10013` 配额错误映射已在 `03309d1` 补齐并独立复审；F-06 专用治理矩阵仍单独跟踪；生产未执行 0008、未部署、未修改生产数据或文件。
- F-11 已在提交 `6f84cc6` 通过隔离 MySQL 8.4.8 测试服务器审核；生产未执行 0009、未部署。F-12 的 F-11 前置条件已满足，但 F-12 尚未实现或验收。
- F-15 原子幂等已在代码侧实现并通过本地 focused/full/race/vet 与无 `.git` 源包门禁。首次隔离 MySQL 8.4 运行在 harness `test_metadata` 阶段因 metadata 容器 UID/GID 边界失败；`f46bb3c` 已按 TDD 修复并通过独立复审，测试服务器仍未批准，生产未部署、未修改线上数据。

### 1.1 首轮问题与后续 schema 修复状态

| 问题 | 修复状态 | 测试服务器审核 | 生产状态 |
| --- | --- | --- | --- |
| P1-4.1 遗留 `LOCKED` | 按批准策略关闭：正式 preflight/postflight fail-closed，非零时停迁并逐行人工批准；不做批量静默改状态 | 生产数据克隆 MySQL 8.4.8 门禁通过 | 维护窗仍须重新预检 |
| P1-4.2 active 订单迁移护栏 | 已关闭：正式 `0004` preflight 要求 active=0，runbook 固定 preflight→up→postflight | 生产数据克隆 MySQL 8.4.8 通过 | `0004` 未在生产执行 |
| P1-4.3 旧唯一索引 / AutoMigrate | 已关闭：显式 SQL 删除旧索引、postflight 校验、禁止只靠 AutoMigrate | MySQL 8.4.8 索引与 AutoMigrate 兼容通过 | 待维护窗执行 |
| P2-4.4 并发与竞争证据 | 已关闭：多库存创建、双完成、双关闭、完成/关闭竞争 smoke | 生产数据克隆 MySQL 8.4.8 通过 | 生产禁止运行破坏性并发 smoke |
| P2-4.5 frontend Vitest | 已关闭：test-only Pro Components stub；全量 test/build 通过 | 隔离服务器桌面/移动关键 UI 流通过 | 新 frontend 尚未部署 |
| F-08 frontend logout | 已修复：调用服务端 logout，失败时仍清理本地会话 | 未单独做服务器 logout 验收；本地 8/8 测试通过 | 新 frontend 尚未部署 |
| F-09 文件表 schema | 已修复：`file_records` 契约 + `0005` 三段门禁 | **MySQL 8.4.8 完整八态矩阵通过** | `0005` 未在生产执行 |
| F-16 分类 schema | 已修复：复合索引 + parent-aware seed | **同一矩阵的 AutoMigrate RED→GREEN 通过** | 新后端尚未部署 |
| F-02 文件绑定授权 | 已修复：商家归属 + PUBLIC 一次性 capability + 商品/执照事务校验 | **MySQL 8.4.8 回填/失败门禁/API/并发/AutoMigrate 矩阵通过** | `0006` 未执行，frontend/backend 未部署 |
| F-06 匿名上传资源治理 / D-03 大小契约 | 代码侧已修复；0008 HAVING 兼容性与前端 `10013` 中文映射均已关闭 | F-11 隔离矩阵已证明 0008 preflight/up/postflight 可在 MySQL 8.4.8 运行；F-06 专用治理矩阵仍单独跟踪 | 生产未执行 0008、未部署、未修改生产数据或文件 |
| F-11 买家意向 open 唯一性 | 代码侧修复并固定接受提交 `6f84cc6` | **MySQL 8.4.8 完整 0008/0009、API、AutoMigrate、full/race/vet 矩阵通过** | 生产未执行 0009、未部署；F-12 前置已满足但尚未实现 |
| F-14 session access 吊销 | 代码侧已修复 | 未审核；专用 Compose 项目尚未获授权运行 | 未部署，未修改生产数据或 session |
| F-15 原子幂等与失败回滚 | 代码侧已修复并本地验证；成功结果与业务写同事务提交，失败不留 claim；`f46bb3c` 修复验收 metadata UID/GID 边界 | **未通过审核**；首次运行 MySQL 8.4 门禁通过，但在 `test_metadata` 失败，完整矩阵未运行；修复后重跑待重新授权 | 无迁移；未部署、未修改生产数据或幂等记录；固定生产容器快照前后一致 |

F-09/F-16 脱敏证据与 SHA-256 见
`docs/superpowers/reviews/2026-07-26-file-category-schema-isolated-acceptance.md`。
F-02 隔离范围、脱敏证据路径与 SHA-256 清单见
`docs/superpowers/reviews/2026-07-26-file-binding-authorization-isolated-acceptance.md`。
F-06 待授权的专用验收命令为：

```bash
ANONYMOUS_UPLOAD_GOVERNANCE_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_UPLOAD_GOVERNANCE_DATA \
ACCEPTANCE_DB_ENGINE=mysql8.4 \
make acceptance-anonymous-upload-governance-smoke
```

固定远程路径为 `/home/yu/services/secondhand-upload-governance-acceptance-20260726`，Compose project 为 `secondhand-upload-governance-acceptance`；此前任何其他问题的传输授权均不适用。

## 2. 本地验证结果

- 后端：`go test ./...` 通过（含 `FileRecord` 表名契约、`0005`/`0006` 迁移制品检查；opt-in `FILE_SCHEMA_MYSQL_TEST` 在无 DSN 时 skip）。
- 前端：Node `22.22.2` 下 `npm run build` 通过，`npm test`（Vitest）为 7 files / 10 tests 全量通过（含注册页不持久化并提交一次性 license capability token；Pro Components 在测试中通过 stub 隔离）。历史记录：2026-07-24 前后曾因 Ant Design 模块初始化挂起未取得绿证，已在后续用 test-only stub 修复，不再作为当前阻断或开放项。
- 小程序：Node `22.22.2` 下 Vitest 为 11 files / 17 tests 全量通过。
- 三条 smoke 脚本通过 `node --check`，已移除固定管理员/测试商家口令；管理员凭据改为环境注入，临时商家密码每次随机生成。
- 主 smoke 已同步新库存断言：数量与总价、剩余库存不售罄、库存归零才 `SOLD`、关单释放预占且不改变上下架状态。
- F-09/F-16：专用测试服务器 MySQL 8.4.8 完整矩阵退出 0；修复前同一 AutoMigrate 测试因 `uk_parent_name` 1061 失败，修复后通过。
- F-02：专用项目 `secondhand-file-binding-acceptance` 完整矩阵退出 0；六类脏引用均以 SQLSTATE 45000 在 DDL 前失败，干净回填、PUBLIC/MERCHANT 未绑定文件、注册认领、商品绑定、并发单赢家及 AutoMigrate 兼容均通过。资源与脱敏证据保留在独立测试目录，生产容器 ID/状态/重启计数前后一致。
- F-06：在代码提交 `c598f38` 上执行 `make test`，全部 Go 包通过；`go vet ./...` 通过；frontend Vitest 为 12 files / 25 tests 全量通过，`npm run build` 成功；`anonymous-upload-governance-smoke.sh` 通过 `bash -n`。补充提交 `03309d1` 修复 code `10013` 中文映射，`8bba664` 覆盖真实 Axios rejected HTTP 409 分支；反向变异使 resolved/rejected 两条 quota 用例准确失败，恢复后 focused 5/5、frontend 12 files / 27 tests、production build 均通过。本次补充使用 Node `19.7.0`，不替代锁定 Node `22.22.2` 的服务器审核；这些结果也不替代 MySQL 8.4 并发、迁移引擎和 Nginx 入口验收。
- F-11：提交 `6f84cc6` 的 120 文件 committed whitelist 在专用 Compose 项目中完整退出 0。MySQL 8.4.8、0008/0009 成功与拒绝矩阵、最终/API schema、AutoMigrate false/true、full/race/vet 均通过；`forbidden_matches=0`，26 个 evidence hash 全部校验通过，三个生产容器固定字段快照前后字节相同。脱敏报告见 `docs/superpowers/reviews/2026-07-27-buyer-intent-open-uniqueness-isolated-acceptance.md`。
- F-15：代码提交至 `15f57dd` 后，focused idempotency/buyer/order 套件、串行 `go test ./... -count=1`、`go test -race ./internal/app ./tests -count=1`、`go vet ./...`、shell/gofmt/diff 门禁均通过；最终 race 的 `tests` 包耗时 368.402 秒。首次与 race 并行的 full 因一个信号 fixture 五秒 ready-file 超时未被接受；该 fixture 单独 2.99 秒通过，随后串行 full 全绿。提交 `ce8787a` 的 127 个 `HEAD` 白名单路径禁入项为 0，metadata-free package 校验通过。唯一一次已授权服务器运行通过源码与 MySQL 8.4 门禁，但在 `test_metadata` 因 root 容器写宿主目录而失败；生产固定字段快照前后一致，脱敏失败报告见 `docs/superpowers/reviews/2026-07-28-idempotency-atomicity-isolated-acceptance-failure.md`。`f46bb3c` 已加入宿主 UID:GID/`HOME=/tmp` 契约，独立复审无问题，fresh focused/full/shell/gofmt/diff 门禁通过；以上仍不构成测试服务器批准。

生产数据克隆隔离验收已通过：MySQL 8.4.8 迁移、索引、CHECK、AutoMigrate 兼容性、并发、管理员安全以及桌面/移动浏览器验收均已完成。生产迁移、部署、管理员轮换和真实生产写验证仍未执行。三条 smoke 会创建测试业务数据，本轮没有在生产执行。

`0004` 正式门禁文件（preflight / up / postflight）已入库；`0005` 文件表对齐门禁亦已入库。维护窗顺序见下文；生产必须跑 `backend/migrations/` 下正式文件，而不是只发镜像 + AutoMigrate。

The canonical file table is `file_records`. Migration `0005` renames the
legacy `files`-only state, treats the existing `file_records`-only state as a
verified no-op, and stops when both or neither table exists. Isolated MySQL
8.4 acceptance passed with `make acceptance-file-schema-smoke` on the
dedicated host. Migration `0005` has not been run in production and still
requires separate production authorization.

Production migration, deployment, administrator rotation, and real production write verification remain undone.

## 3. 生产发布前置条件

生产维护窗只按以下顺序进行；任何一步不满足即停止，不跳过或重排：

```text
recoverable backup evidence
-> protected yaner fingerprint
-> 0004 preflight
-> 0004 up migration exactly once
-> 0004 postflight
-> 0005 file_records preflight
-> 0005 file_records up migration exactly once
-> 0005 file_records postflight
-> 0006 file binding ownership preflight
-> 0006 file binding ownership up migration exactly once
-> 0006 file binding ownership postflight
-> 0007 license file privacy preflight/up/postflight
-> 0008 anonymous upload governance preflight
-> 0008 anonymous upload governance up migration exactly once
-> 0008 anonymous upload governance postflight
-> 0009 buyer intent open uniqueness preflight
-> 0009 buyer intent open uniqueness up migration exactly once
-> 0009 buyer intent open uniqueness postflight
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

三份唯一的 `0005` 门禁文件为：

```text
0005_file_records_table.preflight.sql
-> 0005_file_records_table.up.sql
-> 0005_file_records_table.postflight.sql
```

- `backend/migrations/0005_file_records_table.preflight.sql`
- `backend/migrations/0005_file_records_table.up.sql`
- `backend/migrations/0005_file_records_table.postflight.sql`

三份唯一的 `0006` 门禁文件为：

- `backend/migrations/0006_file_binding_ownership.preflight.sql`
- `backend/migrations/0006_file_binding_ownership.up.sql`
- `backend/migrations/0006_file_binding_ownership.postflight.sql`

迁移前的停止条件：active 订单非零；`LOCKED` 商品非零；旧索引形态不是预期的唯一 `(product_id,is_active)`；缺少可恢复备份证据；或 `yaner` 缺失、重复、或无法采集发布前指纹。`LOCKED > 0` 时，先报告受影响行 ID 及其 active-order 计数，取得明确业务批准后才可逐行处置；不得批量改写所有受影响商品状态。文件表若同时存在 `files` 与 `file_records`，或两者皆无，`0005` preflight 必须失败并人工调查，不得合并或静默丢表。`0006` preflight 若发现孤儿引用、错误业务类型、非 `PASS`、空 URL、跨商家复用或上传账号归属不一致，也必须在 DDL 前停止。

生产不得运行 `smoke-mysql-concurrency.mjs`。生产写验证只使用小数量的专用测试商家/商品，依次执行创建 -> 关闭及创建 -> 完成；不得使用 `yaner` 数据，也不得仅为测试轮换现有管理员密码。

详细预检、迁移、发布和回滚步骤以 [生产加固与商家多库存修复方案](./production-hardening-repair-plan-2026-07-24.md) 为准。
F-09 设计见 [file-record-schema-alignment-design](./superpowers/specs/2026-07-26-file-record-schema-alignment-design.md)。

## 4. 当前阻断项

- 生产数据库备份与迁移证据未完成。
- 新后端与商家前端尚未部署，生产管理员尚未逐个改密。
- 真实生产写验证、`yaner` 发布前后指纹比较和 30-60 分钟观察尚未执行。

这些事项阻断多库存版本正式上线。F-12、F-13、license governance、miniapp ordering、MySQL root rotation 均在本次发布范围外，不得借此扩大生产窗口。

frontend Vitest、F-08 logout、F-02、F-06、F-09、F-11、F-15 与 F-16 已在本地代码侧关闭；F-02/F-09/F-11/F-16 也已通过隔离 MySQL 8.4 测试服务器审核。F-15 首次运行因验收 harness 所有权边界失败，修复后的第二次运行待重新授权，因此测试服务器状态仍是未批准。F-06 专用治理矩阵仍单独跟踪。F-12 的 F-11 前置已经满足，但 F-12 实现和验收仍在本次范围外。生产 frontend/backend bundle 与 `0004`/`0005`/`0006`/`0007`/`0008`/`0009` 仍须在各自批准的维护窗部署/执行后才对线上生效，不能据此标记为生产已完成。F-04 与 F-13 仍按各自台账处理，不因 F-06/F-11/F-15 代码或测试服务器状态改变生产状态。

## 5. 回滚边界

- 新库存语义开放前可回滚应用并保留新增列。
- 一旦产生多笔 active 或多笔 inactive 历史订单，不得恢复旧唯一索引或直接回滚旧订单代码，应关闭创建入口并向前修复；生产回滚只允许前向修复。
- 管理员口令轮换后不得恢复仓库历史公开口令。
- 任何回滚都不得删除、禁用、改名或重置 `yaner`，也不得把其多库存商品机械改为 1。
- `0005` 无 destructive down：应用回滚保留 `file_records`，不得把表改回 `files`（否则会重新打开 F-09）。
- `0006` 无 destructive down：应用回滚保留归属/capability 列和索引；不得清空已回填归属或恢复 capability token。
