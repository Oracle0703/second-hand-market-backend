# 生产旧小程序图片 URL 兼容设计

## 背景

商品图片 `detail-v1` 发布清单要求生产本地文件存储继续通过后端 `/uploads/...` 受控访问，并将 `FILE_PUBLIC_BASE_URL` 显式置空，避免绕过上传文件安全检查。当前线上小程序尚未安排发版，且小程序发版需要审核，因此不能把“客户端支持相对 `/uploads/...` 图片地址”作为本次生产上线前置条件。

## 目标与非目标

| 项目 | 规则 |
| --- | --- |
| 目标 | 不要求小程序发版，生产 API 对旧小程序继续返回可直接展示的绝对图片 URL |
| 目标 | 数据库和回填工具仍保存相对 `/uploads/...`，保持文件访问统一走后端受控路由 |
| 目标 | 商品列表、商品详情、商家商品详情、上传成功响应等对外图片 URL 使用同一输出逻辑 |
| 非目标 | 不恢复 `FILE_PUBLIC_BASE_URL` 的旧语义 |
| 非目标 | 不绕过 `/uploads/*object_key` 的 MIME、路径和缓存头校验 |
| 非目标 | 不提前开启 `REQUIRE_DETAIL_V1_PRODUCT_IMAGES=true` |

## 方案

新增响应层配置 `PUBLIC_UPLOAD_BASE_URL`。该配置只影响 API 响应中的文件 URL 展示，不影响对象键、数据库 `files.url`、本地存储目录或 `/uploads` 读取路由。

| 配置状态 | API 响应 |
| --- | --- |
| `PUBLIC_UPLOAD_BASE_URL` 为空 | 返回相对 `/uploads/<object_key>` |
| `PUBLIC_UPLOAD_BASE_URL=https://market.meaningful.ink/uploads` | 返回 `https://market.meaningful.ink/uploads/<object_key>` |

生产第一阶段发布配置应调整为：

| 配置 | 值 |
| --- | --- |
| `FILE_PUBLIC_BASE_URL` | 空值 |
| `PUBLIC_UPLOAD_BASE_URL` | `https://market.meaningful.ink/uploads` |
| `REQUIRE_DETAIL_V1_PRODUCT_IMAGES` | `false` |
| `IMAGE_PROCESSOR_DRIVER` | `vips` |
| `AUTO_MIGRATE` | `false` |
| `SEED_DEFAULTS` | `false` |

## 校验规则

`PUBLIC_UPLOAD_BASE_URL` 非空时必须满足：

1. 使用 `http` 或 `https` scheme。
2. 不包含 query 或 fragment。
3. 路径必须规范到 `/uploads`，允许配置末尾带 `/`。
4. 最终输出时只拼接清洗后的对象键，不使用数据库中历史遗留的外部 URL。

## 数据流

| 阶段 | 行为 |
| --- | --- |
| 新上传 | 文件处理成功后数据库仍写入 `/uploads/<object_key>` |
| 商品查询 | 后端读取 `files.object_key`，经统一 URL 输出函数转换后返回 |
| 回填 | 回填工具仍把 `files.url` 更新为相对 `/uploads/<target_key>` |
| 静态访问 | Web/Nginx 和 API 继续通过 `/uploads/...` 路由访问真实文件 |

## 验收标准

1. 未配置 `PUBLIC_UPLOAD_BASE_URL` 时，现有测试继续返回相对 `/uploads/...`。
2. 配置 `PUBLIC_UPLOAD_BASE_URL=https://market.meaningful.ink/uploads` 时，上传响应、商家商品详情、买家商品列表和买家商品详情返回绝对 URL。
3. 数据库 `files.url` 不写入绝对域名。
4. 历史 `files.url` 即使保存过外部 URL，API 响应仍按 `object_key` 生成受控 URL。
5. 非法 `PUBLIC_UPLOAD_BASE_URL` 在服务启动时 fail closed。

