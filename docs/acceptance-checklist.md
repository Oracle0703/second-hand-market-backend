# 收口阶段验收清单（PC / Mobile）

## 1. 预检
- [ ] 后端可启动：`cd backend && ... go run ./cmd/server`
- [ ] 前端可启动：`cd frontend && npm run dev`
- [ ] 使用已安全初始化的管理员账号登录；仓库中不存在可用的固定管理员口令。
- [ ] 管理员可在“安全设置”校验当前密码后改密，成功后旧 access/refresh token 失效并返回管理员登录页。

## 2. restricted login（商家入驻阶段）
### PC
- [ ] 新商家注册成功后可用商家账号登录。
- [ ] 登录成功返回受限制态（前端跳转 `/register/status`）。
- [ ] 注册状态页可查看 `PENDING/REJECTED` 与驳回原因。
- [ ] `REJECTED` 可点击“重新提交审核”成功。
- [ ] 受限制态访问商品/订单/仪表盘/日志/账号设置被拒绝（提示权限受限）。

### Mobile（浏览器宽度 < 768）
- [ ] 受限制态登录后仍跳转 `/register/status`。
- [ ] 注册状态页信息无横向溢出，按钮可点击。

## 3. 审核与权限切换
### PC
- [ ] 管理员审核列表可看到新商家。
- [ ] 审核详情可执行通过/驳回。
- [ ] 商家审核通过后重新登录，进入 `/merchant/dashboard`。

### Mobile
- [ ] 管理员审核详情页面可正常滚动查看详情与审核历史。

## 4. 商品主链路
### PC
- [ ] 商品创建页：一级分类 -> 二级分类级联可用。
- [ ] 商品创建成功，进入商品详情页。
- [ ] 商品状态切换：`DRAFT -> ON_SHELF -> OFF_SHELF -> ON_SHELF`。
- [ ] 商品关闭后不可恢复。
- [ ] 编辑页在 `ON_SHELF` 仅允许描述/图片更新。

### Mobile
- [ ] 商品列表与详情在手机宽度可正常查看与操作。
- [ ] 创建/编辑表单无关键控件遮挡。

## 5. 订单主链路
### PC
- [ ] `ON_SHELF` 商品可填写数量和单件成交价创建订单，页面自动计算整单总价。
- [ ] 创建订单只增加 `reserved_stock`，商品不进入 `LOCKED`；同一商品可有多笔 active 订单。
- [ ] 超过 `available_stock=stock-reserved_stock` 的订单返回冲突（`10010`）。
- [ ] 完成订单按数量同时减少总库存和预占库存；仅总库存归零时商品变为 `SOLD`。
- [ ] 关闭订单按数量释放预占，商品保持当前上架/下架状态。
- [ ] 有预占时不能永久关闭商品，也不能把总库存改到预占量以下。

### Mobile
- [ ] 订单列表、订单详情可正常访问。
- [ ] 完成/关闭按钮可点击且状态刷新正确。

## 6. 非主链路页面
- [ ] 管理员审核详情页可看到审核历史。
- [ ] 商家日志页可按 `action/resource_type` 筛选。
- [ ] 账号设置页可查看账号信息并修改密码。

## 7. 回归命令
- [ ] 后端测试：`cd backend && ... go test ./...`
- [ ] 前端测试：`cd frontend && npm run test`
- [ ] 前端构建：`cd frontend && npm run build`
- [ ] 冒烟回归：通过 secret 环境注入 `SMOKE_ADMIN_USERNAME` / `SMOKE_ADMIN_PASSWORD` 后执行 `API_BASE_URL=http://localhost:8080/api/v1 node scripts/smoke-flow.mjs`。

## 8. 发布前人工核对补充
- [ ] 在 Chrome DevTools 切换到 `<768px` 宽度，逐项核对 restricted login / 商品 / 订单关键页面无横向溢出。
- [ ] 在移动端关键操作按钮（上架/下架/完成订单/关闭订单）上执行一次点击回归，确认状态刷新正确。
