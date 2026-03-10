# 发布前问题清单（release-readiness）

更新时间：2026-03-10

## 1. 当前版本已完成范围
- 工程骨架、数据模型与迁移、管理员初始化脚本。
- restricted login 收口（`onboarding/full` scope 边界一致）。
- 商品 P1 主链路：创建、编辑、上架、下架、关闭、详情、列表。
- 订单 P1 主链路：创建、详情、完成、关闭，与商品状态联动。
- 非主链路页面：管理员审核详情、商家日志、账号设置。
- 并发与事务基础保障：单商品单活动订单、事务一致性测试。

## 2. 已验证通过项
- 自动化回归（2026-03-10）：
  - `cd backend && ... go test ./...`：通过
  - `cd frontend && npm run test`：通过
  - `cd frontend && npm run build`：通过
  - `API_BASE_URL=http://localhost:8080/api/v1 node scripts/smoke-flow.mjs`：通过
- 后端关键回归测试覆盖：
  - `TestRestrictedLoginScope`
  - `TestConcurrentOrderCreateSingleActive`
  - `TestOrderProductTransactionConsistency`
  - `TestProductLifecycleStatusTransitions`
  - `TestProductEditRulesByStatus`
- smoke 覆盖：
  - restricted login（`PENDING -> onboarding` + 受限接口拒绝）
  - 管理员审核通过后 `full` 权限登录
  - 商品创建/上架
  - 订单创建/完成（商品 `LOCKED -> SOLD`）
  - 订单创建/关闭（商品 `LOCKED -> OFF_SHELF`）
- 文档一致性核对：
  - restricted login 规则在 `project-overview/specs/frontend-pages/backend-api-checklist` 已一致。
  - 错误码 `10001~10011/20001` 前后端已对齐（前端提示文案已按受限登录语义修正）。

## 3. 仍存在的风险
- Mobile 端当前以手工清单核对为主，尚无自动化 E2E。
- 前端测试存在 React Router v7 future flag warning（不影响当前功能）。
- 冒烟脚本依赖本地端口与初始化管理员账号可用。

## 4. 明确不阻塞发布的问题
- React Router future flag warning（仅升级提示，无行为回归）。
- `README.en.md` 仍是模板内容（不影响交付与运行）。
- 当前未引入完整 UI 端到端自动化（已有手工清单 + API/集成回归兜底）。

## 5. 阻塞发布的问题（如果有）
- 本轮未发现新的阻塞缺陷。

## 6. 建议发布顺序 / 上线前检查项
1. 执行后端回归：`cd backend && ... go test ./...`。
2. 执行前端回归：`cd frontend && npm run test && npm run build`。
3. 启动后端并执行冒烟脚本：`API_BASE_URL=http://localhost:8080/api/v1 node scripts/smoke-flow.mjs`。
4. 按 [收口验收清单](./acceptance-checklist.md) 做 PC + Mobile 最终人工检查。
5. 上线顺序建议：
   - 先发布后端
   - 再发布前端
   - 发布后立刻执行一次 smoke 回归
6. 回滚准备：
   - 保留上一版前后端构建产物
   - 若上线后出现 `10010/10005` 异常激增，优先回滚后端并保留日志样本。
