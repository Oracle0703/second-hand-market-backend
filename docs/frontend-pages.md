# 前端页面规划（frontend-pages）

## 默认假设
1. 前端为单应用（React + TypeScript + Vite），通过响应式布局同时支持 PC 和移动端。
2. 页面按角色鉴权：未登录、平台管理员、商家主账号。
3. UI 组件库可选 Ant Design + 自定义响应式封装（或团队已有组件库）。

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
| 商家区 | 新建商品页 | `/merchant/products/new` |
| 商家区 | 商品编辑页 | `/merchant/products/:productId/edit` |
| 商家区 | 商品详情页 | `/merchant/products/:productId` |
| 商家区 | 订单列表页 | `/merchant/orders` |
| 商家区 | 订单详情页 | `/merchant/orders/:orderId` |
| 商家区 | 账号设置页 | `/merchant/account` |
| 商家区 | 商家操作日志页 | `/merchant/logs` |

## 2. 页面级规格

## 2.1 认证区页面

### 登录页（`/login`）
- 角色：未登录用户（管理员/商家共用入口，通过账号类型或后端返回角色判断跳转）。
- 核心内容：账号、密码、登录按钮、忘记密码提示（本期仅提示联系平台）。
- 关键交互：登录成功后按角色跳转；登录失败展示错误码映射文案。
- 依赖接口：`POST /api/v1/auth/login`。
- 移动端适配：单列布局、按钮全宽、输入框大触控区。

### 商家注册页（`/register`）
- 角色：未登录商家。
- 核心表单：企业名称、联系人、手机号、登录账号、密码、营业执照上传。
- 关键交互：前端字段校验 + 图片上传预览 + 提交后跳转状态页。
- 依赖接口：`POST /api/v1/auth/register`、`POST /api/v1/files/presign`。
- 移动端适配：分步表单（基础信息 -> 资质上传 -> 确认）。

### 注册状态页（`/register/status`）
- 角色：已注册未审核商家。
- 核心信息：审核状态、驳回原因、重新提交入口。
- 关键交互：`REJECTED` 显示“修改并重提”按钮。
- 依赖接口：`GET /api/v1/merchant/profile`、`POST /api/v1/merchant/reapply`。
- 移动端适配：状态卡片 + 固定底部操作按钮。

## 2.2 管理员区页面

### 审核列表页（`/admin/merchants/reviews`）
- 角色：PlatformAdmin。
- 核心内容：审核表格/卡片、状态筛选、关键词搜索、时间范围筛选。
- 关键交互：点击记录进入详情；支持快速通过/驳回（可选二次确认）。
- 依赖接口：`GET /api/v1/admin/merchants`。
- 移动端适配：筛选抽屉 + 卡片列表，卡片内显示核心字段与操作按钮。

### 审核详情页（`/admin/merchants/reviews/:merchantId`）
- 角色：PlatformAdmin。
- 核心内容：商家资料、证照图片、审核历史时间线。
- 关键交互：通过/驳回；驳回必须填写原因。
- 依赖接口：`GET /api/v1/admin/merchants/:id`、`POST /api/v1/admin/merchants/:id/approve`、`POST /api/v1/admin/merchants/:id/reject`。
- 移动端适配：信息分组折叠、图片全屏预览。

### 全局操作日志页（`/admin/logs`）
- 角色：PlatformAdmin。
- 核心内容：按对象类型/操作类型/角色筛选日志。
- 关键交互：查看日志详情（前后状态对比）。
- 依赖接口：`GET /api/v1/admin/logs`。
- 移动端适配：日志卡片 + 详情侧滑面板。

## 2.3 商家区页面

### 仪表盘页（`/merchant/dashboard`）
- 角色：MerchantOwner。
- 核心内容：商品统计（上架/下架/售出/关闭）、订单统计、待办提醒。
- 关键交互：快捷跳转至“新建商品”“商品列表”“订单列表”。
- 依赖接口：`GET /api/v1/merchant/dashboard`。
- 移动端适配：统计卡宫格 + 快捷入口吸顶。

### 商品列表页（`/merchant/products`）
- 角色：MerchantOwner。
- 核心内容：商品列表、状态标签、筛选器、分页。
- 关键交互：上架/下架/关闭/编辑/查看详情；支持批量下架（可选）。
- 依赖接口：`GET /api/v1/merchant/products`、状态操作接口。
- 移动端适配：卡片视图替代表格，操作按钮收敛到“更多菜单”。

### 新建商品页（`/merchant/products/new`）
- 角色：MerchantOwner。
- 核心内容：基础信息表单（标题、分类、价格、成色、库存、描述、图片）。
- 关键交互：保存草稿与“保存并上架”双按钮。
- 依赖接口：`POST /api/v1/merchant/products`、`POST /api/v1/files/presign`。
- 移动端适配：分段表单、图片上传宫格、提交按钮底部固定。

### 商品编辑页（`/merchant/products/:productId/edit`）
- 角色：MerchantOwner。
- 核心内容：回填商品信息并编辑。
- 关键交互：受状态限制（例如 `SOLD/CLOSED` 禁止编辑）。
- 依赖接口：`GET /api/v1/merchant/products/:id`、`PUT /api/v1/merchant/products/:id`。
- 移动端适配：与新建页一致，保留状态限制提示。

### 商品详情页（`/merchant/products/:productId`）
- 角色：MerchantOwner。
- 核心内容：商品完整信息、状态、操作时间线、关联订单。
- 关键交互：状态切换（上架/下架/关闭）、发起成交（创建订单）。
- 依赖接口：商品详情、状态操作、订单创建接口。
- 移动端适配：图文详情纵向布局 + 操作浮动栏。

### 订单列表页（`/merchant/orders`）
- 角色：MerchantOwner。
- 核心内容：订单状态筛选、商品关键词筛选、分页列表。
- 关键交互：完成订单、关闭订单、查看详情。
- 依赖接口：`GET /api/v1/merchant/orders`。
- 移动端适配：时间轴式卡片，按钮分主次显示。

### 订单详情页（`/merchant/orders/:orderId`）
- 角色：MerchantOwner。
- 核心内容：订单信息、商品摘要、状态流转记录。
- 关键交互：`CREATED` 状态下可“完成”或“关闭”。
- 依赖接口：`GET /api/v1/merchant/orders/:id`、状态操作接口。
- 移动端适配：详情区块卡片化。

### 账号设置页（`/merchant/account`）
- 角色：MerchantOwner。
- 核心内容：基本信息、修改密码、登录设备列表（可选）。
- 关键交互：修改密码后强制重新登录（可选策略）。
- 依赖接口：`GET /api/v1/merchant/account`、`PUT /api/v1/merchant/account/password`。
- 移动端适配：表单纵向布局、危险操作分区显著提示。

### 商家操作日志页（`/merchant/logs`）
- 角色：MerchantOwner。
- 核心内容：本商家范围内操作日志列表。
- 关键交互：筛选操作类型、时间区间，查看详情。
- 依赖接口：`GET /api/v1/merchant/logs`。
- 移动端适配：卡片列表 + 折叠详情。

## 3. 跨页面通用规范
1. 权限守卫：
   - 未登录访问受限页跳转登录。
   - 商家审核未通过登录后统一跳转 `/register/status`。
   - 管理员与商家路由互斥访问。
2. 错误处理：
   - 401 统一触发登录失效逻辑。
   - 403 展示无权限页。
   - 业务错误码映射用户可理解文案。
3. 列表体验：
   - 所有列表支持分页、状态筛选、关键词筛选。
   - 筛选条件与 URL query 同步，支持刷新恢复。
4. 上传体验：
   - 上传中、成功、失败状态可见。
   - 支持图片压缩预览（前端可选）。

## 4. 响应式适配策略
1. 断点建议：
   - `>=1200px`：桌面宽屏
   - `768px~1199px`：平板/小桌面
   - `<768px`：手机
2. 布局规则：
   - PC：左侧导航 + 顶部栏 + 主内容区。
   - Mobile：底部导航（商家区）或抽屉导航（管理员区）。
3. 高密度表格降级：
   - 表格列过多时在移动端转换为卡片并保留关键字段。
4. 关键操作可达性：
   - 手机上架/下架/成交/关闭按钮保持单手操作区域可达。

## 5. 前端开发检查清单
1. 页面路由、鉴权守卫、角色路由隔离已完成。
2. 所有页面已接入真实 API（无 mock 残留）。
3. 所有状态操作有二次确认与成功失败反馈。
4. 移动端关键页面通过真机检查（iOS Safari / Android Chrome）。
5. 列表筛选与分页行为一致，URL 可回溯状态。
