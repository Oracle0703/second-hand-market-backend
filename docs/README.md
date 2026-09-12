# 文档索引

更新时间：2026-09-12

## 使用约定

1. 当前运行行为以代码、`backend/internal/app/server.go` 路由和 `backend/migrations/` 显式迁移为最终依据。
2. 下表“当前文档”描述当前版本；功能变化必须同步更新对应文档。
3. `decisions/` 和 `delivery/` 是历史设计与交付记录，用于解释决策，不作为未完成任务清单。
4. 历史交付记录只用于追溯；是否已实现以当前文档、代码和实际环境证据为准。
5. 生产部署、数据库迁移和平台审核不能从代码合并状态推断，统一以发布清单和实际环境证据为准。
6. 独立 review、review response、issue 汇总和测试评审文件已于 2026-09-12 删除；有效结论已并入当前文档、设计记录或交付报告。

## 当前文档

| 文档 | 用途 |
| --- | --- |
| [项目概览](project-overview.md) | 当前范围、角色、业务闭环与边界 |
| [产品与系统规格](specs.md) | 当前功能规则、状态机和验收口径 |
| [数据模型](data-model.md) | 当前核心实体、约束和迁移策略 |
| [页面与路由](frontend-pages.md) | 管理端和买家小程序页面能力 |
| [目录结构](dir-structure.md) | 仓库真实结构与代码归属 |
| [后端 API 清单](backend-api-checklist.md) | 当前路由、权限和接口规则 |
| [买家 API 清单](miniapp-buyer-api-checklist.md) | 买家域接口与商家意向接口 |
| [买家数据模型](miniapp-buyer-data-model.md) | 买家、设备、收藏、历史和意向模型 |
| [开发里程碑](dev-milestones.md) | 已交付能力和后续里程碑 |
| [全局发布清单](release-readiness.md) | 发布门禁、顺序和环境风险 |
| [小程序发布清单](miniapp-release-readiness.md) | 微信/抖音构建、真机和平台验收 |
| [验收清单](acceptance-checklist.md) | 管理端人工验收 |
| [买家小程序验收清单](miniapp-buyer-acceptance-checklist.md) | 小程序人工验收 |

## 运维与排障

| 文档 | 用途 |
| --- | --- |
| [抖音构建与隐私授权排障](operations/douyin-build-troubleshooting.md) | `NODE_ENV`、loading、电话与定位隐私授权 |
| [小程序真机调试](operations/real-device-debug.md) | 真机网络与平台联调 |
| [微信登录联调](operations/wechat-login-integration.md) | 微信登录配置与验证 |
| [远程开发数据库](operations/remote-development-database.md) | SSH 隧道和数据库身份门禁 |

## 历史设计与交付记录

- `decisions/`：已经实施或已定稿的专题设计与关键决策。
- `delivery/2026-08-09-product-sold-out-state/`：商品售罄状态改造的交付证据。
- [买家小程序实现基线与后续计划](miniapp-buyer-implementation-plan.md)：由最初方案更新为当前实现基线。
- [架构演进方案](architecture-evolution-plan-2026-07-24.md)：按 2026-09-12 代码状态更新的后续演进路线。

## 维护要求

- 新增或删除路由：同步 `backend-api-checklist.md` 或 `miniapp-buyer-api-checklist.md`。
- 修改状态机或库存规则：同步 `specs.md`、`data-model.md` 和相关验收清单。
- 修改页面能力：同步 `frontend-pages.md`。
- 修改部署配置、迁移或平台能力：同步两个 release-readiness 文档。
- 新专题设计必须标明代码实现状态与生产发布状态，不再新增独立 review 文档。
- 实施步骤不单独长期保存；完成后的结论应回填当前文档、设计决策或交付报告。
