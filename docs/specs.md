# 产品与系统规格

> 2026-09-14 定制交付调整：自助注册、执照采集与审核接口已下线，采用管理员开户和首次改密。新增接口、字段及历史数据处理以 [商户账号管理](operations/custom-merchant-accounts.md) 为准；下文原注册/审核描述仅用于历史兼容追溯。

更新时间：2026-09-12
状态：当前实现基线

## 1. 目标与边界

系统服务于单店或多商户二手商品经营场景：管理员负责准入，商家负责供给、库存和线下成交，买家通过微信/抖音小程序浏览并联系商家。

当前不提供支付、退款、售后、购物车、IM 和履约。购买意向是线索，轻量订单是商家侧成交记录，两者不自动互转。

## 2. 通用约定

- API 前缀：`/api/v1`。
- 响应结构：`{ code, message, request_id, data }`。
- 分页：`page` 默认 1；`page_size` 默认 20、最大 100。
- 认证：`Authorization: Bearer <access_token>`。
- 买家/游客请求按接口要求携带 `merchant_no`；游客资产还需 `X-Device-Id`。
- 金额使用整数分；时间使用 RFC 3339；主键使用正整数 ID。
- 高风险写接口支持或要求 `Idempotency-Key`，同 scope 下请求体变化返回冲突。

## 3. 商家准入与账户

1. 注册成功创建 `PENDING` 商家和 OWNER 账号。
2. `PENDING/REJECTED` 商家可登录，但 token scope 为 `onboarding`。
3. `onboarding` 只允许查看资料、驳回重提及上传入驻资质。
4. 管理员仅能对 `PENDING` 商家审核通过或驳回。
5. `APPROVED` 商家登录获得 `full` scope。
6. 管理员、商家或买家账号被禁用后不能继续建立有效会话。
7. 管理员只能通过 `backend/scripts/bootstrap_admin` 显式创建；服务启动不创建默认账号。

## 4. 分类规格

1. 商品分类是商户自有的两级树，按 `merchant_id` 隔离。
2. 一级分类无父级；二级分类必须引用同商户一级分类。
3. 同一商户、同一父级下的分类名称必须唯一。
4. 商品只能引用当前商户已启用的二级分类。
5. 被商品引用的二级分类不可删除；删除一级分类时，如任一子分类被引用，整次操作失败。
6. 新商家从系统默认模板复制分类；历史商家通过显式 backfill 脚本迁移。

## 5. 商品与库存规格

### 5.1 状态

| 状态 | 含义 | 允许的主要动作 |
| --- | --- | --- |
| `DRAFT` | 草稿 | 编辑、补/减库存、上架、删除 |
| `ON_SHELF` | 在售 | 编辑描述/图片、下架、创建订单、调整库存、线下售罄 |
| `LOCKED` | 有活动订单 | 完成或关闭对应订单 |
| `OFF_SHELF` | 下架 | 编辑、补/减库存、重新上架、删除 |
| `SOLD` | 库存为 0 的售罄状态 | 仅允许 `INCREASE`，补货后转 `OFF_SHELF` |

商品状态不包含 `CLOSED`。`SOLD` 可通过补库存恢复，不是永久终态。

### 5.2 上架与编辑

1. 上架要求库存大于预占库存、没有活动订单、至少一张有效商品图片，并使用当前商户已启用二级分类。
2. `DRAFT/OFF_SHELF` 可编辑完整业务字段。
3. `ON_SHELF` 只允许编辑描述和图片。
4. `LOCKED/SOLD` 不允许常规编辑。
5. 删除只允许未被订单等业务记录约束的 `DRAFT/OFF_SHELF` 商品。

### 5.3 库存调整

1. `INCREASE` 增加库存；`SOLD` 增加后转为 `OFF_SHELF`。
2. `DECREASE` 不能使 `stock < reserved_stock`；在售商品减至 0 时转为 `OFF_SHELF`。
3. `MARK_SOLD` 记录线下售出，不创建订单、不计入订单销售额；清零时转为 `SOLD`。
4. `LOCKED` 商品拒绝手动库存调整。
5. 每次调整写入 `product_stock_adjustments`，记录前后库存、前后状态、原因和操作人。

## 6. 轻量订单规格

1. 订单只能由当前商品所属商家创建，商品必须为 `ON_SHELF`。
2. 当前请求模型每笔订单数量为 1；应用层同一商品只允许一笔活动订单。
3. 创建在一个事务中：新增 `CREATED` 订单、增加 `reserved_stock`、设置 `active_order_id`，商品进入 `LOCKED`。
4. 完成在一个事务中：订单进入 `COMPLETED`，同时 `stock -= 1`、`reserved_stock -= 1`、清空活动订单；剩余库存大于 0 则商品回到 `ON_SHELF`，否则进入 `SOLD`。
5. 关闭在一个事务中：订单进入 `CLOSED`，释放预占、清空活动订单，商品进入 `OFF_SHELF`。
6. 完成/关闭重复调用到同一目标状态可幂等返回；跨终态调用拒绝。

## 7. 买家域规格

1. 公开商品列表仅返回指定商户的 `ON_SHELF` 商品；详情按现有公开规则处理不可售状态。
2. 收藏和浏览历史同时支持游客设备和登录买家，数据始终按商户隔离。
3. 登录成功后合并未合并的游客资产：收藏并集去重，历史保留最新时间并累加次数。
4. 后端统一登录入口支持微信和抖音 provider；生产禁止 `mock`。
5. 同一买家、同一商品只允许一条未关闭意向；意向不会锁库存。
6. 意向状态为 `NEW/CONTACTED/CLOSED`，买家不能读取商家内部备注和处理人。
7. 当前小程序详情页隐藏意向入口，以直接拨打商家电话为主；后端意向接口和管理端处理页面继续保留。
8. 小程序以 `merchant_no` 固定商户入口；旧客户端缺失时可短期使用 `BUYER_DEFAULT_MERCHANT_NO` 兼容。

## 8. 小程序设备能力

1. 电话使用 `Taro.makePhoneCall`，仅由用户点击触发。
2. 抖音端调用前查询平台隐私设置；平台要求时由 `tt.onNeedPrivacyAuthorization` 回调绑定本次真实点击的 `buttonId`。
3. 不用本地缓存代替平台授权结果。用户已在平台同意后，后续点击应直接调用；授权被撤回时重新进入平台流程。
4. 门店导航使用 `Taro.openLocation` 和固定商户坐标；当前没有持续定位业务，因此不主动调用位置持续更新 API。

## 9. 文件与图片规格

1. 支持 JPEG、PNG、WebP、HEIC、HEIF 静态图；拒绝 MOV 和内容/MIME 不一致文件。
2. 原图限制 40 MB，服务端处理目标 20 MB；商品展示图进一步生成单一 `detail-v1` JPEG。
3. 文件名和扩展名不可信，必须解码验证并重新编码后落盘。
4. 本地存储的 `FILE_PUBLIC_BASE_URL` 必须为空，文件由 `/uploads/*` 受控路由返回。
5. `REQUIRE_DETAIL_V1_PRODUCT_IMAGES` 在生产必须显式配置；历史数据切换严格模式前必须完成回填与谓词检查。

## 10. 非功能与安全要求

- 生产/远程开发不允许自动迁移或 seed。
- 生产 JWT 密钥至少 32 字节、两枚不同，并拒绝已知示例值。
- 远程开发数据库必须校验地址、库名、服务 UUID 和账号。
- 数据隔离同时依赖鉴权上下文与查询条件，跨商户访问返回无权限或不存在。
- 关键状态写入操作日志或领域事件，保留 request ID。
- 当前内存限流只适用于单实例；扩容前需迁移到共享存储。

## 11. 验收基线

发布候选必须至少通过：

1. `make test` 和 `go vet ./...`。
2. `cd frontend && npm run test && npm run build`。
3. `cd miniapp && npm test && npm run build:weapp && npm run build:tt`。
4. 数据库迁移 preflight/postflight、图片管线验收及目标环境 smoke。
5. 微信/抖音真实凭据和真机上的登录、图片、电话、导航、分享与弱网回归。

完整门禁见 [release-readiness.md](release-readiness.md) 和 [miniapp-release-readiness.md](miniapp-release-readiness.md)。
