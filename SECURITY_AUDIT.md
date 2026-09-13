# 安全审计报告

> 2026-09-14 分支整改更新：营业执照采集及自助注册已关闭；历史执照仅管理员鉴权读取，新上传对象名使用 UUID。Access Token 已校验 session 撤销及管理员/商户实时状态，改密和重置会撤销旧会话。JWT 已限定 HS256，并为 JSON 请求增加 2 MiB 流式上限。下文保留 09-12 审计历史；登录限流、限流器容量治理及 pending 文件过期清理仍未完成。新增开户和首次改密流程见 [操作说明](docs/operations/custom-merchant-accounts.md)。

审计范围：后端 API、文件上传与公开文件读取、认证会话、买家游客数据、前端令牌存储、部署配置。

审计时间：2026-09-12

## 结论

项目已有较完整的商户隔离、上传 MIME 检查、路径穿越防护、生产 JWT 密钥校验和数据库启动保护。但在生产部署前仍应优先修复以下问题：公开执照文件、旧 Access Token 失效机制、管理员/商户登录限流，以及进程内限流器的内存增长。

## 高危问题

### 1. 商家营业执照可匿名读取，文件名可预测

`/uploads/*object_key` 对匿名请求开放，处理逻辑只校验路径和图片 MIME，没有根据 `BizType` 或文件记录做访问控制。

相关位置：

- `backend/internal/app/server.go:101`
- `backend/internal/app/file_handlers.go:323`
- `backend/internal/app/file_handlers.go:55-100`
- `backend/internal/common/idgen.go:11`

营业执照对象路径使用时间戳加递增序列生成。攻击者可通过一次上传获知当前编号，再枚举相邻编号，尝试读取同一时间段创建的其他执照。执照通常包含企业信息、联系人和证件资料，属于敏感数据。

修复建议：

1. 只允许商品图片匿名访问，营业执照要求管理员鉴权。
2. 或为营业执照生成短期签名 URL。
3. 对象名改用 UUID/密码学随机值，不使用时间戳和递增序列。
4. 不要仅根据文件扩展名和路径决定公开权限。

### 2. 退出登录和账号禁用后，旧 Access Token 仍可继续使用

认证中间件只校验 JWT 签名和过期时间，没有校验 `sid` 对应的 `AuthSession` 是否已撤销。退出登录虽然写入 `revoked_at`，但旧 Access Token 仍可使用到 Access Token 自身过期（默认两小时）。审核状态、账号状态和角色变化也不会立即反映到已经签发的 Access Token。

相关位置：

- `backend/internal/middleware/auth.go:13-33`
- `backend/internal/app/auth_handlers.go:258-269`
- `backend/internal/auth/jwt.go:44-53`

修复建议：

1. 每次认证请求查询 `sid` 对应的 session，拒绝已撤销或过期 session。
2. 对敏感操作实时查询账号状态和当前权限。
3. 缩短 Access Token 生命周期，并保留 Refresh Token 轮换机制。

### 3. 管理员和商户登录接口没有失败限流

`/api/v1/auth/login` 对管理员和商户登录没有 IP、用户名或账号维度的失败次数限制。bcrypt 校验成本较高，攻击者既可以进行密码爆破，也可以持续消耗服务 CPU。

相关位置：

- `backend/internal/app/server.go:168-170`
- `backend/internal/app/auth_handlers.go:73-145`

现有 `checkRateLimit` 只用于买家小程序登录和部分买家操作。

修复建议：

1. 增加 IP、用户名和账号维度的失败计数及指数退避。
2. 管理员登录增加 MFA，并使用更严格的限制。
3. 多实例部署时使用 Redis 等共享限流存储。

## 中高危问题

### 4. 进程内限流器会无限保留攻击者构造的 key

限流器使用 map 保存 bucket，只有某个 bucket 再次被访问时才会清理旧时间戳，过期 bucket 本身不会从 map 删除。游客请求可自行设置 `X-Device-Id`，攻击者持续发送随机设备 ID 会不断创建新的 key，长期运行可能导致内存耗尽。

相关位置：

- `backend/internal/app/rate_limiter.go:9-42`
- `backend/internal/app/buyer_handlers.go:442-447`
- `backend/internal/app/buyer_handlers.go:721-735`
- `backend/internal/app/buyer_handlers.go:808-818`

修复建议：

1. 使用带 TTL 的 Redis/缓存实现。
2. 如果继续使用内存实现，增加定期清理和最大 bucket 数量。
3. 对游客设备 ID 增加格式约束，并同时按 IP 限流。

## 中等风险

### 5. JSON 请求没有统一大小上限

`bindJSON` 直接调用 `ShouldBindJSON`，登录、注册等公开接口没有统一的请求体上限。超大 JSON 可能造成内存和 CPU 压力。

相关位置：

- `backend/internal/app/server.go:268-272`

建议在全局中间件或 `bindJSON` 中使用 `http.MaxBytesReader`，并为不同接口设置合理上限。

### 6. 文件预签名记录没有真正的过期机制

预签名响应包含 15 分钟的 `expire_at`，但文件表没有过期字段，服务端也没有检查过期时间。匿名客户端可以无限创建 pending 文件记录，长期会造成数据库垃圾数据堆积。

相关位置：

- `backend/internal/app/file_handlers.go:55-110`
- `backend/internal/model/models.go:217-234`

建议增加 `expires_at`，上传和确认时校验，并定期清理过期 pending 记录；同时给 `/files/presign` 增加 IP/账号限流。

### 7. JWT 解析未显式限制签名算法

解析函数没有检查 `t.Method` 是否为 HS256。当前密钥保护下不一定能直接利用，但建议显式拒绝非预期算法，避免未来改动引入算法混淆问题。

相关位置：

- `backend/internal/auth/jwt.go:44-65`

## 验证记录

- 已完成源码级安全审计和路由/权限边界检查。
- `go test ./internal/auth` 可运行通过。
- 完整后端测试在当前环境因沙箱禁止 `httptest` 监听本地端口而中断，未发现测试断言本身失败。

## 建议的修复顺序

1. 私有化营业执照访问并更换随机对象名。
2. 让 Access Token 受 session 撤销和实时权限控制。
3. 为管理员/商户登录增加限流和 MFA。
4. 替换或修复进程内限流器的 key 清理机制。
5. 增加请求体限制和 pending 上传清理。
