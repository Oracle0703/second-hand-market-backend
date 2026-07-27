# 0008 MySQL 8.4 索引 HAVING 兼容性设计

**日期：** 2026-07-28

**分支：** `codex/f11-buyer-intent-open-uniqueness`

**状态：** 方案与完整设计已批准；书面规格待审核；实现尚未开始；失败的隔离验收现场已保留

**问题归属：** F-11 隔离验收暴露的 F-06/0008 MySQL 8.4 兼容性跟进

## 1. 问题与证据

F-11 的第二次隔离验收使用提交
`e55be6a8ecd7f32fc644a136bf6c35820d537fed`，向
`aliyun-server:/home/yu/services/secondhand-buyer-intent-acceptance-20260727`
传输了 `BUYER_INTENT_SOURCE_LIST_ONLY=1` 产生的 120 个已提交文件。本地与远端
source manifest 字节相同，其文件 SHA-256 均为：

```text
b9b230c6706bfb399ad2679b92c4ca3a58d6f176ca18176dcd641c3a1cccc226
```

专用 Compose 项目 `secondhand-buyer-intent-acceptance` 启动了隔离 MySQL
8.4.8。迁移链通过 0007 postflight 后，在
`0008_anonymous_upload_governance.preflight.sql` 的过程调用处失败：

```text
ERROR 1054 (42S22): Unknown column 'non_unique' in 'having clause'
```

命令退出 2，尚未进入 0009。该次授权只允许运行一次，因此没有重跑、清理、覆盖
或读取失败 evidence；远端专用资源、secrets 和 evidence 保持原状。生产 0008/0009
均未执行，未部署，也未修改生产数据、配置、上传文件或服务。

本地使用全新 `mktemp` 数据目录、`--no-defaults`、Unix socket 和
`--skip-networking` 启动隔离 MySQL 9.3 探针。与 0008 相同的派生查询稳定复现
1054；投影 `non_unique AS is_non_unique` 并在 HAVING 使用该别名后退出 0，且两个
预期索引均被识别：

```text
alias_probe_count
2
```

探针实例随后正常停止。它只证明 SQL 解析和结果形状；MySQL 8.4 的最终证据仍必须
来自新的、精确授权的隔离验收。

## 2. 根因

0008 的四个派生查询按 `index_name, non_unique` 分组，并在 HAVING 中直接引用
`non_unique`，但 SELECT 列表只投影 `index_name`。MySQL 在该派生查询形状中不能
解析未投影的分组列，因而在执行存储过程时返回 1054，而不是进入设计要求的
schema 判定或 `SIGNAL SQLSTATE '45000'`。

仓库的 0004 迁移已经使用兼容模式：投影
`non_unique AS is_non_unique`，再在 HAVING 中引用 `is_non_unique`。本设计复用该
既有模式，不引入新的 SQL 抽象。

## 3. 目标

1. 让 0008 preflight、up 和 postflight 在 MySQL 8.4 上可解析并执行。
2. 保持现有索引名称、列顺序、唯一性和数量判定完全不变。
3. 保持 schema drift 继续以 `SIGNAL SQLSTATE '45000'` fail closed。
4. 以先失败后通过的自动化契约覆盖全部四个受影响查询。
5. 重新通过 F-06 聚焦门禁、F-11 当前迁移链门禁、完整后端质量门禁和独立审阅。
6. 在取得新的精确授权前，不触碰保留的远端失败项目。

## 4. 非目标与保护边界

- 不改变 0008 的 DDL、DML、表、列、索引、固定 guard 行或迁移顺序。
- 不修改 0001..0007、0009 或应用运行时代码。
- 不放宽 0008 对 InnoDB、列定义、索引形状、历史行和 quota guard 的校验。
- 不新增 down migration，不修复或改写任何业务行。
- 不读取远端 `.env`、secrets、数据库、上传文件、备份或失败 evidence。
- 不重启、迁移、部署或修改生产容器、数据库、配置、服务和数据。
- 不修改、读取、提交或传输三份受保护审查文档、`.tmp/` 或 `backend/app.db`。
- 不在原授权下清理或重跑
  `/home/yu/services/secondhand-buyer-intent-acceptance-20260727`。

## 5. 采用方案

每个受影响的查询使用同一形状：

```sql
SELECT index_name, non_unique AS is_non_unique
FROM information_schema.statistics
WHERE table_schema = DATABASE()
  AND table_name = '<expected table>'
GROUP BY index_name, non_unique
HAVING (
  index_name = '<expected index>'
  AND is_non_unique = <0 or 1>
  AND COUNT(*) = <expected column count>
  AND GROUP_CONCAT(column_name ORDER BY seq_in_index) = '<expected columns>'
)
```

只修改以下四处：

| 文件 | 查询职责 | 预期别名投影数 |
| --- | --- | ---: |
| `0008_anonymous_upload_governance.preflight.sql` | 0006 capability 两个索引 | 1 |
| `0008_anonymous_upload_governance.preflight.sql` | quota guard 两个索引 | 1 |
| `0008_anonymous_upload_governance.up.sql` | 已存在 quota guard 的恢复状态 | 1 |
| `0008_anonymous_upload_governance.postflight.sql` | 最终 quota guard 两个索引 | 1 |

WHERE 中用于预过滤的 `AND non_unique = 0/1` 不属于故障形状，保持不变。其他迁移
也不做机械替换。

## 6. 数据与错误语义

该修改只影响 `information_schema.statistics` 的只读 SELECT 投影和 HAVING 名称解析：

- 正确 schema 继续产生相同的 `v_count`。
- 缺失、多余、错误顺序或错误唯一性的索引继续被拒绝。
- schema drift 继续由现有 `SIGNAL SQLSTATE '45000'` 和稳定消息报告。
- 不执行新增的 ALTER、CREATE、DROP、INSERT、UPDATE、DELETE 或 REPLACE。
- 不读取或写入业务行，因此无需数据回滚或 down migration。

若实现后仍出现 SQL parser/error 1054，验收必须失败并保留现场；不得把 parser
错误当作预期的 45000 拒绝结果。

## 7. 测试设计

### 7.1 RED 契约

在 `backend/migrations/anonymous_upload_governance_migration_test.go` 增加
`TestAnonymousUploadGovernanceGroupedIndexHavingProjectsNonUnique`：

1. 读取 0008 preflight、up 和 postflight。
2. 规范化空白，但不改写标识符。
3. 分别统计 `GROUP BY index_name, non_unique` 与
   `SELECT index_name, non_unique AS is_non_unique`。
4. 要求每个 grouped-HAVING 查询都有且只有一个对应别名投影。
5. 要求 HAVING 使用 `is_non_unique` 完成唯一/非唯一判定。
6. 固定 preflight 为两处、up 为一处、postflight 为一处，防止遗漏或重复修复。

该测试必须先在当前提交上失败，失败原因应为四个查询均未投影别名，而不是测试语法
或文件读取错误。

### 7.2 GREEN 与回归门禁

完成最小 SQL 修改后依次运行：

```bash
cd backend
gofmt -w migrations/anonymous_upload_governance_migration_test.go
go test ./migrations -run 'TestAnonymousUploadGovernanceGroupedIndexHavingProjectsNonUnique' -count=1
go test ./migrations -run 'TestAnonymousUploadGovernance|TestBuyerIntentOpenUniqueness' -count=1
go test ./migrations -count=1
cd ..
bash -n deploy/acceptance/anonymous-upload-governance-smoke.sh
bash -n deploy/acceptance/buyer-intent-open-uniqueness-smoke.sh
git diff --check
```

随后使用仓库本地 Go cache 运行：

```bash
cd backend
env GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" go test ./... -count=1
env GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" go test -race ./... -count=1
env GOMODCACHE="$(pwd)/.cache/go/mod" GOCACHE="$(pwd)/.cache/go/build" go vet ./...
```

本地隔离 MySQL 探针记录设计选择的可行性，但不替代新的 MySQL 8.4 服务器验收。

## 8. 审阅与提交边界

实施提交只允许包含：

```text
backend/migrations/0008_anonymous_upload_governance.preflight.sql
backend/migrations/0008_anonymous_upload_governance.up.sql
backend/migrations/0008_anonymous_upload_governance.postflight.sql
backend/migrations/anonymous_upload_governance_migration_test.go
```

实施完成后必须进行独立规格和代码审阅，重点检查：四处查询是否全部覆盖、索引判定
是否等价、是否混入 DDL/DML 或范围外修改、测试是否能在回退 SQL 修复后重新失败。
Critical/Important finding 必须通过新的 RED/GREEN 修复轮次关闭。

设计文档与后续实施计划使用独立文档提交，不与 SQL 实现混在同一提交中。

## 9. 远端重跑与证据

本地修复和审阅完成后，必须重新生成提交白名单和 source manifest。再次执行服务器
验收前，需要新的精确书面授权，至少明确允许：

1. 删除且仅删除失败目录
   `/home/yu/services/secondhand-buyer-intent-acceptance-20260727`。
2. 删除且仅删除 Compose 项目 `secondhand-buyer-intent-acceptance` 的已保留容器、卷
   和网络，并在删除前解析其精确 ID。
3. 以 0700 重建同一路径，只传输新提交的 committed whitelist。
4. 核对本地/远端 SHA-256，重新生成远端专用 secrets，并只运行该项目一次。
5. 只有新运行成功且漏扫通过后，才只读审计新生成的脱敏 evidence。

新授权不得扩大生产检查范围。生产侧仍只允许读取三个固定容器的名称、ID、状态和
重启计数；禁止生产 SQL、日志、环境变量、挂载、配置、迁移、部署和数据修改。

通过后的接受报告必须同时记录 0008 compatibility gate、0009 全矩阵、完整/race/vet
结果、source/evidence manifest、生产快照相等性和生产未部署声明。未通过或证据不完整
时，F-11 测试服务器状态继续保持未审核，F-12 继续被阻塞。

## 10. 回滚

在生产迁移前，代码回滚只需 revert 本兼容性实施提交。由于该提交不改变数据或 DDL，
无需 SQL 回滚。若新的隔离验收失败，保留其独立资源和 evidence，并按新的精确授权
处理；不得在失败现场就地覆盖或继续尝试。

## 11. 完成标准

- 方案和完整设计已获书面批准。
- 本书面规格已获审核批准。
- 实施计划逐文件、逐命令记录 RED、GREEN、审阅和提交步骤。
- 四个受影响查询均使用批准的别名模式，且没有范围外 SQL 修改。
- 聚焦、全部 migration、full、race、vet、脚本语法和 diff 门禁全部通过。
- 独立审阅无未关闭的 Critical/Important finding。
- 新的精确授权下，隔离 MySQL 8.4 验收退出 0、漏扫通过且证据完整。
- F-11/F-06 状态文档准确区分代码、本地测试、测试服务器和生产状态。
- 生产 0008/0009 未执行，未部署，生产数据、配置、上传和服务未修改。
