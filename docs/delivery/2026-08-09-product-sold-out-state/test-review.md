# 测试与审查

## 测试矩阵

| 验收标准 | 验证方式 | 结果 | 证据 |
|---|---|---|---|
| SC-1 商品移除 `CLOSED` | 后端商品路由/状态/统计测试，商家页面渲染测试 | 通过 | 商品 close 路由已移除；商品统计为五状态；订单 `CLOSED` 保留 |
| SC-2 售罄库存与预占归零 | 商品库存调整及订单完成 Go 测试 | 通过 | 快捷售罄、最后一件完成、实际流水数量均有回归断言 |
| SC-3 售罄补货后显式上架 | 状态流、库存调整、商家弹窗测试 | 通过 | `SOLD + INCREASE -> OFF_SHELF`，不会自动上架 |
| SC-4 零库存或活动订单限制 | 商品上架、锁定商品和快捷售罄 Go 测试 | 通过 | 非法路径返回 `10005` |
| SC-5 商家端售罄操作 | 前端全量 Vitest 与生产构建 | 通过 | 10 文件 29/29；build 退出码 0 |
| SC-6 小程序不可购买入口 | 小程序全量 Vitest 与微信构建 | 通过 | 16 文件 157/157；`build:weapp` 退出码 0 |
| SC-7 历史状态迁移 | 迁移目录测试、SQL 与 SHA256 检查 | 通过 | 0007 已注册；历史商品 `CLOSED` 和矛盾 `SOLD` 转为 `OFF_SHELF` |

## 规格符合性审查

| 结论 | 证据 | 状态 |
|---|---|---|
| SC-1 至 SC-7 均有对应实现和测试 | 主设计、详细计划、后端/前端/小程序 diff | 已完成 |
| 未引入 `SOLD_OUT`、商品 `SOLD` 直接删除、多活动订单或额外安全治理 | 状态常量、路由、迁移和 UI 检查 | 符合范围 |
| 未恢复已下线的意向页面；仅限制现有电话 CTA | 小程序详情、收藏页与页面渲染测试 | 符合范围 |
| 全分支审查未发现未处理的 Critical/Important | 独立任务审查、全分支审查及两轮范围复审 | 已完成 |

## 代码质量审查

| 级别 | 发现 | 处理结果 |
|---|---|---|
| Minor | 快捷售罄测试最初未按 `movement_id` 精确核对落库流水 | 已补充实际 `quantity/stock_after/status_after` 断言 |
| Minor | 商家页面和小程序真实页面接线证明力不足 | 已新增 List/Detail/Dashboard、详情/收藏真实渲染测试 |
| 补正 | 前端页面测试 mock 的 `request` 参数类型导致 `TS2554` | 已修正测试签名并通过生产 build |
| 补正 | Miniapp Vitest 未同步生产 `@ -> src` 别名 | 已仅在测试配置补齐别名，并通过全量测试与生产构建 |
| 最终结论 | 无未处理 Critical、Important 或 Minor 功能问题 | 已完成 |

## 验证日志

| 命令 / 检查 | 结果 | 备注 |
|---|---|---|
| `go test -p 1 ./internal/app ./internal/stateflow ./tests ./scripts/migrate -count=1` | 未通过，平台门槛 | `internal/app`、`internal/stateflow`、`scripts/migrate` 通过；`tests` 仅因既有小程序验收合同要求 Linux 而失败 |
| `go test -p 1 ./internal/app ./internal/stateflow ./tests ./scripts/migrate -run 'Test.*(Product|Dashboard|MigrationCatalog|Order)' -count=1` | 退出码 0 | 本功能涉及的四个 Go 包全部通过 |
| `frontend: npm test` | 退出码 0 | 10 个测试文件，29/29 通过 |
| `frontend: npm run build` | 退出码 0 | `tsc -b` 与 Vite build 通过；有既有循环 chunk 和大 chunk 警告 |
| `miniapp: npm test` | 退出码 0 | 16 个测试文件，157/157 通过 |
| `miniapp: npm run build:weapp` | 退出码 0 | Taro/webpack 编译成功；有包体积、异步分包和 Browserslist 警告 |
| `rg -n "ProductClosed|ProductStatus\\.CLOSED|closeProduct|/products/:id/close" backend/internal frontend/src miniapp/src` | 0 个匹配 | 商品生产代码和客户端源码无 `CLOSED` 入口残留；`rg` 因无匹配返回 1 |
| `rg -n "OrderClosed|IntentClosed|order_stats.*closed" backend/internal frontend/src miniapp/src` | 匹配符合预期 | 订单与意向 `CLOSED` 及订单关闭统计仍保留 |
| `git diff --check` | 退出码 0 | 代码与交付文档更新后复跑 |

## 环境记录

| 工具 | 当前环境 | 仓库目标 | 判定 |
|---|---|---|---|
| Node.js | `v24.13.0` | `22.22.2` | 版本偏差；本次测试和构建实际通过 |
| npm | `11.6.2` | `10.9.7` | 版本偏差；本次测试和构建实际通过 |
| Go | `go1.23.8 windows/amd64` | `go.mod 1.22.0` | 向后兼容验证通过，但不是精确工具链 |

## 残余风险

| 风险 / 未验证项 | 影响 | 发布前动作 |
|---|---|---|
| Linux-only 小程序验收合同不能在当前 Windows 环境执行 | 该独立合同尚无本机通过证据，不属于本次商品状态功能失败 | 在 WSL2 或 Linux 主机运行未过滤的后端完整命令 |
| 0007 未在真实发布数据库执行 | 当前只证明 SQL 内容、目录注册和校验摘要正确 | 发布后端前执行迁移，并确认两条异常查询计数均为 0 |
| Node/npm 与仓库锁定版本不一致 | 当前通过不等于精确工具链验收 | 发布流水线使用 Node 22.22.2 / npm 10.9.7 复验 |
| 构建存在既有分包和体积警告 | 不阻塞本次功能，但仍有性能维护成本 | 后续单独治理，不纳入本次最小实现 |
