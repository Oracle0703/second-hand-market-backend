# 定制交付：管理员分配商户账号

2026-09-14。此流程替代历史自助注册、营业执照采集和商家审核流程。

## 使用流程

管理员登录后进入「商户管理 → 创建商户账号」，填写商户名称、联系人、电话、登录账号和初始密码。不需要营业执照，新商户直接开通并获得独立的默认商品分类。

开户和重置密码均提供「生成随机密码」「复制」按钮。生成器使用 Web Crypto 安全随机数与无偏采样，输出 20 位且保证包含大小写字母、数字和符号。手动输入密码须为 12–72 位可打印 ASCII，包含四类字符，不含空格。前后端独立校验，服务端只保存 bcrypt 哈希。按钮需要 HTTPS 或 localhost；无安全随机源时明确报错，不回退到 Math.random。

请在提交前复制初始密码并通过安全渠道交付。表单关闭/提交成功后清空密码，接口和日志不返回明文密码。商户首次登录只能访问账号设置、修改密码和退出；改密后重新登录才能进入业务页面。初始密码必须被替换，不能原样保存。

管理员在商户详情中可重置初始密码、启用/禁用账号。重置、禁用和商户改密会撤销该账号全部会话；访问令牌也实时检查会话及账号状态。操作保留审计记录。

## API 变化

- 新增 `POST /api/v1/admin/merchants`：管理员开户，参数 `merchant_name/contact_name/phone/username/password`。
- 新增 `PUT /api/v1/admin/merchants/:id/password`：管理员重置该商户 OWNER 账号的密码，参数 `password`。
- 新增 `PUT /api/v1/admin/merchants/:id/status`：管理员设置 OWNER 账号状态，参数 `status=ACTIVE|DISABLED`。
- 下线 `POST /api/v1/auth/register`、`POST /api/v1/merchant/reapply`、商户 approve/reject 接口，返回 404。
- 登录响应的 `user.must_change_password` 表示必须改密。后端独立执行限制，不能靠篡改浏览器状态绕过。
- `/files/presign` 仅接受已认证管理员或已开通商户的 `PRODUCT_IMAGE`；执照的预签名、上传、确认均关闭，包括历史 pending 记录。
- 旧执照文件仅管理员鉴权后可读取，匿名请求返回 404。历史字段和文件保留，不自动删除。

## 上线顺序与历史账号

1. 备份数据库，在维护窗口使用独立迁移凭据执行新增迁移。API 不自动改表。
2. 从 `backend/` 执行 `go run ./scripts/migrate --migration 0010_merchant_initial_password`，使用项目现有 DB_DRIVER、DB_DSN 及维护授权配置。迁移仅增加 `merchant_accounts.must_change_password`，默认 FALSE。
3. 同一维护窗口更新后端和管理端。先验证管理员登录、新商户开户、首次改密、商品访问、重置、禁用、日志记录。
4. 已开通历史账号继续可用，不批量重置密码。历史 PENDING/REJECTED 商户继续受限；如需启用，先单独核对资料并通过受控数据库维护把其 review_status 调整为 APPROVED（本变更不静默批量批准历史商户）。
5. 旧字段、执照文件不在本次物理清理范围内；清理需另行确定留存策略。旧执照静态直连/CDN 路径必须保持关闭。

回退代码时保留新增字段即可，但不要回退到开放自助注册和匿名执照访问的版本。启用首次改密限制的账号必须继续受到后端限制。

本次未引入多实例共享登录限流或 pending 文件定期清理，这些仍属于后续安全整改项。
