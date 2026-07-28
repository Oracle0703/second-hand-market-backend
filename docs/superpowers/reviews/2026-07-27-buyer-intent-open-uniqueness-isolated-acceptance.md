# F-11 买家意向 open 唯一性隔离验收报告

**验收日期：** 2026-07-28

**结论：** 通过隔离 MySQL 8.4 测试服务器审核；生产 `0008`/`0009` 未执行，未部署，生产数据、配置、上传和服务未修改。

## 1. 验收边界

- 源码提交：`6f84cc68c2f6dd870e2e6943f240d9b8589d6396`。
- 远端目录：`/home/yu/services/secondhand-buyer-intent-acceptance-20260727`。
- Compose 项目：`secondhand-buyer-intent-acceptance`。
- committed whitelist：120 个普通文件，零符号链接。
- source manifest SHA-256：
  `1e64fc5ff65f59be44544037eae5222f858ef9355549781b6c57273e424649ab`。
- 接受的修复序列从 `ee1a964fb921ef35ceca4a1c2514b1027e35f5fd` 到
  `6f84cc68c2f6dd870e2e6943f240d9b8589d6396`（含两端）；它补充既有 F-11 实现范围
  `77771d379ce260b548b54c45882c8173747467fe..0f2cf7b5db9bbe7f00c18490dc523b09709d8467`。

执行命令：

```sh
ssh aliyun-server \
  'cd /home/yu/services/secondhand-buyer-intent-acceptance-20260727 &&
   env COMPOSE_PROJECT_NAME=secondhand-buyer-intent-acceptance \
   BUYER_INTENT_ACCEPTANCE_CONFIRM=I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_BUYER_INTENT_DATA \
   ACCEPTANCE_DB_ENGINE=mysql8.4 \
   make acceptance-buyer-intent-smoke'
```

命令退出 0。成功 evidence 位于：

```text
/home/yu/services/secondhand-buyer-intent-acceptance-20260727/deploy/acceptance/evidence/buyer-intent-open-uniqueness
```

## 2. 验收矩阵

- MySQL 版本门禁：`8.4.8`。
- `0008`：fail-fast 完整迁移链的 preflight/up/postflight 在所有后续 `0009` 场景前通过；此前的 HAVING 1054 阻断未复现。
- `0009` 成功态：legacy、marker-only、both-keys、final-rerun 均通过；final-rerun 连续执行两次通过，行摘要前后相同。
- `0009` 拒绝态：五种非法 status/is_open 组合、duplicate-open、drifted-marker、drifted-key、unknown-partial 均以 `ERROR 1644 (45000)` fail closed，行摘要前后相同。
- 最终/API schema：`open_marker` 为 nullable stored generated `tinyint`；唯一索引为
  `uk_buyer_intent_open(buyer_id,product_id,open_marker)`，无旧索引。
- `AUTO_MIGRATE=false`：通过；三次关闭历史和一个 open 行保留，并发创建为一个
  `200/0` 和一个 `409/10010`。
- `AUTO_MIGRATE=true`：通过同一业务、并发与 schema 矩阵。
- `go test ./... -count=1`：全部包通过。
- `go test -race ./... -count=1`：全部包通过；`backend/tests` 为 `172.894s`。
- `go vet ./...`：通过。

## 3. Evidence 门禁

- 独立漏扫和 retained marker 均为 `forbidden_matches=0`。
- `evidence-sha256.txt` 的 26 个条目全部通过 `sha256sum -c`。
- `production-before.txt` 与 `production-after.txt` 字节相同，SHA-256 均为
  `ee185083b43c7f2eef2dd462388010b966485ef355ccc6600b17bff309c33235`。
- 生产快照只包含三个固定容器的名称、完整 ID、状态和重启计数；未读取生产 SQL、日志、环境变量、挂载、配置、服务或数据。

已验证的 evidence SHA-256：

```text
df6d2f3ac43ce8866525fd40752d3c2a748e6926e30f938778800c55ef10df24  api-matrix-schema.txt
e2bac80252506020cb51a7f873d9a083e94bd49373916d413024bec6634287ef  backend-race.txt
53f04a1346c30ecfb7f9628bb6f4b8d59779cb87890169dc4808681d35bdba9a  backend-tests.txt
1fa36cacf618aa251d7bd2d9a407af1a0fc88cb40ff5a0a4ed284d038322cf3b  both-keys.txt
90f466e62d744fd2a30d56e95d0d53519b913d149407f5041330162055e7c042  drifted-key.txt
1d1d334847e016cfce35a6eb50c429ce67627811dcd0426a152686871d1e0af3  drifted-marker.txt
dc858450efee55a9352e2e129dbc361baf67f5922cf14aa577873a949f68ac6c  duplicate-open.txt
64ff0f5118201932260226197711b0fdf9d6f59e9f0ab8683d9a1c4580f44133  evidence-leak-scan.txt
52363c34a84600002c1d2f9de4e542844291b465639b6f5087214396e1190558  final-rerun.txt
df6d2f3ac43ce8866525fd40752d3c2a748e6926e30f938778800c55ef10df24  final-schema.txt
4ba67601d550a1c12692a49d374418843f5f9511fdad3e5457bb12d469f8b647  go-vet.txt
802e9c4f6c758ba710772802493e277c2b262f3a2f0afb782660bb2908359cf5  invalid-state-bogus-false.txt
791462f3df307fc0cf23548c4dac5d582fed78d107bd95c49e56af72e60a3901  invalid-state-bogus-true.txt
577e0863170feb2bcf012a408472b9cd3ac47bcccfbeb766e16c4bd8dd3bbd14  invalid-state-closed-true.txt
69007e3149ab61ab6c3d1dfeddf8de5b252b7ef2f85a058fd47f8e4732eef8c0  invalid-state-contacted-false.txt
1154af4fbf1eaf250974a23a15568b9b626ba7ecc27d5e70536477e109ad863e  invalid-state-new-false.txt
a2bbd19112e23e4de8ce083239d1d97ce781a1d9d16a42958dcde745eea51b98  legacy.txt
53116cab097c555bca82da5bac788bd55465d9288aa3750f14aa62875de32c6f  marker-only.txt
d4b68b5b7ae95f68c68a07f811ba32dae7a36e94bf01512b229117fb949988c7  mysql-auto-migrate-false.txt
39b611e7d492b46bf8203bbf020cc49774fc2fe8df7c7665b4459e089450b7f1  mysql-auto-migrate-true.txt
98dc43f2187419a8b8de34b84f71945cc2ad28b052784cdd6bea7b28fe26ad51  mysql-version.txt
ee185083b43c7f2eef2dd462388010b966485ef355ccc6600b17bff309c33235  production-after.txt
ee185083b43c7f2eef2dd462388010b966485ef355ccc6600b17bff309c33235  production-before.txt
4ddd6d27e86432da29e89acdc298f757d8c761c6f5f8bf152d35d817499b8ac0  resource-retention.txt
1e64fc5ff65f59be44544037eae5222f858ef9355549781b6c57273e424649ab  source-sha256.txt
491669ed2315d9a509a1daf796c089122917c1648e75b136a59f132077f8e4b3  unknown-partial.txt
```

## 4. 保留状态与后续边界

验收后无运行中的 smoke/make 进程。隔离 MySQL 容器
`secondhand-buyer-intent-acceptance-mysql-1` 以 exit 0 停止；专用
`mysql-data`、`uploads` 卷和 `acceptance` 网络连同脱敏 evidence 保留供审计。

F-11 测试服务器审核通过，F-12 的 F-11 前置条件已满足；这不表示 F-12 已实现或已验收。
生产 `0008`/`0009`、应用部署和生产写验证仍需独立授权，不能把本报告解释为生产关闭。

## 5. 文档审阅修复

以 `c72643b` 为审阅基线的第一轮文档审阅发现，`docs/release-readiness.md`
的生产维护窗顺序在 0008 postflight 后直接进入部署，与同一文档要求生产 `0009`
必须在独立授权维护窗执行的当前状态矛盾。现已在部署前明确补入：

```text
0009 buyer intent open uniqueness preflight
-> 0009 buyer intent open uniqueness up migration exactly once
-> 0009 buyer intent open uniqueness postflight
```

本轮只修正文档，没有执行任何生产命令。聚焦顺序检查和 `git diff --check` 均通过。
