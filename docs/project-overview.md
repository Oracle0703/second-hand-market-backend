# 项目总体说明

更新时间：2026-09-12
状态：当前实现基线

## 1. 项目定位

本项目是二手商品经营与展示平台，采用单仓管理三个运行端：

- `backend/`：Go 1.22、Gin、GORM API，支持 MySQL 和本地 SQLite。
- `frontend/`：React、TypeScript、Vite 管理端，供管理员和商家使用。
- `miniapp/`：Taro 买家小程序，同时构建微信和抖音版本。

系统围绕“商家入驻 -> 分类与商品经营 -> 买家发现与联系 -> 轻量订单成交”形成闭环，不包含在线支付和履约。

## 2. 当前范围

### 管理员与商家

1. 商家注册、审核、驳回重提和受限登录。
2. 管理员审核商家、查看全局操作日志。
3. 商户自有一级/二级分类管理；新商户复制默认分类，历史商户通过显式脚本回填。
4. 商品创建、编辑、删除、上下架、库存调整、售罄和图片管理。
5. 轻量订单创建、完成和关闭，与库存预占、释放、扣减事务联动。
6. 买家意向查询、标记已联系和关闭。
7. 商家仪表盘、账户设置和操作日志。

### 买家小程序

1. 按商户入口浏览分类、商品列表和商品详情。
2. 游客收藏、浏览历史；登录后合并到买家账号。
3. 微信、抖音小程序登录，后端支持 `mock/real/disabled` 环境模式。
4. 买家意向 API 和状态回读仍保留；当前小程序商品详情以直接拨打商家电话为主要联系入口，意向提交 UI 暂时隐藏。
5. 门店导航调用 `Taro.openLocation`，使用配置的固定门店坐标。
6. 电话调用 `Taro.makePhoneCall`；抖音端通过平台隐私授权回调和用户真实点击完成授权，不在本地伪造或永久缓存平台授权状态。

### 图片与文件

1. `presign -> upload -> confirm` 上传流程。
2. 原图最大 40 MB；`vips` 支持 JPEG、PNG、WebP、HEIC、HEIF，并执行内容校验与重新编码。
3. 商品图按 `detail-v1` 生成 JPEG 展示文件；本地存储通过后端 `/uploads/*` 受控交付。
4. 历史图片提供显式回填账本、dry-run、应用和延迟清理流程。

## 3. 明确非范围

- 在线支付、退款、发票、售后和履约。
- 购物车、IM 聊天、推荐算法和营销体系。
- 商家子账号的完整管理界面。
- 自动执行生产迁移、自动创建管理员或自动写入 seed。
- 多实例共享限流、对象存储和生产级可观测平台。

## 4. 角色与权限

| 角色 | 当前能力 | 主要限制 |
| --- | --- | --- |
| `SUPER_ADMIN` | 商家审核、全局日志 | 不直接编辑商家商品 |
| `ADMIN` | 商家审核、全局日志 | 不创建管理员，不编辑商家商品 |
| `MERCHANT/OWNER` | 入驻；审核通过后管理分类、商品、订单、意向、账户和日志 | 只能访问本商户数据 |
| `MERCHANT/STAFF` | 模型预留 | 当前不开放完整创建与登录流程 |
| `BUYER` | 登录、收藏/历史合并、意向提交与查询 | 只能访问自己的账号数据和指定商户数据 |
| 游客设备 | 浏览、收藏和浏览历史 | 不能提交或查询买家意向 |

商家 `PENDING/REJECTED` 登录后获得 `onboarding` scope，只能访问资料和重提相关接口；`APPROVED` 获得 `full` scope。禁用账号拒绝登录。

## 5. 核心状态与流程

### 商家审核

```text
PENDING -> APPROVED
PENDING -> REJECTED
REJECTED -> PENDING
```

### 商品与库存

商品状态只有五种：

```text
DRAFT --> ON_SHELF
ON_SHELF --> OFF_SHELF
OFF_SHELF --> ON_SHELF
ON_SHELF --> LOCKED
LOCKED --> OFF_SHELF（订单关闭）
LOCKED --> ON_SHELF / SOLD（订单完成后按剩余库存决定）

SOLD --INCREASE--> OFF_SHELF
```

- 商品没有 `CLOSED` 状态；订单和购买意向仍有 `CLOSED`。
- 创建订单只接受 `ON_SHELF` 商品，当前每笔订单数量固定为 1。
- 创建订单增加 `reserved_stock`，设置 `active_order_id`，商品进入 `LOCKED`。
- 完成订单同时扣减 `stock` 和 `reserved_stock`；剩余库存大于 0 时回到 `ON_SHELF`，为 0 时进入 `SOLD`。
- 关闭订单释放预占库存，并把商品置为 `OFF_SHELF`。
- 当前应用层仍限制同一商品同时只有一笔活动订单。

### 手动库存调整

| 类型 | 规则 |
| --- | --- |
| `INCREASE` | 增加库存；`SOLD` 补货后进入 `OFF_SHELF`，由商家确认后再上架 |
| `DECREASE` | 减少库存，不得小于预占库存；在售商品扣至 0 时下架 |
| `MARK_SOLD` | 记录线下售出；可按数量扣减，也可用 `all_remaining` 清零并进入 `SOLD` |

`LOCKED` 商品不能手动调库存。

### 订单与意向

```text
订单：CREATED -> COMPLETED | CLOSED
意向：NEW -> CONTACTED -> CLOSED
            \\------------> CLOSED
```

买家意向与订单是独立领域：提交意向不会锁定库存；创建商家订单才会预占库存。

## 6. 技术与数据边界

1. 后端当前是模块化程度较轻的单体，HTTP 编排主要位于 `backend/internal/app/*_handlers.go`。
2. 状态转移表集中在 `backend/internal/stateflow/`，复杂库存不变量由事务 handler 和数据库约束共同保证。
3. 数据库结构只能通过 `backend/migrations/` 和显式迁移命令推进；长驻 API 不执行 DDL 或 seed。
4. 管理端与小程序通过 `/api/v1` 访问同一后端，但使用独立角色、scope 和商户隔离规则。
5. 买家公开读取以及游客资产都必须携带 `merchant_no`；`BUYER_DEFAULT_MERCHANT_NO` 仅用于旧版本过渡。

## 7. 当前发布边界

代码回归通过不等于已发布。生产上线仍需完成数据库身份确认、迁移前后检查、真实平台凭据、微信/抖音真机验证、图片回填门禁和网关配置。具体见 [发布清单](release-readiness.md) 与 [小程序发布清单](miniapp-release-readiness.md)。
