# 前端页面规划（frontend-pages）

## 默认假设
1. 前端为单应用（React + TypeScript + Vite），通过响应式布局同时支持 PC 和移动端。
2. 页面按角色与 scope 鉴权：未登录、平台管理员、商家主账号（`full/onboarding`）。
3. 商品状态采用：`DRAFT/ON_SHELF/LOCKED/OFF_SHELF/SOLD/CLOSED`。
4. 分类采用商户自有两级分类，页面通过当前商家的分类接口动态加载并支持商家自主管理。
5. `stock` 为当前可用库存；商品列表和详情提供“调整库存”入口，创建/编辑页保留基础库存输入。

## 1. 路由与信息架构

| 一级分区 | 页面 | 路由 |
| --- | --- | --- |
| 认证区 | 登录页 | `/login` |
| 认证区 | 商家注册页 | `/register` |
| 认证区 | 注册结果/待审核页 | `/register/status` |
| 管理员区 | 审核列表页 | `/admin/merchants/reviews` |
| 管理员区 | 审核详情页 | `/admin/merchants/reviews/:merchantId` |
| 管理员区 | 全局操作日志页 | `/admin/logs` |
| 商家区 | 仪表盘页 | `/merchant/dashboard` |
| 商家区 | 商品列表页 | `/merchant/products` |
| 商家区 | 商品分类页 | `/merchant/categories` |
| 商家区 | 新建商品页 | `/merchant/products/new` |
| 商家区 | 商品编辑页 | `/merchant/products/:productId/edit` |
| 商家区 | 商品详情页 | `/merchant/products/:productId` |
| 商家区 | 订单列表页 | `/merchant/orders` |
| 商家区 | 订单详情页 | `/merchant/orders/:orderId` |
| 商家区 | 账号设置页 | `/merchant/account` |
| 商家区 | 商家操作日志页 | `/merchant/logs` |

## 2. 页面级规格

### 2.1 认证区页面

#### 登录页（`/login`）
- 角色：未登录用户（管理员/商家共用入口）。
- 核心内容：账号、密码、登录类型切换（管理员/商家）、登录按钮。
- 关键动作 -> 接口：
  - 登录：`POST /api/v1/auth/login`（携带 `login_type`）。
- 核心状态展示：
  - `PENDING/REJECTED` 登录成功时，后端返回 `token_scope=onboarding`，前端直接跳转 `/register/status`。
  - `APPROVED` 登录成功时，后端返回 `token_scope=full`，前端跳转 `/merchant/dashboard`。
- PC/移动差异：
  - PC 为居中卡片。
  - 移动端按钮全宽、输入区放大触控面积。

#### 商家注册页（`/register`）
- 角色：未登录商家。
- 核心表单：企业名称、联系人、手机号、登录账号、密码、营业执照上传。
- 关键动作 -> 接口：
  - 获取上传凭证：`POST /api/v1/files/presign`
  - 提交注册：`POST /api/v1/auth/register`
- PC/移动差异：
  - PC 单页表单。
  - 移动端分步表单（基础信息 -> 资质上传 -> 确认）。

#### 注册状态页（`/register/status`）
- 角色：已注册未审核商家。
- 核心信息：审核状态、驳回原因、重新提交入口。
- 关键动作 -> 接口：
  - 获取状态：`GET /api/v1/merchant/profile`
  - 重新提交：`POST /api/v1/merchant/reapply`
- 接口职责说明：
  - `merchant/profile` 用于商家主体资料与审核状态，不承载账号安全设置。
- 状态展示规则：
  - `PENDING`：显示等待审核。
  - `REJECTED`：显示驳回原因与重提按钮。
- scope 规则：
  - 仅允许 `onboarding` 与 `full` 商家 token 访问。
  - `onboarding` token 在本页仅展示入驻相关动作，不展示商品/订单入口。

### 2.2 管理员区页面

#### 审核列表页（`/admin/merchants/reviews`）
- 角色：PlatformAdmin。
- 核心内容：审核表格/卡片、状态筛选、关键词、时间区间。
- 关键动作 -> 接口：
  - 拉取列表：`GET /api/v1/admin/merchants`
  - 点击进入详情：`GET /api/v1/admin/merchants/:id`
- 状态展示规则：
  - 徽标颜色：`PENDING=橙`、`APPROVED=绿`、`REJECTED=红`。
- PC/移动差异：
  - PC 使用表格。
  - 移动端使用卡片 + 筛选抽屉。

#### 审核详情页（`/admin/merchants/reviews/:merchantId`）
- 角色：PlatformAdmin。
- 核心内容：商家资料、证照图片、审核历史时间线。
- 关键动作 -> 接口：
  - 审核通过：`POST /api/v1/admin/merchants/:id/approve`
  - 审核驳回：`POST /api/v1/admin/merchants/:id/reject`
- 按钮规则：
  - 仅 `PENDING` 显示“通过/驳回”按钮。
  - `APPROVED/REJECTED` 隐藏审核动作按钮。
- PC/移动差异：
  - PC 双栏详情。
  - 移动端分组折叠 + 图片全屏预览。

#### 全局操作日志页（`/admin/logs`）
- 角色：PlatformAdmin。
- 关键动作 -> 接口：
  - 拉取日志：`GET /api/v1/admin/logs`
- 状态展示规则：
  - 展示对象类型、动作、前后状态、结果码。
- PC/移动差异：
  - PC 表格 + 侧边详情。
  - 移动端卡片 + 折叠详情。

### 2.3 商家区页面

#### 仪表盘页（`/merchant/dashboard`）
- 角色：MerchantOwner。
- 关键动作 -> 接口：
  - 拉取统计：`GET /api/v1/merchant/dashboard`
- 状态展示规则：
  - 商品统计需包含 `LOCKED` 数量。

#### 商品分类页（`/merchant/categories`）
- 角色：MerchantOwner。
- 核心内容：一级分类列表、二级分类列表、新增/编辑/启停/删除操作。
- 关键动作 -> 接口：
  - 查询一级/二级分类：`GET /api/v1/merchant/categories`
  - 新增分类：`POST /api/v1/merchant/categories`
  - 编辑分类：`PUT /api/v1/merchant/categories/:id`
  - 删除分类：`DELETE /api/v1/merchant/categories/:id`
- 交互规则：
  - 新增一级分类无需父级；新增二级分类必须选择当前商家的一级分类。
  - 分类名称必填，排序值为非负整数，状态支持 `ENABLED/DISABLED`。
  - 删除一级分类前需先删除其二级分类；删除已被商品引用的二级分类由后端拒绝并提示用户。
  - 分类变更后刷新商品新建/编辑页的分类选项。

#### 商品列表页（`/merchant/products`）
- 角色：MerchantOwner。
- 核心内容：商品列表、状态标签、筛选器、分页。
- 关键动作 -> 接口：
  - 列表：`GET /api/v1/merchant/products`
  - 上架：`POST /api/v1/merchant/products/:id/on-shelf`
  - 下架：`POST /api/v1/merchant/products/:id/off-shelf`
  - 关闭：`POST /api/v1/merchant/products/:id/close`
  - 调整库存：`POST /api/v1/merchant/products/:id/stock-adjustments`
- 状态展示规则：
  - `DRAFT`：显示“编辑/上架/调整库存/关闭”
  - `ON_SHELF`：显示“下架/创建订单/调整库存/关闭”
  - `OFF_SHELF`：显示“编辑/上架/调整库存/关闭”
  - `LOCKED`：显示“查看订单”，禁用编辑与上下架
  - `SOLD/CLOSED`：仅允许查看详情
- 库存调整弹窗：
  - 展示当前库存。
  - 支持 `补充库存`、`减少库存`、`线下售出` 三类调整。
  - 必填调整数量和调整原因。
  - `LOCKED/SOLD/CLOSED` 商品不展示可执行调整入口。
- PC/移动差异：
  - PC 操作列直接展示按钮。
  - 移动端卡片“更多”菜单。

#### 新建商品页（`/merchant/products/new`）
- 角色：MerchantOwner。
- 核心内容：标题、价格、成色、库存数量、描述、图片、分类选择。
- 关键动作 -> 接口：
  - 获取一级/二级分类：`GET /api/v1/merchant/categories`
  - 创建商品：`POST /api/v1/merchant/products`
  - 上传图片：`POST /api/v1/files/presign` + `POST /api/v1/files/confirm`
- 分类交互规则：
  - 先选一级再加载二级。
  - 必选二级分类后才允许提交。
  - 库存必须为大于 `0` 的整数。

#### 商品编辑页（`/merchant/products/:productId/edit`）
- 角色：MerchantOwner。
- 关键动作 -> 接口：
  - 获取详情：`GET /api/v1/merchant/products/:id`
  - 更新商品：`PUT /api/v1/merchant/products/:id`
  - 分类查询：`GET /api/v1/merchant/categories`
- 字段可编辑规则：
  - `DRAFT/OFF_SHELF`：可编辑标题、描述、分类、价格、成色、图片和库存。
  - `ON_SHELF`：仅描述、图片可编辑。
  - `LOCKED/SOLD/CLOSED`：禁止编辑。
- 库存规则：
  - 编辑页兼容保留基础库存输入。
  - 日常盘点、减少库存、线下售出扣减优先通过商品列表/详情的“调整库存”入口完成，以便后端记录调整流水和原因。
- 前后端约束：
  - 前端禁用不可编辑字段和提交按钮。
  - 后端二次校验字段变更，拒绝越权更新。

#### 商品详情页（`/merchant/products/:productId`）
- 角色：MerchantOwner。
- 关键动作 -> 接口：
  - 详情：`GET /api/v1/merchant/products/:id`
  - 上架/下架/关闭：对应商品状态接口
  - 创建订单：`POST /api/v1/merchant/orders`
  - 调整库存：`POST /api/v1/merchant/products/:id/stock-adjustments`
- 状态展示规则：
  - `LOCKED` 明确展示“占用中（待完成/待关闭订单）”。
  - 若 `active_order_id` 存在，展示“查看关联订单”按钮。
  - `DRAFT/ON_SHELF/OFF_SHELF` 展示“调整库存”按钮；`LOCKED/SOLD/CLOSED` 不展示可执行调整入口。

#### 订单列表页（`/merchant/orders`）
- 角色：MerchantOwner。
- 关键动作 -> 接口：
  - 列表：`GET /api/v1/merchant/orders`
  - 详情：`GET /api/v1/merchant/orders/:id`
- 状态展示规则：
  - `CREATED`：可进入详情执行完成/关闭。
  - `COMPLETED/CLOSED`：只读。

#### 订单详情页（`/merchant/orders/:orderId`）
- 角色：MerchantOwner。
- 关键动作 -> 接口：
  - 完成订单：`POST /api/v1/merchant/orders/:id/complete`
  - 关闭订单：`POST /api/v1/merchant/orders/:id/close`
- 按钮规则：
  - 仅 `CREATED` 显示完成/关闭按钮。
  - 提交中按钮置灰，防重复点击。
- 业务提示：
  - 完成后商品转 `SOLD`。
  - 关闭后商品转 `OFF_SHELF`。

#### 账号设置页（`/merchant/account`）
- 角色：MerchantOwner。
- 关键动作 -> 接口：
  - 获取信息：`GET /api/v1/merchant/account`
  - 修改密码：`PUT /api/v1/merchant/account/password`
- 接口职责说明：
  - `merchant/account` 返回当前登录账号信息与安全设置（如密码更新时间）。
  - 与 `merchant/profile` 分工明确，避免主体资料与账号安全模型混用。

#### 商家操作日志页（`/merchant/logs`）
- 角色：MerchantOwner。
- 关键动作 -> 接口：
  - 拉取日志：`GET /api/v1/merchant/logs`
- 状态展示规则：
  - 重点展示商品/订单状态前后变化。

## 3. 核心状态展示规范

### 3.1 审核状态
1. `PENDING`：待处理（橙色标签）
2. `APPROVED`：已通过（绿色标签）
3. `REJECTED`：已驳回（红色标签 + 原因）

### 3.2 商品状态
1. `DRAFT`：草稿
2. `ON_SHELF`：在售
3. `LOCKED`：占用中
4. `OFF_SHELF`：已下架
5. `SOLD`：已成交
6. `CLOSED`：已关闭

### 3.3 订单状态
1. `CREATED`：待处理
2. `COMPLETED`：已完成
3. `CLOSED`：已关闭

## 4. 跨页面通用规范
### 4.1 接口职责边界
1. `merchant/profile`：商家主体资料 + 审核状态。
2. `merchant/account`：当前登录账号资料 + 安全设置。
3. 前端按页面职责调用接口，禁止“在账号设置页读取 profile 代替 account”。

### 4.2 通用行为
1. 权限守卫：
   - 未登录访问受限页跳转登录。
   - `token_scope=onboarding` 登录后统一跳转 `/register/status`。
   - `token_scope=onboarding` 访问商品分类/商品/订单/仪表盘/账号设置/商家日志路由时强制回跳 `/register/status`。
   - 管理员与商家路由互斥访问。
2. 错误处理：
   - 401 统一触发登录失效逻辑。
   - 403 展示无权限页。
   - `10005`（状态非法）提示“当前状态下不可执行该操作”。
   - `10010`（并发冲突）提示“商品已被其他订单占用”。
3. 列表体验：
   - 所有列表支持分页、状态筛选、关键词筛选。
   - 筛选条件与 URL query 同步，支持刷新恢复。
4. 上传体验：
   - 上传中、成功、失败状态可见。
   - 支持图片压缩预览（前端可选）。
   - `onboarding` token 仅允许资质上传（`MERCHANT_LICENSE`），不允许商品图片上传。

## 5. 响应式适配策略
1. 断点建议：
   - `>=1200px`：桌面宽屏
   - `768px~1199px`：平板/小桌面
   - `<768px`：手机
2. 布局规则：
   - PC：左侧导航 + 顶部栏 + 主内容区。
   - Mobile：底部导航（商家区）或抽屉导航（管理员区）。
3. 高密度表格降级：
   - 表格列过多时在移动端转换为卡片并保留关键字段与状态标签。
4. 关键操作可达性：
   - 手机上架/下架/完成订单/关闭订单保持单手可达。

## 6. 前端开发检查清单
1. 页面路由、鉴权守卫、角色路由隔离已完成。
2. 审核页、商品页、订单页均已建立“动作 -> 接口”映射。
3. 商品编辑字段限制已按状态实现并联调后端校验。
4. 分类管理页已接入查询、新增、编辑、启停和删除接口；商品页分类级联选择读取当前商家分类。
5. 移动端关键页面通过真机检查（iOS Safari / Android Chrome）。
