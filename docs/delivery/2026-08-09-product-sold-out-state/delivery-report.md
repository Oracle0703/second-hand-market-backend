# 交付报告

## 摘要

| 字段 | 内容 |
|---|---|
| 结果 | 已完成商品售罄状态最小功能实现 |
| 日期 | 2026-08-10 |
| 分支 | `hotfix/hy/0000_product_sale_status` |
| 基线 | `4c9617740bfe5e86de656952c92a34131ed607b4` |
| 状态 | 工作区可供审阅；发布前仍需 Linux-only 合同和真实数据库迁移 |

## 已交付变更

| 区域 | 变更 |
|---|---|
| 设计 | 合并两份评审，明确五种商品状态、售罄归零、补货后下架及显式重新上架 |
| 后端 | 移除商品 `CLOSED`；保留订单/意向 `CLOSED`；实现订单预占、完成扣减、关闭释放及末件售罄 |
| 库存 | `MARK_SOLD + all_remaining=true` 在行锁内归零并记录实际流水；`SOLD` 仅允许补货并转 `OFF_SHELF` |
| 上架 | 要求图片、可销售库存大于 0 且无活动订单 |
| 数据 | 新增并注册 `0007_product_sold_out_state.up.sql`，归一化历史 `CLOSED` 和矛盾 `SOLD` 数据 |
| 商家端 | 商品改为五状态，`SOLD` 显示“售罄”，提供快捷售罄和售罄补货，移除商品关闭入口与统计 |
| 小程序 | 商品卡、详情和收藏显示中文状态；非 `ON_SHELF` 隐藏“我想要”；未恢复隐藏意向页 |
| 测试 | 增加后端库存/订单/迁移回归、商家真实页面渲染和小程序真实页面渲染测试 |

## 验证证据

| 检查 | 结果 |
|---|---|
| 后端功能相关四包 | 退出码 0；`internal/app`、`internal/stateflow`、`tests`、`scripts/migrate` 均通过 |
| 后端未过滤四包 | 仅被既有 Linux-only 小程序验收合同阻塞；其余三个包通过 |
| 商家端测试 | 10 文件，29/29 通过 |
| 商家端生产构建 | 退出码 0 |
| 小程序测试 | 16 文件，157/157 通过 |
| 微信小程序构建 | 退出码 0 |
| 0007 SHA256 | `1ddaea62f22d198c6659bfadc0dfbdc071a35606f38bd9a926345ee3941bff58` |
| 商品 `CLOSED` 源码残留检查 | 0 个匹配；订单/意向 `CLOSED` 保留检查符合预期 |
| 差异格式检查 | `git diff --check` 退出码 0 |

## 重要文件

| 文件 | 用途 |
|---|---|
| `docs/superpowers/specs/2026-08-09-product-sold-out-state-design.md` | 最终业务设计与验收标准 |
| `backend/internal/app/product_stock_adjustment_handlers.go` | 快捷售罄、部分售出和售罄补货规则 |
| `backend/internal/app/order_handlers.go` | 订单预占、完成扣减和关闭释放 |
| `backend/migrations/0007_product_sold_out_state.up.sql` | 历史商品状态归一化 |
| `frontend/src/pages/merchant/products/components/StockAdjustmentModal.tsx` | 商家快捷售罄与售罄补货入口 |
| `miniapp/src/utils/product-status.ts` | 小程序商品状态文案和联系购买判断 |
| `docs/delivery/2026-08-09-product-sold-out-state/test-review.md` | 完整测试、审查与环境证据 |

## 已知限制

| 限制 / 跳过的检查 | 原因 | 影响 |
|---|---|---|
| Linux-only 小程序验收合同未通过当前主机验证 | 合同校验 POSIX 权限、符号链接和原子目录所有权，明确拒绝 Windows | 发布前必须在 WSL2 或 Linux 主机复验 |
| 0007 未对真实发布数据库执行 | 本次仅修改仓库，不进行部署或数据库运维 | 发布时必须先迁移并验证异常记录数为 0 |
| Node/npm 非仓库锁定版本 | 当前环境为 Node 24.13.0 / npm 11.6.2 | 当前测试和构建通过，但仍需精确工具链复验 |
| 构建警告未治理 | 属于既有分包和包体积问题，超出最小功能范围 | 不影响本次构建退出码和功能验收 |

## Git 交付边界

| 操作 | 结论 |
|---|---|
| 本地提交 | 用户已于 2026-08-10 明确授权；最终提交信息以 Git 日志为准 |
| 推送 | 未获授权，不执行 |
| Pull Request | 未获授权，不创建 |
| 缓存清理 | 未执行；保留现有 Go 测试缓存目录 |

## 后续动作

| 优先级 | 动作 | 负责人 |
|---|---|---|
| 发布前 | 在 Node 22.22.2 / npm 10.9.7 和 Linux 环境复验完整合同 | 发布/验收人员 |
| 发布前 | 先执行 0007，并确认商品 `CLOSED` 与 `SOLD AND stock > 0` 计数为 0 | 数据库运维人员 |
| 提交后 | 由用户决定是否推送或创建 PR | 用户 |
