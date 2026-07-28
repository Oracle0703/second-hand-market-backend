# F-01 至 F-15 提交检查点状态

**记录日期：** 2026-07-29

**分支：** `codex/f15-idempotency-atomicity`

**状态口径：** 本文只记录当前分支已经存在的代码、测试和既有隔离验收证据。
“代码侧已做”不等于“测试服务器审核通过”，也不等于“已部署生产”。本检查点不修改
线上数据、线上文件、生产服务、生产配置或生产数据库。

## 总表

| 问题 | 当前状态 | 隔离测试服务器 | 仍未完成 |
| --- | --- | --- | --- |
| F-01 生产弱默认值与管理员凭据 | **部分完成**：默认值防护、安全 bootstrap、自助改密和 session 吊销已在代码侧实现 | 生产数据克隆管理员安全矩阵已有通过记录 | 生产管理员及应用/root 数据库口令未在批准维护窗轮换，不能标记为生产关闭 |
| F-02 文件绑定授权 | **已做（代码侧）**：归属、用途、扫描状态、URL 和一次性 capability 已实现 | **已通过**独立 MySQL 8.4.8 矩阵 | 生产 `0006`、frontend/backend 发布未执行 |
| F-03 多库存订单 schema | **已做（代码侧）**：`0004` 三段迁移、数量/单价/总价和多 active 订单已实现 | **已通过**生产数据克隆 MySQL 8.4.8 矩阵 | 生产 `0004` 和应用发布未执行 |
| F-04 管理员营业执照预览 | **部分完成**：业务代码已实现；最新 acceptance provenance 加固未闭环并已交接 | **未通过/未完成**专用审核 | Compose/`.env` 快照加固、完整本地门禁、独立复审及服务器审核均未完成 |
| F-05 小程序 401 token refresh | **部分完成**：业务功能已实现；最新 acceptance exporter 加固仍失败 | **未通过**锁定 Node/npm 的专用审核 | 两个 exporter 身份/清理合约失败；Node `22.22.2` / npm `10.9.7` 完整门禁及服务器审核未完成 |
| F-06 匿名上传资源治理 | **部分完成**：业务代码、`0008` MySQL 8.4 HAVING 兼容性和前端 `10013` 映射已实现 | 专用治理矩阵**未完成** | 当前 acceptance provenance checkpoint 尚未跑完整合约、复审或专用服务器审核；生产 `0008` 未执行 |
| F-07 库存事务与状态语义 | **已做（代码侧）**：预占、完成双减、关闭释放及竞争语义已实现 | **已通过**生产数据克隆并发矩阵 | 生产迁移和应用发布未执行 |
| F-08 frontend logout | **已做（代码侧）**：调用服务端 logout，失败仍清理本地会话 | 本地 8/8 测试已有通过记录；无独立服务器批准记录 | 新 frontend 未发布 |
| F-09 文件表 schema | **已做（代码侧）**：`file_records` 唯一契约及 `0005` 三段迁移已实现 | **已通过**MySQL 8.4.8 八态矩阵 | 生产 `0005` 未执行 |
| F-10 Git 跟踪本地业务数据库 | **未做完**：`backend/app.db` 仍在当前索引中，且仍可从 4 个历史提交到达 | 不适用 | 未获得实际历史改写/强推授权；当前索引移除、引用审计、历史对象清理均未执行 |
| F-11 买家意向 open 唯一性 | **已做（代码侧）**：接受提交 `6f84cc6` | **已通过**独立 MySQL 8.4.8 完整矩阵 | 生产 `0009` 和应用发布未执行 |
| F-12 生产买家 mock 身份与迁移 | **未实现**：F-11 前置已满足，但仓库中没有 `0010`、四表 claim 实现或专用 harness | **未审核** | 四表 claim 并发不变量、迁移、应用逻辑、本地测试及服务器验收均未完成 |
| F-13 营业执照私有访问 | **部分完成**：与 F-04 共享的私有访问业务代码已实现 | **未通过/未完成**专用审核 | 与 F-04 相同的 acceptance provenance、完整门禁及服务器审核未完成；生产 `0007` 未执行 |
| F-14 session access 吊销 | **部分完成**：每个认证请求校验当前 session/账号状态已在代码侧实现 | **未审核** | 当前 acceptance provenance checkpoint 尚未完成完整合约、复审和专用服务器运行；生产未发布 |
| F-15 原子幂等与失败回滚 | **部分完成**：代码侧和本地 focused/full/race/vet 已有通过记录，metadata UID/GID 修复已提交 | 首次服务器运行在 `test_metadata` 失败，修复后**尚未重跑通过** | 需要重新授权并完成隔离 MySQL 8.4 全矩阵；生产未发布 |

## 本次已提交内容

- `fec048c`：记录 F-04/F-13 acceptance provenance 未闭环范围、已通过的定向用例、
  未完成门禁和 Claude 接手命令。
- `dc2c339`：提交四个 acceptance shell 与四个 Go contract 的当前
  provenance hardening checkpoint。该提交是检查点，不代表四项修复完成。
- 四个 Go contract 已执行 `gofmt`；四个 shell 已通过 `bash -n`；
  `git diff --check` 已通过。

## 最新失败证据

在 `dc2c339` 上执行：

```bash
cd backend
go test ./tests -run '^TestMiniappAuthRefreshAcceptance' -count=1
```

结果为 `FAIL`（`backend/tests` 66.083s），失败项：

```text
TestMiniappAuthRefreshAcceptanceSourceExportUsesImmutableHEAD/
  identity_observation_replacement_cannot_redirect_artifacts
TestMiniappAuthRefreshAcceptanceSourceExportUsesImmutableHEAD/
  cleanup_replacement_cannot_recursively_delete_new_owner
```

因此 F-05 不能标记为 acceptance 加固完成，`dc2c339` 也不能作为全量测试通过
或测试服务器批准的证据。

## 当前明确未完成的集合

1. F-04/F-13、F-05、F-06、F-14 的 acceptance provenance 完整修复、全量门禁和
   专用测试服务器审核。
2. F-10 当前索引清理和历史改写；历史改写、对象清理和强推仍需要单独明确授权。
3. F-12 四表 claim 设计修订、`0010`、应用实现、本地门禁和独立 MySQL 8.4 验收。
4. F-15 修复后的独立 MySQL 8.4 重跑与审核通过记录。
5. 所有明确标为“生产未执行/未发布”的迁移、部署和口令轮换；本检查点没有执行
   任何生产操作。

## 安全边界

本次检查点没有 SSH、源码传输、远端 Docker、服务器数据库操作、生产 SQL、生产
日志/环境变量/挂载/配置读取、部署、迁移或线上数据/文件修改。三份受保护审查文档、
`.tmp`、`backend/app.db` 内容、`.env`、密钥、数据库、上传、备份、缓存、
`node_modules` 和已有 evidence 均未读取、修改、暂存或提交。
