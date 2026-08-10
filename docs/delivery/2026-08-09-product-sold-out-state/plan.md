# 实施计划

| 字段 | 内容 |
|---|---|
| 需求文档 | `requirements.md` |
| 设计规格 | `specs.md` |
| 分支 | `hotfix/hy/0000_product_sale_status` |
| 负责人 | 控制器与按任务串行派发的 RD 智能体 |

详细可执行步骤见 `docs/superpowers/plans/2026-08-09-product-sold-out-state.md`。

## 任务清单

| 状态 | 任务 | 负责人 / 智能体 | 文件 / 模块范围 | 验证方式 |
|---|---|---|---|---|
| 已完成 | 后端状态与库存领域规则（Task 1） | 后端 RD | `backend/internal/stateflow`、库存调整 DTO/handler/tests | 定向 Go 测试与任务审查通过 |
| 已完成 | 后端商品入口、统计和迁移（Task 2） | 后端 RD | 商品 API、状态机、Dashboard、迁移目录 | 定向 Go 测试与迁移目录测试通过 |
| 已完成 | 商家后台状态、库存入口和 Dashboard | 前端 RD | `frontend/` | 6 项 Vitest 与 build 通过 |
| 已完成 | 小程序状态文案与电话 CTA 限制 | 小程序 RD | `miniapp/` | 16 项 Vitest、`build:weapp` 与独立任务审查通过 |
| 已完成 | 规格及代码质量审查 | QA | 全部改动 | 全分支审查及测试补强范围复审通过 |

## 审查关卡

| 关卡 | 必需证据 | 状态 |
|---|---|---|
| 规格符合性审查 | SC-1 至 SC-7 均映射到实现和验证 | 已完成 |
| 代码质量审查 | 无未处理高/中优先级功能缺陷 | 已完成 |
| 最终验证 | 后端功能测试、前端测试/build、小程序测试/build；Linux-only 合同单独记录 | 已完成 |
