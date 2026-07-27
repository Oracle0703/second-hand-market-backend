# F-11 隔离验收资源身份验证修订设计

**日期：** 2026-07-28

**分支：** `codex/f11-buyer-intent-open-uniqueness`

**状态：** 方案 A 已批准；合并授权已批准；书面规格待独立审阅；远端清理和验收尚未重新执行

**问题归属：** F-11 隔离 MySQL 8.4 验收 Task 3 的远端资源预清理门禁

## 1. 问题与证据

提交 `fc8b6a95c7be56e719f656614daa91f40b33fa06` 的 0008 HAVING
兼容性修复已通过本地门禁和独立审阅。Task 3 在重新运行 F-11 隔离验收前，先对
保留的专用 Compose 资源执行只读身份验证。

本地源码门禁通过：

- committed whitelist：120 个文件；
- source manifest SHA-256：
  `ee7694a2d5c7ceed37243cbfc2c1d24adc4843db9fd91ea0a3ceea452118740b`；
- 白名单不包含 `.env`、secrets、数据库、上传、备份、evidence、`.git`、缓存、
  `node_modules`、`backend/app.db`、`.tmp`、`.superpowers` 或受保护审查文档。

远端只读门禁随后退出 1，且发生在任何删除、传输、prepare 或验收运行之前。诊断证明
目标目录存在且不是符号链接，精确项目标签下恰有一个停止的 MySQL 容器、一个卷和
一个网络，名称与标签均正确。错误来自计划中的两个类型混淆：

1. `docker container ls -aq` 默认输出 12 字符短 ID，计划却将其与
   `docker inspect .Id` 返回的完整 ID 做字符串全等比较。
2. `docker network ls -q` 返回网络 ID，计划却将该值直接与预期网络名称比较。

本次失败没有删除 Docker 资源或目录，没有传输文件，没有运行 `prepare.sh`，没有创建
单次运行标记，验收命令执行次数为 0，成功专用 evidence 未读取。原授权按失败规则
终止，远端现场保持原状。

## 2. 根因

计划把 Docker CLI 的不同输出类型当成同一种标识符：

- container list 的默认值是短 ID；container inspect 的 `.Id` 是完整 ID；
- network list 的 quiet 值是 ID，不是名称；
- volume list 的 quiet 值才是卷名称。

资源本身没有漂移。失败是执行计划没有为容器 ID、网络 ID、资源名称建立独立变量和
逐字段断言，也没有在审阅中验证 `--no-trunc` 语义。

## 3. 目标

1. 在任何远端变更前，以完整 ID、精确名称、精确项目标签和停止状态共同绑定唯一资源。
2. 容器、网络和卷使用符合各自 Docker CLI 输出类型的变量，不允许短 ID 前缀或名称/ID
   混用。
3. 保持原授权的 host、目录、Compose 项目、源码、证据和生产只读边界不变。
4. 只运行一次隔离 MySQL 8.4 验收；失败后不清理、不覆盖、不重启、不重跑。
5. 通过独立书面规格审阅后才消耗新的合并授权。

## 4. 非目标与保护边界

- 不修改 0008、0009、应用运行时代码、验收脚本、Compose 文件或依赖。
- 不改变资源预期数量、名称、项目标签或停止状态。
- 不允许短 ID 前缀匹配，也不允许只按名称删除容器或网络。
- 不使用 `docker system prune`、任何 broad prune 或针对其他目录/项目的
  `docker compose down`。
- 不读取 `.env` 或 secrets 内容、数据库、上传文件、备份、旧 evidence、`.git`、缓存、
  `node_modules`、`backend/app.db`、`.tmp` 或三份受保护审查文档。
- 不读取生产 SQL、日志、环境变量、挂载或配置；不执行生产迁移、部署、服务或数据修改。
- 生产侧仅允许读取三个固定容器的名称、完整 ID、状态和重启计数。
- 不把本次计划修订误报为 F-11 测试服务器验收通过。

## 5. 采用方案 A：完整 ID 类型化验证

### 5.1 固定授权字面值

远端脚本首先设置并验证：

```bash
target=/home/yu/services/secondhand-buyer-intent-acceptance-20260727
project=secondhand-buyer-intent-acceptance
[[ "$target" == /home/yu/services/secondhand-buyer-intent-acceptance-20260727 ]]
[[ "$project" == secondhand-buyer-intent-acceptance ]]
[[ -d "$target" && ! -L "$target" ]]
```

所有 Docker 查询都必须使用精确标签：

```text
com.docker.compose.project=secondhand-buyer-intent-acceptance
```

### 5.2 容器身份

使用 `--no-trunc` 获取完整容器 ID，并要求恰好一个：

```bash
mapfile -t container_ids < <(
  docker container ls -aq --no-trunc --filter \
    "label=com.docker.compose.project=$project"
)
[[ "${#container_ids[@]}" -eq 1 ]]
container_id="${container_ids[0]}"

container_record="$(docker inspect --type container --format \
  '{{.Id}}|{{.Name}}|{{ index .Config.Labels "com.docker.compose.project" }}|{{.State.Running}}' \
  "$container_id")"
IFS='|' read -r inspected_container_id container_name container_project container_running \
  <<<"$container_record"

[[ "$inspected_container_id" == "$container_id" ]]
[[ "$container_name" == /secondhand-buyer-intent-acceptance-mysql-1 ]]
[[ "$container_project" == "$project" ]]
[[ "$container_running" == false ]]
```

删除时只使用验证后的完整 `container_id`。

### 5.3 卷身份

`docker volume ls -q` 返回卷名称，因此单独使用 `volume_names`：

```bash
mapfile -t volume_names < <(
  docker volume ls -q --filter "label=com.docker.compose.project=$project"
)
[[ "${#volume_names[@]}" -eq 1 ]]
volume_name="${volume_names[0]}"
[[ "$volume_name" == secondhand-buyer-intent-acceptance_mysql-data ]]
[[ "$(docker volume inspect --format \
  '{{ index .Labels "com.docker.compose.project" }}' "$volume_name")" == "$project" ]]
```

删除时只使用验证后的精确 `volume_name`。

### 5.4 网络身份

使用 `--no-trunc` 获取完整网络 ID，并通过 inspect 独立验证名称和标签：

```bash
mapfile -t network_ids < <(
  docker network ls -q --no-trunc --filter \
    "label=com.docker.compose.project=$project"
)
[[ "${#network_ids[@]}" -eq 1 ]]
network_id="${network_ids[0]}"

network_record="$(docker network inspect --format \
  '{{.Id}}|{{.Name}}|{{ index .Labels "com.docker.compose.project" }}' \
  "$network_id")"
IFS='|' read -r inspected_network_id network_name network_project <<<"$network_record"

[[ "$inspected_network_id" == "$network_id" ]]
[[ "$network_name" == secondhand-buyer-intent-acceptance_acceptance ]]
[[ "$network_project" == "$project" ]]
```

删除时只使用验证后的完整 `network_id`。

## 6. 执行数据流

### 6.1 本地源码门禁

授权的可传输源码固定为提交
`fc8b6a95c7be56e719f656614daa91f40b33fa06`。设计和计划文档提交不会改变该源码边界。
新的临时目录重新生成 whitelist 和 manifest 后，每个路径必须同时满足：

- 是 `fc8b6a9` 中的 committed blob；
- 当前工作树对应文件的 hash 与该路径在 `fc8b6a9` 中的 blob hash 相同；
- list 有序、唯一，且与 manifest 路径集合字节一致；
- `sha256sum -c` 通过；
- forbidden-category 扫描为零。

只要任一 whitelist 路径与 `fc8b6a9` 不同，就在 SSH 前停止。

### 6.2 零变更远端预检

同一个远端只读阶段依次完成：字面路径断言、非符号链接断言、Docker CLI
`--no-trunc` 能力检查、三个资源计数和所有逐字段身份断言。该阶段不包含删除命令。

控制器必须先取得该阶段退出 0 和脱敏字段摘要，才允许进入删除阶段。任何失败都保持
现场原样并终止合并授权。

### 6.3 精确清理和目录重建

预检退出 0 后，按以下绑定值删除：

1. 完整 `container_id`；
2. 精确 `volume_name`；
3. 完整 `network_id`。

随后以精确项目标签分别要求容器、卷、网络查询为空。只有三类资源均为空时，才删除
字面目录并以 mode `0700` 重建。

Docker 删除不是事务。若删除阶段发生异常，立即停止后续命令，记录已经成功和未成功的
精确目标；不得尝试恢复、扩大清理或继续传输，必须取得新的书面授权后处理。

### 6.4 传输、prepare 和单次运行

1. `rsync -anvi --from0 --files-from=...` 的文件集合必须与 whitelist 字节一致，目录项
   只能是白名单文件的祖先。
2. 实际 rsync 使用完全相同的 selector；远端 regular-file 集合、零 symlink 和 source
   manifest 必须与本地一致。
3. `prepare.sh` 只生成远端专用 `.env` 和 secrets；不得读取其内容。
4. 在验收命令前创建本地 non-overwritable marker。
5. 仅执行一次 `acceptance-buyer-intent-smoke`。非零退出后保留当前资源和 evidence，
   不读取成功专用 evidence，也不重跑。

### 6.5 成功后证据门禁

只有验收退出 0 后才能先读取：

- `evidence-leak-scan.txt`，要求 `forbidden_matches=0`；
- `evidence-sha256.txt`，要求所有条目校验通过；
- `production-before.txt` 与 `production-after.txt`，要求字节相同。

三道门禁通过后，才读取本次新生成的脱敏 evidence，并验证 MySQL 8.4、0008/0009
迁移矩阵、API 矩阵、AutoMigrate false/true、full/race/vet、source manifest、资源保留
和生产快照字段边界。成功后保留专用项目资源和 evidence，不部署生产。

## 7. 错误语义

| 阶段 | 失败处理 |
| --- | --- |
| 本地 source gate | 不发起 SSH |
| 远端只读预检 | 零变更停止；原现场保留 |
| 精确清理 | 停止后续操作；记录精确部分结果；不补偿、不扩大操作 |
| dry-run 或 manifest | 不运行 prepare |
| prepare | 不运行验收命令 |
| 单次验收 | 授权消耗；保留资源；不重跑，不读取成功专用 evidence |
| 漏扫、hash 或生产快照门禁 | 不继续读取其余 evidence，不宣称通过 |

任何失败都不能推进 F-11 测试服务器状态，F-12 继续保持阻塞。

## 8. 验证与独立审阅

书面规格和实施计划在远端操作前必须通过独立只读审阅，至少验证：

1. container/network 的 list 值均是 `--no-trunc` 完整 ID；
2. container/network inspect 的完整 ID 与 list 值全等；
3. 名称、项目标签和停止状态使用独立字段比较；
4. volume 仍按名称验证，没有伪造卷 ID；
5. 删除参数只来自已验证的变量；
6. 只读预检与删除位于两个独立 SSH 阶段；
7. 源码固定为 `fc8b6a9` 且所有传输 blob 与该提交相同；
8. 单次 marker、失败停止和成功后 evidence 门禁完整；
9. 未扩大生产或 forbidden-resource 范围。

审阅出现 Critical/Important finding 时先修正文档并做 scoped re-review，审阅干净前不得
执行远端清理。

## 9. 回滚与保留

本修订只改变书面执行流程，不改变数据库或业务代码，因此文档回滚可 revert 对应提交。
远端 Docker 删除和目录删除不可事务回滚；这也是所有身份断言必须在独立零变更阶段
完成的原因。任何远端异常都保留当时状态，不根据猜测重建或继续运行。

成功验收后继续保留隔离 Compose 项目、卷、网络和脱敏 evidence，供 Task 4 审计追溯。
生产 0008/0009 仍未执行。

## 10. 完成标准

- 方案 A 和合并授权均有明确用户批准记录。
- 本规格与实施计划通过独立审阅，无未关闭 Critical/Important finding。
- 新的零变更预检退出 0，证明完整 ID、名称、标签和停止状态一致。
- 仅删除批准的一个容器、一个卷、一个网络和精确目录。
- `fc8b6a9` 的 120 文件 whitelist 与本地/远端 manifest 完全一致。
- 隔离 MySQL 8.4 验收只执行一次并退出 0。
- `forbidden_matches=0`、evidence hash 和生产快照相等性均通过。
- Task 4 记录完整脱敏证据后，才把 F-11 测试服务器状态改为通过并解除 F-12 前置阻塞。
- 生产未部署、未迁移、未修改数据、配置、上传文件或服务。

## 11. 审批记录

- 2026-07-28：用户批准 Task 3 资源身份验证完整修订设计方案 A。
- 2026-07-28：用户批准合并授权：允许将方案 A 写入书面规格和实施计划；独立审阅确认
  一致后无需再次请求书面规格批准，并允许在原精确远端范围内执行一次修订后的隔离验收。
