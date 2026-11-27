# 独立平台 SDK

`index.js` 和 `index.d.ts` 是独立平台仓库 `event-analytics/sdk/miniapp/` 中同名文件的原样副本，对应可分发包 `@event-analytics/miniapp` 0.1.0。升级时从平台仓复制这两个文件，核对 SHA-256，并运行小程序测试。当前 JavaScript SHA-256 为 `6f402bbbaf99c445c1ed3abcfff6edddbfaadaed6b0a2f3d39f694b907065e1d`。

构建前同时配置 `TARO_APP_ANALYTICS_ENDPOINT`（完整的 HTTPS `/api/events` 地址）和 `TARO_APP_ANALYTICS_PROJECT_KEY`（家具项目的写入标识）。开发模式允许指定本机测试服务。缺少两项配置时只发送原商户报表，配置齐全时同一个业务动作还会独立送到项目写入接口。项目 SDK 负责自己的队列与重试。

商品编号在独立平台中位于 `properties.product_id`。若自定义事件已使用全部 16 个属性且没有商品编号字段，保留调用方的属性。商户报表继续接收原有顶层的商品编号。
