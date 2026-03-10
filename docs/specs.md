# 产品 Specs / 目标文档（specs）

## 默认假设
1. 当前生效角色：`PlatformAdmin`（`SUPER_ADMIN/ADMIN`）、`MerchantOwner`。
2. 订单为轻量交易记录，不承载支付、退款、售后。
3. 商家采用受限制登录（restricted login）：`PENDING/REJECTED` 可登录但只获得 `onboarding scope`，`APPROVED` 获得 `full scope`。
4. 商品与订单状态机以本文为唯一准则。
5. 商品支持订单占用状态 `LOCKED`，用于避免同一商品并发成交。
6. 分类采用两级只读字典，本期不开放分类后台管理页面。

## 1. 产品目标

### 1.1 业务目标
1. 将商家入驻周期标准化并可追踪。
2. 将商家商品管理流程产品化，减少线下沟通成本。
3. 建立可实现的商品-订单闭环，避免状态歧义。
4. 为后续子账号、订单扩展建立稳定接口基础。

### 1.2 成功指标（本期）
1. 商家注册后 5 分钟内可在管理员审核列表中检索到。
2. 审核通过后商家登录成功率 >= 99%（排除密码错误）。
3. 商品发布到上架流程可在 3 分钟内完成（中位时间）。
4. 商品管理与订单关键操作审计覆盖率 100%。
5. 移动端核心流程（登录、商品新增、上下架、订单完成）完成率 >= 95%。

## 2. 用户与核心场景

### 2.1 平台管理员（PlatformAdmin）
1. 审核商家资质并给出通过/驳回理由。
2. 查看商家经营状态与关键日志。
3. `SUPER_ADMIN` 可通过初始化脚本管理管理员账户；本期无页面入口。

### 2.2 商家主账号（MerchantOwner）
1. 完成注册并跟踪审核结果。
2. 管理商品全生命周期（草稿、上架、锁定、下架、成交、关闭）。
3. 创建并维护轻量订单状态。

## 3. 功能规格

### 3.1 认证与账号
1. 商家注册：
   - 输入企业名、联系人、手机号、登录账号、密码、营业执照图片。
   - 提交后创建商家与主账号，审核状态 `PENDING`。
2. 登录：
   - 请求需带 `login_type`（`ADMIN/MERCHANT`）。
   - 商家 `PENDING/REJECTED` 允许登录，但只返回 `onboarding scope` token。
   - 商家 `APPROVED` 登录返回 `full scope` token。
   - 返回 `access_token`、`refresh_token`、`token_scope`、用户信息。
3. 管理员账号：
   - 本期通过初始化脚本预置，不支持后台新增管理员。
   - 角色区分 `SUPER_ADMIN/ADMIN`，但本期无管理员管理页面。
4. 令牌刷新与退出：
   - 支持刷新 token。
   - 退出后当前 refresh token 失效。

restricted login 规则：
1. `onboarding scope` 允许：
   - `GET /merchant/profile`
   - `POST /merchant/reapply`
   - 入驻资质上传（`/files/presign`、`/files/confirm`，仅 `MERCHANT_LICENSE`）
2. `onboarding scope` 禁止：
   - 商品管理、订单管理、仪表盘、商家日志、账号设置等正式经营能力。
3. 前端跳转：
   - `PENDING/REJECTED` 登录成功后统一跳转 `/register/status`。
   - `APPROVED` 登录成功后跳转 `/merchant/dashboard`。

### 3.2 商家审核
1. 审核列表支持筛选：状态、时间区间、关键词（商家名/联系人/手机号）。
2. 审核详情展示：基础资料、证照图片、历史审核记录。
3. 审核动作：
   - 通过：状态改为 `APPROVED`。
   - 驳回：状态改为 `REJECTED`，必须填写驳回原因。

接口职责边界：
1. `GET /merchant/profile`：返回“商家主体资料 + 审核状态”，用于注册状态页和审核流相关页面。
2. `GET /merchant/account`：返回“当前登录账号资料 + 安全设置信息”，用于账号设置页。
3. `merchant/profile` 与 `merchant/account` 禁止混用，避免把账号安全字段塞入商家主体模型。

### 3.3 分类字典
1. 分类为两级结构：一级分类 + 二级分类。
2. 商品必须选择二级分类（`category_id` 指向二级分类）。
3. 本期仅提供分类查询接口，不提供分类管理接口。
4. 分类数据通过初始化脚本维护，变更需走版本化脚本。

### 3.4 商品管理
1. 创建商品：标题、描述、二级分类、价格、成色、库存（固定 1）、图片。
2. 编辑商品：按状态执行字段级限制（见第 5.4 节）。
3. 状态操作：上架、下架、关闭。
4. 商品列表：
   - 支持状态筛选、关键词搜索、时间排序、分页。
   - PC 默认表格；移动端默认卡片。
5. `stock` 规则（本期）：
   - 不允许 `stock > 1`。
   - `stock` 为保留字段，默认值固定为 `1`，用于后续可能的多件同款扩展。
   - 若请求显式传入 `stock` 且不为 `1`，后端返回参数错误。

### 3.5 轻量订单
1. 创建订单：
   - 仅允许对 `ON_SHELF` 商品创建。
   - 创建成功后商品状态立即变更为 `LOCKED`。
2. 并发约束：
   - 同一商品仅允许一个 `CREATED` 订单并存。
   - 若已有未完成订单，创建新订单返回冲突错误码。
3. 完成订单：
   - 订单 `CREATED -> COMPLETED`。
   - 商品 `LOCKED -> SOLD`。
4. 关闭订单：
   - 订单 `CREATED -> CLOSED`。
   - 商品 `LOCKED -> OFF_SHELF`（统一回退规则）。

### 3.6 审计与日志
1. 关键操作必须写日志：
   - 审核通过/驳回
   - 商品创建/编辑/上架/下架/关闭
   - 订单创建/完成/关闭
2. 日志内容：操作人、角色、对象、前后状态、结果、时间、请求标识。

## 4. 关键流程规格

### 4.1 注册 -> 审核 -> 登录
1. 商家提交注册后，系统返回“待审核”状态。
2. 待审核与驳回状态允许登录，但仅返回 `onboarding scope`。
3. `onboarding scope` 仅允许访问入驻流程接口，访问经营接口返回 `10006`。
4. 审核通过后再次登录返回 `full scope`，进入商家首页。

### 4.2 发布商品 -> 上下架 -> 成交/关闭
1. 商家创建商品默认 `DRAFT`。
2. 满足必填字段后允许上架至 `ON_SHELF`。
3. 创建订单后自动锁定 `LOCKED`。
4. 订单完成后商品变更 `SOLD`。
5. 订单关闭后商品回退 `OFF_SHELF`，由商家决定是否再次上架。

## 5. 状态机定义

### 5.1 商家审核状态机

| 当前状态 | 目标状态 | 触发动作 | 触发角色 | 规则 |
| --- | --- | --- | --- | --- |
| `PENDING` | `APPROVED` | 审核通过 | PlatformAdmin | 必须记录审核人和时间 |
| `PENDING` | `REJECTED` | 审核驳回 | PlatformAdmin | 必须填写驳回原因 |
| `REJECTED` | `PENDING` | 重新提交资料 | MerchantOwner | 变更后重新进入审核 |

### 5.2 商品状态机

| 当前状态 | 目标状态 | 触发动作 | 触发角色 | 规则 |
| --- | --- | --- | --- | --- |
| `DRAFT` | `ON_SHELF` | 上架 | MerchantOwner | 必填字段齐全 |
| `ON_SHELF` | `OFF_SHELF` | 下架 | MerchantOwner | 即时生效 |
| `OFF_SHELF` | `ON_SHELF` | 重新上架 | MerchantOwner | 字段仍合法 |
| `ON_SHELF` | `LOCKED` | 创建订单 | MerchantOwner | 仅无未完成订单时允许 |
| `LOCKED` | `SOLD` | 完成订单 | MerchantOwner | 事务内更新订单与商品 |
| `LOCKED` | `OFF_SHELF` | 关闭订单 | MerchantOwner | 统一回退到下架态 |
| `DRAFT`/`ON_SHELF`/`OFF_SHELF` | `CLOSED` | 关闭商品 | MerchantOwner | 关闭后不可恢复 |

### 5.3 订单状态机

| 当前状态 | 目标状态 | 触发动作 | 触发角色 | 规则 |
| --- | --- | --- | --- | --- |
| `CREATED` | `COMPLETED` | 完成订单 | MerchantOwner | 商品 `LOCKED -> SOLD` |
| `CREATED` | `CLOSED` | 关闭订单 | MerchantOwner | 商品 `LOCKED -> OFF_SHELF` |

### 5.4 商品字段级编辑与操作矩阵

| 商品状态 | 允许编辑字段 | 禁止编辑字段 | 允许操作 | 校验要求 |
| --- | --- | --- | --- | --- |
| `DRAFT` | 标题、描述、分类、价格、成色、图片 | 库存 | 上架、关闭 | 前端库存固定展示 1；后端拒绝修改库存 |
| `ON_SHELF` | 描述、图片 | 分类、价格、库存、成色、标题 | 下架、创建订单、关闭 | 前端禁用禁止字段；后端二次校验拒绝越权字段 |
| `OFF_SHELF` | 标题、描述、分类、价格、成色、图片 | 库存 | 上架、关闭 | 与 `DRAFT` 类似，允许完整调整后再上架 |
| `LOCKED` | 无 | 全字段禁止 | 完成订单、关闭订单（订单页） | 商品页按钮禁用编辑；后端仅允许订单状态接口 |
| `SOLD` | 无 | 全字段禁止 | 无 | 前端隐藏动作按钮；后端统一返回状态流转错误 |
| `CLOSED` | 无 | 全字段禁止 | 无 | 前端隐藏动作按钮；后端统一返回状态流转错误 |

## 6. 商品-订单状态联动表

| 场景 | 订单变化 | 商品变化 | 是否事务内 | 备注 |
| --- | --- | --- | --- | --- |
| 创建订单 | `N/A -> CREATED` | `ON_SHELF -> LOCKED` | 是 | 同一商品仅允许一个 `CREATED` |
| 完成订单 | `CREATED -> COMPLETED` | `LOCKED -> SOLD` | 是 | 失败整体回滚 |
| 关闭订单 | `CREATED -> CLOSED` | `LOCKED -> OFF_SHELF` | 是 | 关闭后不自动上架 |

## 7. 非功能规格
1. 鉴权：JWT + Refresh Token，access token 默认 2h，refresh token 默认 7d。
2. 错误码：统一业务错误码，前后端共享字典。
3. 日志审计：所有状态变更接口必须写审计日志。
4. 分页筛选：统一 `page/page_size/total/items`。
5. 图片上传：类型限制（jpg/png/webp），单图 <= 5MB，单商品 <= 9 张。
6. 并发控制：订单创建/完成/关闭必须基于事务与行级锁。
7. 幂等处理：状态变更类接口支持幂等重试（重复请求返回一致结果或冲突码）。
8. 移动端适配：最小支持宽度 360px，关键功能无横向滚动阻塞。
9. 可维护性：模块分层、统一 DTO、接口文档与实现同版本维护。

## 8. 验收标准（关键用例）
1. Given 商家处于 `PENDING`，When 使用正确密码登录，Then 登录成功且 `token_scope=onboarding`。
2. Given 商家处于 `REJECTED`，When 使用正确密码登录，Then 登录成功且 `token_scope=onboarding`。
3. Given `onboarding scope`，When 访问商品或订单接口，Then 返回 `10006`。
4. Given 管理员审核通过商家，When 商家再次登录，Then 登录成功且 `token_scope=full`。
5. Given 商品为 `DRAFT` 且必填项不全，When 发起上架，Then 返回参数校验错误。
6. Given 商品为 `ON_SHELF`，When 创建订单，Then 创建成功且商品变更为 `LOCKED`。
7. Given 商品已 `LOCKED` 且有 `CREATED` 订单，When 再次创建订单，Then 返回并发冲突错误。
8. Given 订单为 `CREATED`，When 完成订单，Then 订单为 `COMPLETED` 且商品为 `SOLD`。
9. Given 订单为 `CREATED`，When 关闭订单，Then 订单为 `CLOSED` 且商品为 `OFF_SHELF`。
10. Given `LOCKED/SOLD/CLOSED` 商品，When 调用商品编辑接口，Then 返回状态流转或字段限制错误。

## 9. 版本边界
1. 本期不做支付、退款、售后和买家订单展示。
2. 子账号与 RBAC 只保留设计接口，不做可用功能。
3. 分类管理页面不纳入本期，分类通过初始化脚本维护。
4. 管理员管理管理员不纳入本期。
