# 隔离环境验收结果（2026-07-24）

## 1. 结论

分支 `codex/reconcile-code-reviews` 上的管理员加固与商家后台多库存实现，已在生产数据克隆、MySQL 8.4.8 和生产模式构建组成的隔离环境中通过迁移、接口并发、管理员安全和桌面/移动浏览器验收。

本结论只覆盖 A2c（管理员改密与 session 吊销）和 Checkpoint C（商家后台多库存订单）。它不是生产发布记录，也不代表整份历史审核中的 F-12、F-13、营业执照治理、买家小程序下单或其他后续项已经关闭。

本次没有向生产部署新代码，没有执行生产迁移，没有修改生产管理员，也没有修改、重置或使用 `yaner` 创建测试订单。

## 2. 被测版本与环境

- 业务修复提交：`69d0820 fix: harden admin access and support multi-stock orders`
- 验收工具版本：本报告所在提交中的 Compose、SQL gate、Dockerfile 和 3 个 Node 脚本
- 验收仓库：`/home/yu/services/secondhand-market-acceptance`
- Compose 项目：`secondhand-acceptance`
- MySQL：8.4.8，镜像摘要 `mysql@sha256:be18eb9dc45eea9b86cb74f8c68ab92ce8569ecc37ea4e6fade02e37036c5ff4`，使用独立 volume，无宿主端口
- API：仅 `127.0.0.1:18082`
- Web：仅 `127.0.0.1:18081`
- 网络：MySQL 只连接内部 `acceptance`；API 连接 `acceptance` 与 `edge`；Web 只连接 `edge`
- 生产克隆：80,924 字节，SHA-256 `850f318787535cd8443cfcba3653ef6bb033ed5f7c634bdd4fe425594900688f`

验收 API 最终以容器环境 `AUTO_MIGRATE=true` 重建，用于验证 GORM 启动不会恢复已删除的旧唯一索引。服务器上的验收 `.env` 仍保持 `AUTO_MIGRATE=false`，两者差异是本次显式测试覆盖，不是生产配置变更。

## 3. 验收结果

| 项目 | 结果 | 关键证据 |
| --- | --- | --- |
| Compose 隔离 | 通过 | MySQL 无端口；Web/API 仅回环监听；网络按数据库与边缘流量拆分 |
| 克隆预检 | 通过 | 9 个商品、2 笔历史订单、0 笔 active 订单；无负库存、孤儿订单或重复订单号 |
| SQL 迁移 `0004` | 通过 | 只执行一次；新增 `quantity`、`reserved_stock`、3 个 CHECK；删除 `uk_product_active`，建立普通索引 |
| 历史数据兼容 | 通过 | 历史订单 `quantity=1`；迁移后预占为 0；既有 `SOLD / stock>0` 商品保持冻结，未自动修数 |
| AutoMigrate 兼容 | 通过 | `AUTO_MIGRATE=true` 重启后旧唯一索引没有被重建 |
| 管理员改密 | 通过 | 一次性验收管理员改密后，旧密码、旧 access、旧 refresh 立即失效；control 管理员不受影响 |
| 主业务 smoke | 通过（无独立输出文件） | 注册、审核、登录、商品和订单主链路均使用验收专用数据完成；执行时观察通过，但未单独保存 stdout |
| MySQL 并发 | 通过 | 库存 5、10 个并发创建恰好 5 成功/5 冲突；双完成、双关闭、完成/关闭竞争均只变更库存一次 |
| 数量与价格 | 通过 | 数量、单件成交价和自动总价返回一致；非正值、超 INT、库存不足等边界均被拒绝 |
| SQL 最终门禁 | 通过 | 精确校验列类型/默认值、索引列顺序和 CHECK 表达式；active 订单 0、总预占 0、无旧索引或遗留状态 |
| `yaner` 保护 | 通过 | 账号密码哈希及所属商品稳定字段在迁移、并发和 UI 验收前后逐字一致 |
| 桌面/移动 UI | 通过 | 1440x900 完成创建、关闭、完成和详情链路；390x844 验证表单预览、详情可读和操作可达 |

### 3.1 管理员安全用例

测试使用 `acceptance_rotate_evidence` 和 `acceptance_control` 两个只存在于隔离克隆的管理员：

1. 当前密码错误和弱新密码均被拒绝。
2. 只更新目标管理员密码。
3. 改密前 access token、refresh token 和旧密码立即失效。
4. 新密码可以登录。
5. control 管理员的既有 session 仍可访问受保护接口。

没有对克隆中的 `admin`、`superadmin`、商家账号或 `yaner` 做密码操作。

### 3.2 多库存与并发用例

最终一轮 `smoke-mysql-concurrency.mjs` 完整执行 5 组用例：输入边界、10 路库存竞争、双完成、双关闭、完成/关闭竞争。清理后 SQL 门禁显示：

- active 订单：0
- 总预占库存：0
- 旧 `uk_product_active`：不存在
- `idx_order_product_active(product_id, is_active)`：普通索引存在
- `reserved_stock` 与 active 订单数量一致

边界用例会创建 `INT_MAX` 库存的隔离测试商品，因此最终测试库的库存合计很大；该数值来自验收数据，不是生产库存变化。

### 3.3 商家后台 UI 用例

浏览器只使用一次性验收商家和商品：

- 数量 5、单价 60.50 元时，总价自动计算为 302.50 元；关闭后库存恢复为总库存 5、已预占 0、可售 5。
- 数量 2、单价 50.00 元时，总价自动计算为 100.00 元；完成后库存为总库存 3、已预占 0、可售 3，商品仍为在售。
- 移动视口再次验证数量 3、单价 12.34 元时预览总价为 37.02 元。
- 订单列表和详情均显示数量、单价、总价及总库存/已预占/可售库存。

390 像素宽度下，订单详情中的长订单号和金额会换行，表格通过横向滚动展示列；信息仍可读且操作可达。该项记录为非阻断的响应式体验改进，不扩大本轮修复范围。

## 4. `yaner` 与生产只读复核

隔离环境中 `protected-before.txt` 与最终 `protected-final.txt` 的 SHA-256 均为：

`6443f6ba4d1829edf93d1758ad862fe8b5c70e86c51f603bec802f4c1c4d9197`

该比较覆盖 `yaner` 账号的稳定身份/密码哈希字段以及所属商品的编号、状态、库存和价格字段。迁移拥有的新增字段不参与指纹，避免把预期 schema 变化误报为账号变化。

生产只读复核结果：

- `https://market.meaningful.ink/` 返回 HTTP 200。
- 公网直连宿主机 `8081` 失败；生产 Web 容器只绑定 `127.0.0.1:8081`。
- 生产 API `/healthz` 返回 `code=0`。
- 生产 API、Web、MySQL 容器重启次数均为 0。
- 复核期间生产 API/Web/MySQL 约占 10.89 MiB、4.29 MiB、475.2 MiB；验收栈停止前约占 8.02 MiB、3.22 MiB、484.9 MiB。

## 5. 证据位置与校验值

服务器证据目录：

`/home/yu/services/secondhand-market-acceptance/deploy/acceptance/evidence`

关键文件：

- `preflight.txt`
- `post-migration.txt`
- `post-smoke.txt`
- `post-smoke-after-automigrate.txt`
- `post-smoke-after-ui.txt`
- `mysql-concurrency-final.txt`
- `admin-security-final.txt`
- `post-smoke-final.txt`
- `protected-before.txt`
- `protected-final.txt`

最终证据 SHA-256：

- `admin-security-final.txt`: `b50b357559ba9e9939cc2dead068f4d35d1459bf7e704029923a439105cb1607`
- `mysql-concurrency-final.txt`: `55b799f5e5d12fe87dcf833dcd0829bd8f8da4fd8a24deb045b24cd13dc6a314`
- `post-smoke-final.txt`: `3aa266f834f5305d4c7a22fd1bcb067c74b1fea838353a0e58eb1f482642be42`
- `protected-before.txt` / `protected-final.txt`: `6443f6ba4d1829edf93d1758ad862fe8b5c70e86c51f603bec802f4c1c4d9197`

证据目录、生产克隆、`.env` 和密码文件均不进入 Git。报告不包含密码、token、DSN、JWT、原始营业执照或个人信息。

本机 SSH 隧道已关闭，包含 UI 商家明文凭据、control/rotate 管理员口令副本和临时 smoke 输出的 `0700` 临时目录已删除。服务器仍保留以下受控材料供复核：`backups/production-clone-20260724.sql`、验收 `.env`、验收管理员密码文件、evidence、已停止容器和独立 volume；克隆、`.env` 和两个密码文件权限均为 `0600`。这些材料不随 `docker compose stop` 删除；复核结束后的删除或保留期限需要单独批准，不得把生产克隆复制进仓库或普通共享目录。

## 6. 本地验证

已通过：

- `go test -timeout 180s -v ./...`（使用 `/private/tmp` 构建缓存）
- `frontend: npm run build`
- `miniapp: npm test`（11 个测试文件、17 个用例）
- 3 个新增 Node 脚本的 `node --check`
- `deploy/acceptance/prepare.sh` 的 `bash -n`
- 远程 `docker compose config --quiet`
- `git diff --check`

未取得完成结果：`frontend: npm test`。Node 23 与 22.22.2 下，全量及单文件 Vitest 都在加载 Ant Design 组件的模块初始化阶段持续等待，未进入业务断言，也未输出失败断言；会话在确认无进展后人工中止。曾尝试用远程 Linux Docker 构建阶段复跑，但服务器未保留 Node 构建阶段镜像，临时构建在获取 Docker Hub 基础镜像元数据时持续等待，同样在进入测试前中止。生产构建成功，且本次修改涉及的 UI 行为已由隔离环境真实浏览器覆盖，但不能据此宣称 frontend Vitest 全绿，应另行修复或隔离该测试运行器问题。

## 7. 剩余边界

1. 新代码和迁移仍未部署生产；正式发布必须继续按维护窗方案执行预检、备份、迁移、同步发布和发布后门禁。
2. `yaner` 的一个历史 `SOLD / stock>0` 商品仍保持冻结，等待实际使用者确认，不得机械修数。
3. 买家小程序当前仍不开放直接下单；本轮只完善商家后台，不提前实现未确认的买家下单需求。
4. F-12、F-13 和营业执照治理按已约定的真实用户/真实资质接入门槛另行处理。
5. 隔离容器和 volume 已保留供复核，三个服务已执行 `docker compose stop` 释放内存；未执行 `down --volumes`。
