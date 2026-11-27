# 通用事件上报与查询

微信和抖音小程序共用一个上报入口。首次采集新的业务动作时，在业务触发处接入 SDK，随业务版本发布。第一条有效事件入库后，商家和管理员即可在「事件查询」中选择它。已上报的数据可以直接换条件、换分组查询，无需另建报表。

## 接入一个新事件

例如，业务约定 `order_created` 表示下单接口已成功返回，可在业务成功回调中调用 SDK。若要分析用户尝试下单，应在按钮点击时发送另一个名称，如 `checkout_click`。示例只说明接入位置和字段。

```ts
import { trackEvent } from '@/services/analytics'

trackEvent('order_created', {
  source: 'checkout',
  properties: {
    amount_cent: 129900,
    delivery: 'pickup',
    is_first: true,
    campaign: '周末门店试用'
  }
})
```

`trackEvent` 返回 `true` 表示事件已进入本机队列。可选参数还有 `page` 和 `productId`。SDK 在可用时自动读取当前页面路径并排除查询串。事件 ID 与发生时间在首次入队时生成，补报沿用原值。事件或属性不符合约定时返回 `false`。自定义场景可调用同一函数提供不同属性，例如只为售后事件附加 `reason`。沿用旧调用形式的按钮也可直接使用 `trackClick('delivery_option_selected', page, source, productId)`，与 `trackEvent` 在同一处查询。

| 字段 | 约定 |
| --- | --- |
| 事件名称、来源 | 小写字母起头，使用字母、数字和分隔单词的下划线，最长 64 字符 |
| 页面 | 路由路径最长 128 字符，例如 `pages/checkout/result`。页面曝光需要路径，其他事件可在无页面上下文时上报 |
| 属性 | 每条最多 16 个键，键名沿用事件命名方式，最长 48 字符。值支持字符串、有限数字、布尔值。字符串最长 128 个 Unicode 码点，数值绝对值不能超过 JavaScript 安全整数上限 |
| 数据选择 | 使用履约方式、业务状态等分析维度。凭据和个人联系方式等敏感键会被拒绝；属性值应使用业务分类，避免填入个人资料、用户输入原文或完整 URL |

### 直接使用 HTTP

沿用 `POST /api/v1/buyer/analytics/events?merchant_no=...`，发送 `X-Device-Id`、`Content-Type: application/json`。上报方应生成稳定的事件 ID，重试同一条事件时沿用 ID。

```json
{
  "platform": "tt",
  "events": [{
    "event_id": "demo_order_0001",
    "name": "order_created",
    "page": "pages/checkout/result",
    "source": "checkout",
    "occurred_at": "2026-11-28T10:30:00+08:00",
    "properties": { "amount_cent": 129900, "delivery": "pickup", "is_first": true }
  }]
}
```

实际接入请使用事件真实发生时间。每批最多 50 条。标准响应的 `data.received` 为收到的条数，`discarded` 为丢弃的条数，`rejected` 中的 `index` 从 0 开始，`reason` 可用于定位不合规的事件。同批中其余有效记录仍会入库。同一商户重复发送同一事件 ID 只计一次。客户端事件可用于行为分析，订单履约、支付与财务核对应使用相应业务系统的记录。

## 在后台组合查询

例如，查某天抖音用户在门店自提的高金额下单事件：

1. 在「事件查询」选择北京时间日期范围、平台「抖音」与事件 `order_created`。
2. 添加属性条件 `delivery` 等于字符串 `pickup`，并添加 `amount_cent` 大于等于数字 `100000`。各条件同时生效。
3. 按 `campaign` 属性或来源分组，点击「查询」，查看每组的事件数和去重设备数。展开明细可核对事件 ID、发生时间和上报属性。本例金额以分为单位。
4. 收藏查询结果的页面链接，可再次打开同一组条件。其他人打开分享链接时，数据范围仍由其登录身份决定。

商家只能查询当前商户。管理员可查询全平台，也可限定单个商户。去重设备按商户、平台和匿名安装标识计算；同一人使用不同小程序或重新安装可能对应多个设备标识。事件查询统计所有匹配事件的设备，原有「小程序统计」概览的设备指标仍按页面曝光计算。

| 接口 | 用途 |
| --- | --- |
| `GET /api/v1/merchant/analytics/explore` | 当前商户的事件总数、去重设备、分组和分页明细 |
| `GET /api/v1/admin/analytics/explore` | 管理员全局查询，同一请求可带 `merchant_id` |
| `GET /api/v1/merchant/analytics/dimensions`、`GET /api/v1/admin/analytics/dimensions` | 在当前权限和日期范围内发现已上报的事件名称、页面、来源和属性键。每类返回最多 200 个候选，也可以直接输入键名 |

查询日期 `start_date`、`end_date` 使用 `YYYY-MM-DD`，含首尾两天，最多 93 天。可组合 `platform`、`name`、`page`、`source`、`product_id` 和属性条件。`filters` 是 URL 编码的 JSON 数组，最多 8 项。例如：

```json
[
  { "key": "delivery", "op": "eq", "value": "pickup" },
  { "key": "amount_cent", "op": "gte", "value": 100000 }
]
```

`eq`、`ne` 按相同类型比较，缺少属性时均不命中。`gte`、`lte` 用于数字，`contains` 用于字符串。`group_by` 支持 `name`、`date`、`platform`、`page`、`source`、`product_id`、管理员可用的 `merchant_id`，以及 `prop.<属性名>`。

结果的 `total` 和 `devices` 对应所有匹配记录。`groups_total` 是分组总数，`groups` 按事件数返回前 100 组。属性分组的 `value_type` 可区分数字 `1` 和字符串 `"1"`。`events` 按发生时间倒序分页返回，`page_no` 从 1 开始，`page_size` 默认 20、最大 100。

## 升级

### 接入独立的多项目埋点平台

在独立平台管理后台创建「二手家居」项目。构建本仓小程序时同时提供以下变量，已有的曝光、商品、收藏和联系事件会额外上报到该项目。独立平台项目可由获授权的运营人员查询，原商家报表继续按原有商户范围统计。

| 构建变量 | 值 |
| --- | --- |
| `TARO_APP_ANALYTICS_ENDPOINT` | 独立平台的完整采集地址，如 `https://analytics.example.com/api/events` |
| `TARO_APP_ANALYTICS_PROJECT_KEY` | 管理后台为本项目生成的写入标识 |

微信、抖音正式包需要将采集域名加入 request 合法域名；本机模拟器可以使用开发环境地址。商品 ID 映射为独立平台的 `properties.product_id`。未配置独立平台的构建继续使用本仓的采集服务。

`miniapp/src/vendor/event-analytics/` 中的 `index.js` 与 `index.d.ts` 是独立仓库 `event-analytics/sdk/miniapp/` 对应版本的原样副本。升级 SDK 时一并更新两份文件，再运行小程序测试与双端构建。

### 独立后台的双项目验收

2026-11-28，在同一后台从「二手家居」切换到「转盘小程序」。两个测试项目的初始事件数均为 0。

| 项目 | 模拟器操作 | 后台结果 |
| --- | --- | --- |
| 二手家居（微信） | 从首页打开书桌详情并点击收藏 | 4 条事件、1 台设备：首页和详情曝光各 1 次，商品点击、收藏各 1 次 |
| 转盘示例（抖音） | 打开并重编译示例，点击「转一次」，转盘停在「喝杯茶」 | 4 条事件、1 台设备：曝光 2 次、开始转动 1 次、转动结果 1 次 |

家具 3 批、转盘 4 批上报均返回 HTTP 200，丢弃 0 条。两个模拟器的 8 个事件 ID 都与所属项目的数据库记录及后台明细一致，出现在另一项目中的数量为 0。切到转盘项目，按 `wheel_result`、文本 `choice = "tea"`、数值 `segment >= 5` 筛出 1 条，刷新查询链接后仍能复现。家具原商户接口同时接收到了对应的 4 条事件。

[家居项目后台截图](images/standalone-analytics/furniture-query.jpg) · [转盘项目筛选截图](images/standalone-analytics/wheel-query.jpg)

### 本仓原有采集服务的数据库迁移

在目标 MySQL 的 `backend/` 运行已存在的 `0012_miniapp_analytics` 迁移后，再运行 `go run ./scripts/migrate --migration 0013_analytics_properties`。已有 0012 的环境只需执行 0013。随后依次发布 API、后台和小程序。历史曝光、点击可直接查询，旧行的属性显示为空对象。
