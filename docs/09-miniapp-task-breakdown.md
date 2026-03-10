# 09 - Miniapp Task Breakdown

## 阻塞说明
当前文档体系（实现文档 + 数据模型 + API checklist）已可支持任务拆解，无硬阻塞。

实现前置准备（不阻塞排期）：
1. 微信小程序 `appid/app_secret` 与测试账号。
2. 网关确认支持 `X-Device-Id` 透传。
3. 测试环境准备 buyer/merchant 联调账号与基础商品数据。

---

## 拆解约束
1. 买家小程序与商家后台配套任务分离，不混成单任务。
2. 买家域全部使用 `/api/v1/buyer/*`，不复用商家经营接口。
3. “购买意向闭环”与“游客合并机制”独立拆解。
4. 仅覆盖本期范围，不扩展支付、退款、售后、聊天、推荐。

---

## 一、买家端前端任务

### Task FE-001
- Task ID: FE-001
- Task Title: 小程序工程骨架初始化（Taro + React）
- Priority: P0
- Domain: Frontend-Miniapp
- Goal: 建立可运行的买家小程序工程骨架。
- Inputs: `docs/miniapp-buyer-implementation-plan.md` §5/§8
- Scope: 工程初始化、目录结构、路由骨架、环境配置。
- Output: 可启动小程序工程与基础页面路由。
- Dependencies: 无
- Test Requirements: 本地启动成功，路由可访问占位页。
- Definition of Done: 开发/构建命令可用，基础路由与文档一致。
- Risks: Taro 版本与插件兼容性问题。

### Task FE-002
- Task ID: FE-002
- Task Title: 公共请求层 / token / device_id 注入
- Priority: P0
- Domain: Frontend-Miniapp
- Goal: 实现统一请求与会话处理。
- Inputs: `docs/miniapp-buyer-api-checklist.md` §4/§10
- Scope: 请求封装、`Authorization` 注入、`X-Device-Id` 注入、401 refresh、错误码映射。
- Output: 可复用请求 SDK。
- Dependencies: FE-001
- Test Requirements: 游客请求自动带 `device_id`；token 过期自动刷新。
- Definition of Done: 全页面通过统一请求层调用 API。
- Risks: refresh 并发导致重复请求或循环刷新。

### Task FE-003
- Task ID: FE-003
- Task Title: 首页
- Priority: P0
- Domain: Frontend-Miniapp
- Goal: 提供首屏商品浏览入口。
- Inputs: `docs/miniapp-buyer-implementation-plan.md` §5
- Scope: 推荐流、下拉刷新、分页加载、空态。
- Output: 首页可稳定展示商品并跳转详情。
- Dependencies: FE-002, BE-008
- Test Requirements: 有数据/无数据/翻页场景通过。
- Definition of Done: 首页可用，跳详情链路打通。
- Risks: 分页和排序参数与后端不一致。

### Task FE-004
- Task ID: FE-004
- Task Title: 分类页
- Priority: P1
- Domain: Frontend-Miniapp
- Goal: 提供分类导航与跳转能力。
- Inputs: `docs/miniapp-buyer-api-checklist.md` §3
- Scope: 一级/二级分类展示、分类切换、跳商品列表。
- Output: 分类页可驱动列表筛选。
- Dependencies: FE-002, BE-007
- Test Requirements: 分类切换后列表参数正确。
- Definition of Done: 分类数据加载、切换、跳转均可用。
- Risks: 分类空数据或异常时页面可用性。

### Task FE-005
- Task ID: FE-005
- Task Title: 搜索页
- Priority: P0
- Domain: Frontend-Miniapp
- Goal: 支持关键词搜索商品。
- Inputs: `docs/miniapp-buyer-implementation-plan.md` §5
- Scope: 搜索输入、历史记录、结果跳转。
- Output: 搜索可进入列表结果。
- Dependencies: FE-002, BE-008
- Test Requirements: 关键词为空/命中/无结果场景通过。
- Definition of Done: 搜索流程可用且状态恢复正确。
- Risks: 高频输入触发请求风暴。

### Task FE-006
- Task ID: FE-006
- Task Title: 商品列表页
- Priority: P0
- Domain: Frontend-Miniapp
- Goal: 展示分类/搜索结果并支持分页。
- Inputs: `docs/miniapp-buyer-api-checklist.md` §1
- Scope: 列表卡片、排序、分页、状态标签。
- Output: 商品列表页可稳定浏览。
- Dependencies: FE-002, BE-008
- Test Requirements: 分页、排序、筛选组合返回正确。
- Definition of Done: 列表可稳定展示并跳详情。
- Risks: 状态显示与后端枚举不一致。

### Task FE-007
- Task ID: FE-007
- Task Title: 商品详情页
- Priority: P0
- Domain: Frontend-Miniapp
- Goal: 承接核心转化（收藏、登录引导、意向入口）。
- Inputs: `docs/miniapp-buyer-api-checklist.md` §2/§7
- Scope: 图文详情、图片预览、分享、收藏按钮、提交意向入口。
- Output: 详情页可用并可触发收藏与意向流程。
- Dependencies: FE-002, BE-008, BE-009, BE-010
- Test Requirements: 在售/不可售状态下按钮行为正确。
- Definition of Done: 详情链路完整，状态展示正确。
- Risks: 不可售状态误放开提交意向。

### Task FE-008
- Task ID: FE-008
- Task Title: 收藏页
- Priority: P1
- Domain: Frontend-Miniapp
- Goal: 提供收藏列表管理能力。
- Inputs: `docs/miniapp-buyer-api-checklist.md` §5
- Scope: 列表、取消收藏、空态、状态同步。
- Output: 收藏页可读写收藏。
- Dependencies: FE-002, BE-009
- Test Requirements: 游客态和登录态收藏行为一致。
- Definition of Done: 收藏页与详情页状态实时同步。
- Risks: 本地态和服务端态不同步。

### Task FE-009
- Task ID: FE-009
- Task Title: 浏览记录页
- Priority: P1
- Domain: Frontend-Miniapp
- Goal: 展示并清理浏览历史。
- Inputs: `docs/miniapp-buyer-api-checklist.md` §6
- Scope: 记录列表、单条删除、全部清空。
- Output: 浏览记录页可查询和清理。
- Dependencies: FE-002, BE-010
- Test Requirements: 浏览上报后记录可见，清理生效。
- Definition of Done: 浏览记录页可稳定使用。
- Risks: 去重窗口导致用户误认为未记录。

### Task FE-010
- Task ID: FE-010
- Task Title: 登录页（显式授权登录）
- Priority: P0
- Domain: Frontend-Miniapp
- Goal: 提供微信授权登录与登录后回跳。
- Inputs: `docs/miniapp-buyer-implementation-plan.md` §2/§7
- Scope: 登录触发、失败处理、回跳参数保留、隐私协议确认。
- Output: 登录页和登录流程可用。
- Dependencies: FE-002, BE-006
- Test Requirements: 未登录提交意向跳转登录并成功回跳。
- Definition of Done: 登录成功后用户态与 token 生效。
- Risks: 授权失败重试路径不完整。

### Task FE-011
- Task ID: FE-011
- Task Title: 游客引导页
- Priority: P2
- Domain: Frontend-Miniapp
- Goal: 解释游客模式和登录收益。
- Inputs: `docs/miniapp-buyer-implementation-plan.md` §5
- Scope: 首次引导、关闭后不再弹出。
- Output: 游客引导页。
- Dependencies: FE-001
- Test Requirements: 首次/非首次展示逻辑正确。
- Definition of Done: 引导不阻塞主链路。
- Risks: 引导过重影响首屏转化。

### Task FE-012
- Task ID: FE-012
- Task Title: 我的页面
- Priority: P1
- Domain: Frontend-Miniapp
- Goal: 展示买家身份与聚合统计。
- Inputs: `docs/miniapp-buyer-api-checklist.md` §8
- Scope: 游客态卡片、登录态资料、收藏/浏览/意向统计入口。
- Output: 我的页面可用。
- Dependencies: FE-002, BE-012
- Test Requirements: 游客和登录态展示差异正确。
- Definition of Done: 统计数据与对应页面一致。
- Risks: 聚合统计缓存导致短时不一致。

### Task FE-013
- Task ID: FE-013
- Task Title: 购买意向提交页
- Priority: P0
- Domain: Frontend-Miniapp
- Goal: 登录买家提交购买意向。
- Inputs: `docs/miniapp-buyer-api-checklist.md` §7.1
- Scope: 表单校验（手机号/微信号至少一项）、提交、防重复点击。
- Output: 意向提交能力可用。
- Dependencies: FE-010, BE-011, ST-005
- Test Requirements: 未登录拦截、校验错误提示、冲突提示正确。
- Definition of Done: 提交成功后可进入我的意向列表。
- Risks: 前后端表单规则不一致。

### Task FE-014
- Task ID: FE-014
- Task Title: 我的意向页
- Priority: P0
- Domain: Frontend-Miniapp
- Goal: 买家查看意向状态回读。
- Inputs: `docs/miniapp-buyer-api-checklist.md` §7.2/§7.3
- Scope: 意向列表、状态筛选、详情查看、简化状态映射展示。
- Output: 我的意向页可查询状态。
- Dependencies: FE-002, BE-011, MB-004
- Test Requirements: 商家处理后状态在买家端正确回读。
- Definition of Done: “处理中/已联系/已关闭”映射稳定一致。
- Risks: 状态映射字段命名不统一。

---

## 二、买家端后端任务

### Task BE-001
- Task ID: BE-001
- Task Title: buyer_users 数据表与模型
- Priority: P0
- Domain: Backend-Buyer
- Goal: 建立买家账号主表。
- Inputs: `docs/miniapp-buyer-data-model.md` §2.1
- Scope: 表结构、索引、状态枚举、迁移脚本。
- Output: `buyer_users` 可读写。
- Dependencies: 无
- Test Requirements: `openid` 唯一约束生效。
- Definition of Done: 迁移通过，模型可用。
- Risks: 微信 openid 重复冲突处理。

### Task BE-002
- Task ID: BE-002
- Task Title: buyer_device_bindings 数据表与模型
- Priority: P0
- Domain: Backend-Buyer
- Goal: 建立设备与账号绑定关系。
- Inputs: `docs/miniapp-buyer-data-model.md` §2.2
- Scope: 表结构、唯一约束、绑定时间字段。
- Output: `buyer_device_bindings` 可用。
- Dependencies: BE-001
- Test Requirements: `uk_device_buyer` 生效。
- Definition of Done: 可记录一机多号与一号多机。
- Risks: 绑定记录膨胀。

### Task BE-003
- Task ID: BE-003
- Task Title: buyer_favorites 数据表与模型
- Priority: P0
- Domain: Backend-Buyer
- Goal: 支撑游客与登录态收藏。
- Inputs: `docs/miniapp-buyer-data-model.md` §2.3
- Scope: owner_key、唯一约束、失活与合并标记字段。
- Output: `buyer_favorites` 可用。
- Dependencies: BE-001
- Test Requirements: 同 owner 同商品去重生效。
- Definition of Done: 收藏可增删查并保留历史。
- Risks: owner_key 生成不统一。

### Task BE-004
- Task ID: BE-004
- Task Title: buyer_histories 数据表与模型
- Priority: P0
- Domain: Backend-Buyer
- Goal: 支撑游客与登录态浏览记录。
- Inputs: `docs/miniapp-buyer-data-model.md` §2.4
- Scope: 时间字段、view_count、合并标记、唯一约束。
- Output: `buyer_histories` 可用。
- Dependencies: BE-001
- Test Requirements: 同 owner 同商品 upsert 生效。
- Definition of Done: 浏览记录可上报与查询。
- Risks: 高频写入导致热点。

### Task BE-005
- Task ID: BE-005
- Task Title: buyer_intents 数据表与模型
- Priority: P0
- Domain: Backend-Buyer
- Goal: 建立购买意向线索模型。
- Inputs: `docs/miniapp-buyer-data-model.md` §2.5
- Scope: 状态字段、`is_open`、关闭原因、唯一冲突约束。
- Output: `buyer_intents` 可用。
- Dependencies: BE-001
- Test Requirements: 同 buyer 同商品未关闭唯一约束生效。
- Definition of Done: 意向状态机数据结构可支撑闭环。
- Risks: 状态与 `is_open` 同步错误。

### Task BE-006
- Task ID: BE-006
- Task Title: buyer auth（wechat-login / refresh / logout / guest merge）
- Priority: P0
- Domain: Backend-Buyer
- Goal: 提供买家认证与游客数据合并入口。
- Inputs: `docs/miniapp-buyer-api-checklist.md` §4
- Scope: `wechat-login`、`refresh`、`logout`、`guest/merge` API。
- Output: `/api/v1/buyer/auth/*` 与 `/api/v1/buyer/guest/merge`。
- Dependencies: BE-001, BE-002, BE-003, BE-004, ST-001, ST-002
- Test Requirements: 登录签发 token 正确，merge 幂等。
- Definition of Done: 登录后收藏/浏览数据可归并到账号。
- Risks: 第三方微信服务不稳定。

### Task BE-007
- Task ID: BE-007
- Task Title: buyer categories API
- Priority: P1
- Domain: Backend-Buyer
- Goal: 提供买家可访问分类接口。
- Inputs: `docs/miniapp-buyer-api-checklist.md` §3
- Scope: `GET /api/v1/buyer/categories`。
- Output: 买家分类 API。
- Dependencies: 无（复用 categories）
- Test Requirements: 仅返回 ENABLED 分类。
- Definition of Done: 分类接口稳定可调用。
- Risks: 分类层级兼容差异。

### Task BE-008
- Task ID: BE-008
- Task Title: buyer products list / detail API
- Priority: P0
- Domain: Backend-Buyer
- Goal: 提供买家商品浏览核心接口。
- Inputs: `docs/miniapp-buyer-api-checklist.md` §1/§2
- Scope: `GET /buyer/products`、`GET /buyer/products/:id`，状态可见性过滤。
- Output: 买家商品列表与详情 API。
- Dependencies: ST-004
- Test Requirements: 列表仅在售，详情状态可见性正确。
- Definition of Done: 前端浏览链路可完整调用。
- Risks: 图片 URL 组装错误。

### Task BE-009
- Task ID: BE-009
- Task Title: favorites API
- Priority: P0
- Domain: Backend-Buyer
- Goal: 实现收藏读写接口。
- Inputs: `docs/miniapp-buyer-api-checklist.md` §5
- Scope: `GET/POST/DELETE /api/v1/buyer/favorites`。
- Output: 收藏 API 可用。
- Dependencies: BE-003, ST-004
- Test Requirements: 游客/登录 owner 行为正确。
- Definition of Done: 收藏操作幂等且可回读。
- Risks: owner 切换导致重复数据。

### Task BE-010
- Task ID: BE-010
- Task Title: histories API
- Priority: P0
- Domain: Backend-Buyer
- Goal: 实现浏览记录上报与查询接口。
- Inputs: `docs/miniapp-buyer-api-checklist.md` §6
- Scope: `POST /views`、`GET /histories`、`DELETE /histories`。
- Output: 浏览记录 API 可用。
- Dependencies: BE-004, ST-002
- Test Requirements: 30 秒去重、次数累加、清空逻辑正确。
- Definition of Done: 浏览记录闭环可用。
- Risks: 高频写请求引发性能问题。

### Task BE-011
- Task ID: BE-011
- Task Title: intents API
- Priority: P0
- Domain: Backend-Buyer
- Goal: 实现买家意向创建与查询接口。
- Inputs: `docs/miniapp-buyer-api-checklist.md` §7
- Scope: `POST /buyer/intents`、`GET /buyer/intents`、`GET /buyer/intents/:id`。
- Output: 买家意向 API 可用。
- Dependencies: BE-005, ST-003, ST-004, ST-005
- Test Requirements: 登录校验、冲突规则、状态映射字段正确。
- Definition of Done: 买家可提交并回读自己的意向状态。
- Risks: 并发重复提交处理不稳定。

### Task BE-012
- Task ID: BE-012
- Task Title: me summary API
- Priority: P1
- Domain: Backend-Buyer
- Goal: 提供买家“我的”聚合统计接口。
- Inputs: `docs/miniapp-buyer-api-checklist.md` §8
- Scope: `GET /api/v1/buyer/me/summary`。
- Output: 汇总 API。
- Dependencies: BE-009, BE-010, BE-011
- Test Requirements: 游客态/登录态返回结构正确。
- Definition of Done: 前端我的页面可直接消费。
- Risks: 聚合查询性能波动。

---

## 三、商家后台配套任务

### Task MB-001
- Task ID: MB-001
- Task Title: 商家意向线索列表页
- Priority: P0
- Domain: Merchant-Frontend
- Goal: 商家可查看线索池并筛选。
- Inputs: `docs/miniapp-buyer-implementation-plan.md` §4.3
- Scope: 列表、筛选、分页、状态标签。
- Output: 意向线索列表页。
- Dependencies: MB-003
- Test Requirements: 筛选、分页、状态展示正确。
- Definition of Done: 商家可进入线索详情。
- Risks: 大量线索时列表性能下降。

### Task MB-002
- Task ID: MB-002
- Task Title: 商家意向详情页
- Priority: P0
- Domain: Merchant-Frontend
- Goal: 商家查看线索详情并执行处理。
- Inputs: `docs/miniapp-buyer-implementation-plan.md` §4.3.1
- Scope: 联系方式、留言、商品信息、处理按钮。
- Output: 意向详情页。
- Dependencies: MB-003
- Test Requirements: 状态按钮按规则启用/禁用。
- Definition of Done: 详情可触发处理动作并刷新状态。
- Risks: 状态并发变化导致操作冲突。

### Task MB-003
- Task ID: MB-003
- Task Title: 商家处理意向状态接口
- Priority: P0
- Domain: Merchant-Backend
- Goal: 提供商家端意向查询与状态处理 API。
- Inputs: `docs/miniapp-buyer-api-checklist.md` §9
- Scope: `GET /merchant/intents`、`GET /merchant/intents/:id`、`POST /contacted`、`POST /close`。
- Output: 商家意向处理接口。
- Dependencies: BE-005
- Test Requirements: 合法流转成功，非法流转返回 `10005`。
- Definition of Done: 商家可处理本商家线索且有审计记录。
- Risks: 越权访问他商家线索。

### Task MB-004
- Task ID: MB-004
- Task Title: 买家可见状态映射与回读
- Priority: P0
- Domain: Merchant-Backend
- Goal: 保证商家处理后买家端可读到统一简化状态。
- Inputs: `docs/miniapp-buyer-implementation-plan.md` §4.3.2
- Scope: `NEW/CONTACTED/CLOSED` -> `处理中/已联系/已关闭` 映射输出。
- Output: 状态映射统一实现与回读字段。
- Dependencies: MB-003, BE-011
- Test Requirements: 买家列表与详情状态文本一致。
- Definition of Done: 映射逻辑单一来源，无双份实现。
- Risks: 双端分别映射导致不一致。

---

## 四、状态与合并机制任务

### Task ST-001
- Task ID: ST-001
- Task Title: 游客收藏合并规则
- Priority: P0
- Domain: Backend-Core
- Goal: 登录后收藏数据无损并幂等合并。
- Inputs: `docs/miniapp-buyer-data-model.md` §2.3/§5
- Scope: owner_key 归并、upsert 去重、合并标记写入。
- Output: 收藏合并服务规则。
- Dependencies: BE-003, BE-006
- Test Requirements: 重复 merge 不重复插入。
- Definition of Done: 合并前后收藏集合符合预期。
- Risks: 部分失败导致数据不一致。

### Task ST-002
- Task ID: ST-002
- Task Title: 游客浏览记录合并规则
- Priority: P0
- Domain: Backend-Core
- Goal: 浏览记录合并后时间和次数正确。
- Inputs: `docs/miniapp-buyer-data-model.md` §2.4/§5
- Scope: `last_viewed_at=max`、`view_count` 累加、合并标记。
- Output: 浏览记录合并服务规则。
- Dependencies: BE-004, BE-006
- Test Requirements: 多次 merge 结果稳定幂等。
- Definition of Done: 时间与计数规则全部通过测试。
- Risks: 并发 merge 导致计数异常。

### Task ST-003
- Task ID: ST-003
- Task Title: 同 buyer 同商品未关闭意向冲突规则
- Priority: P0
- Domain: Backend-Core
- Goal: 阻止重复有效线索。
- Inputs: `docs/miniapp-buyer-data-model.md` §2.5
- Scope: 唯一约束、冲突码 `10010`、接口幂等策略。
- Output: 冲突规则统一落地。
- Dependencies: BE-005, BE-011
- Test Requirements: 并发提交仅一条成功。
- Definition of Done: 冲突行为可预测、错误码稳定。
- Risks: 数据库差异导致唯一约束行为不一致。

### Task ST-004
- Task ID: ST-004
- Task Title: 商品状态对买家可见性规则
- Priority: P0
- Domain: Backend-Core
- Goal: 统一买家视角可见范围。
- Inputs: `docs/miniapp-buyer-implementation-plan.md` §4.5
- Scope: 列表仅 `ON_SHELF`；详情支持 `ON_SHELF/LOCKED/OFF_SHELF/SOLD`；隐藏 `DRAFT/CLOSED`。
- Output: 状态过滤器与详情可见规则。
- Dependencies: BE-008, BE-011
- Test Requirements: 各状态返回行为符合文档。
- Definition of Done: 所有买家接口使用同一规则。
- Risks: 不同接口各自实现造成分叉。

### Task ST-005
- Task ID: ST-005
- Task Title: 买家提交意向校验与限流
- Priority: P0
- Domain: Backend-Core
- Goal: 防刷与质量控制。
- Inputs: `docs/miniapp-buyer-api-checklist.md` §7/§10
- Scope: 登录校验、联系方式校验、商品在售校验、频控阈值。
- Output: 意向提交校验链与限流策略。
- Dependencies: BE-011
- Test Requirements: 命中频控返回 `10009`，字段错误返回 `10001`。
- Definition of Done: 规则可观测、可回归。
- Risks: 频控阈值过严影响正常用户。

---

## 五、测试任务

### Task QA-001
- Task ID: QA-001
- Task Title: 后端单元测试
- Priority: P0
- Domain: QA-Backend
- Goal: 覆盖买家域核心规则单测。
- Inputs: buyer/merchant 新增服务代码
- Scope: 状态机、校验、限流、映射、owner_key。
- Output: 单元测试套件。
- Dependencies: BE-001~BE-012, MB-003~MB-004, ST-001~ST-005
- Test Requirements: 核心规则分支覆盖。
- Definition of Done: 单测稳定通过。
- Risks: 规则频繁迭代导致测试脆弱。

### Task QA-002
- Task ID: QA-002
- Task Title: 后端集成测试
- Priority: P0
- Domain: QA-Backend
- Goal: 覆盖 buyer API 主链路与异常链路。
- Inputs: `docs/miniapp-buyer-api-checklist.md`
- Scope: 认证、商品、收藏、浏览、意向、商家处理回流。
- Output: 集成测试套件。
- Dependencies: BE-006~BE-012, MB-003, MB-004
- Test Requirements: 主路径和错误路径都覆盖。
- Definition of Done: 集成测试可重复通过。
- Risks: 测试数据准备和清理复杂。

### Task QA-003
- Task ID: QA-003
- Task Title: 游客态/登录态切换测试
- Priority: P0
- Domain: QA-EndToEnd
- Goal: 验证权限边界正确。
- Inputs: `docs/miniapp-buyer-implementation-plan.md` §2/§4
- Scope: 游客收藏/浏览允许，游客意向提交拦截，登录后意向可提交。
- Output: 边界测试用例。
- Dependencies: FE-010, FE-013, BE-006, BE-009, BE-010, BE-011
- Test Requirements: 返回码与交互提示符合预期。
- Definition of Done: 边界回归通过。
- Risks: 客户端缓存导致伪登录态。

### Task QA-004
- Task ID: QA-004
- Task Title: device_id 合并测试
- Priority: P0
- Domain: QA-EndToEnd
- Goal: 验证合并幂等和多设备多账号规则。
- Inputs: `docs/miniapp-buyer-data-model.md` §5
- Scope: 同设备多账号、同账号多设备、重复 merge。
- Output: 合并专项测试。
- Dependencies: ST-001, ST-002, BE-006
- Test Requirements: 合并后数据不重不丢。
- Definition of Done: 关键合并场景全部通过。
- Risks: 并发 merge 竞态。

### Task QA-005
- Task ID: QA-005
- Task Title: 收藏/浏览记录测试
- Priority: P1
- Domain: QA-EndToEnd
- Goal: 验证收藏和浏览记录一致性。
- Inputs: `docs/miniapp-buyer-api-checklist.md` §5/§6
- Scope: 增删查、分页、去重、清空。
- Output: 收藏/浏览回归用例。
- Dependencies: BE-009, BE-010, FE-008, FE-009
- Test Requirements: 游客态与登录态均覆盖。
- Definition of Done: 用例可稳定回归。
- Risks: 时间相关断言不稳定。

### Task QA-006
- Task ID: QA-006
- Task Title: 购买意向闭环测试
- Priority: P0
- Domain: QA-EndToEnd
- Goal: 验证买家提交到商家处理全链路。
- Inputs: `docs/miniapp-buyer-implementation-plan.md` §4.3
- Scope: 买家提交、商家处理、买家回读。
- Output: 闭环 E2E 用例。
- Dependencies: FE-013, FE-014, MB-003, MB-004, BE-011
- Test Requirements: 每一步状态变化可验证。
- Definition of Done: 闭环链路一次通过率稳定。
- Risks: 双端联调节奏不同步。

### Task QA-007
- Task ID: QA-007
- Task Title: 商家意向处理回流测试
- Priority: P0
- Domain: QA-EndToEnd
- Goal: 验证商家动作后买家状态回读一致。
- Inputs: `docs/miniapp-buyer-api-checklist.md` §7/§9
- Scope: `contacted/close` 回流到买家列表与详情。
- Output: 回流一致性用例。
- Dependencies: MB-003, MB-004, BE-011, FE-014
- Test Requirements: 状态文本一致且可追踪。
- Definition of Done: 回流状态无差异。
- Risks: 缓存导致短暂不一致。

### Task QA-008
- Task ID: QA-008
- Task Title: 小程序页面交互测试
- Priority: P1
- Domain: QA-Frontend
- Goal: 验证核心页面交互与状态切换。
- Inputs: FE 页面任务交付
- Scope: 首页 -> 列表 -> 详情 -> 登录 -> 意向提交 -> 我的意向。
- Output: 页面交互测试脚本/手测清单。
- Dependencies: FE-003~FE-014
- Test Requirements: 核心交互无阻断。
- Definition of Done: 关键页面交互回归通过。
- Risks: 小程序自动化工具稳定性。

### Task QA-009
- Task ID: QA-009
- Task Title: 最小冒烟链路测试
- Priority: P0
- Domain: QA-Release
- Goal: 发布前快速验证最小闭环。
- Inputs: 全部 P0 任务输出
- Scope: 游客浏览、登录、提交意向、商家处理、买家回读。
- Output: 冒烟脚本与执行清单。
- Dependencies: 全部 P0 任务
- Test Requirements: 一条链路脚本完成关键验证。
- Definition of Done: 冒烟通过作为上线门禁。
- Risks: 微信环境依赖导致非代码失败。

---

## 最小可行开发顺序（推荐顺序）
1. 数据与认证底座：BE-001 ~ BE-006。
2. 状态与合并规则：ST-001 ~ ST-005。
3. 买家浏览与行为 API：BE-008、BE-009、BE-010、BE-011。
4. 买家前端核心链路：FE-001、FE-002、FE-003、FE-005、FE-006、FE-007、FE-010、FE-013、FE-014。
5. 商家意向配套：MB-003 -> MB-001/MB-002 -> MB-004。
6. 测试门禁：QA-001、QA-002、QA-003、QA-004、QA-006、QA-007、QA-009。
7. 增强任务：P1/P2（FE-004、FE-008、FE-009、FE-011、FE-012、BE-007、BE-012、QA-005、QA-008）。

## 哪 3 个任务最适合先做
1. BE-006（buyer auth + guest merge）
2. BE-008（buyer 商品列表/详情）
3. FE-002（请求层 + token/device_id 注入）

## 哪些任务必须串行
1. BE-001~BE-005 先于 BE-006~BE-011。
2. ST-003 必须先于 BE-011（意向冲突规则）。
3. MB-003 先于 MB-001/MB-002。
4. MB-004 必须在 MB-003 与 BE-011 完成后进行。
5. QA-009 必须在全部 P0 完成后执行。

## 哪些任务可以前后端并行
1. FE-003/FE-005/FE-006/FE-007 可在 API 契约冻结后并行开发（先 mock 再联调）。
2. FE-008/FE-009 与 BE-009/BE-010 可并行推进。
3. MB-001/MB-002 可在 MB-003 契约稳定后并行。
4. QA-001 单元测试可与各后端模块同步并行补齐。

## 当前文档是否已经足够支撑正式进入实现阶段
是。当前文档体系已覆盖：
1. 业务闭环与边界。
2. 接口契约与权限规则。
3. 数据模型与合并机制。
4. 状态流转与风控规则。
5. 测试门禁与发布冒烟路径。

可正式进入实现阶段。
