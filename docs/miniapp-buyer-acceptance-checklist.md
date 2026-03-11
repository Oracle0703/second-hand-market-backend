# 买家小程序第二阶段验收清单（真机联调 + 页面级 E2E）

更新时间：2026-03-11

## 1. 自动化验收（可直接执行）

## 1.1 基础回归命令
1. 后端测试：`cd backend && GOMODCACHE=$(pwd)/.cache/go/mod GOCACHE=$(pwd)/.cache/go/build go test ./...`
2. 商家后台前端：`cd frontend && npm run test && npm run build`
3. 小程序单测：`cd miniapp && npm run test`
4. 小程序构建：`cd miniapp && npm run build:weapp`

## 1.2 买家页面链路 smoke（新增）

```bash
# 终端 A：启动后端
cd backend
mkdir -p .cache/go/mod .cache/go/build
GOMODCACHE=$(pwd)/.cache/go/mod GOCACHE=$(pwd)/.cache/go/build go run ./cmd/server

# 终端 B：执行页面链路 smoke
cd ..
API_BASE_URL=http://localhost:8080/api/v1 node scripts/smoke-miniapp-page-e2e.mjs
```

可选变量：
1. `API_BASE_URL`：API 网关地址。
2. `BUYER_DEVICE_ID`：指定固定设备 ID；不传则脚本自动生成。
3. `BUYER_WECHAT_LOGIN_CODE`：当后端为 `BUYER_WECHAT_LOGIN_MODE=real` 时，传入真机 `wx.login` 获取的临时 code。

## 1.3 自动化覆盖矩阵

| 页面/链路 | 覆盖方式 | 对应动作 |
| --- | --- | --- |
| 首页 -> 商品列表 -> 商品详情 | 自动化 | `/buyer/products` + `/buyer/products?category_id=...` + `/buyer/products/:id` |
| 游客收藏商品 | 自动化 | `POST /buyer/favorites` + `GET /buyer/favorites` |
| 游客浏览记录生成 | 自动化 | `POST /buyer/histories/views` + `GET /buyer/histories` |
| 登录后 guest merge | 自动化 | `POST /buyer/auth/wechat-login` + `POST /buyer/guest/merge` + `GET /buyer/me/summary` |
| 登录用户提交购买意向 | 自动化 | `POST /buyer/intents` |
| 我的意向列表查看状态 | 自动化 | `GET /buyer/intents`（校验 `NEW/处理中`） |
| 商家处理后买家状态回读 | 自动化 | 商家 `contacted/close` + 买家 `GET /buyer/intents/:id`、`GET /buyer/intents?status=CLOSED` |

---

## 2. 真机手工验收（不可完全自动化部分）

说明：以下步骤必须在微信开发者工具 + 真机预览执行，不能只看 API。

## 2.1 分享落地（高风险）
1. 打开商品详情页，点击右上角“转发”。
2. 将商品分享给测试号或文件传输助手。
3. 在另一台手机点击分享卡片。
4. 预期：进入 `pages/product/detail/index?id=...`，商品信息正确，页面无空白。

## 2.2 真机登录交互
1. 游客进入详情页点击“提交意向”。
2. 预期：跳转登录页而不是直接提交。
3. 点击“授权登录”，观察授权弹窗与回跳路径。
4. 预期：登录后回到原目标页（意向页或我的页）。

## 2.3 guest merge 页面表现
1. 游客先做收藏与浏览记录。
2. 登录后进入“收藏页”和“浏览记录页”。
3. 预期：登录前游客数据在登录后可见，且无重复条目。

## 2.4 多机型兼容（至少 2 台）
1. iOS 真机完成“首页 -> 详情 -> 收藏 -> 登录 -> 提交意向”。
2. Android 真机重复同样动作。
3. 预期：页面不抖动、不白屏、按钮可点击、请求成功。

## 2.5 弱网容错
1. 真机切到弱网（或限速代理）后打开详情页并尝试收藏/提交意向。
2. 预期：加载中与错误提示可见，不出现不可恢复卡死。

---

## 3. 结果记录模板（执行时填写）

| 检查项 | 环境 | 结果 | 证据 |
| --- | --- | --- | --- |
| smoke-miniapp-page-e2e | local/test | PASS/FAIL | 终端日志 |
| 分享落地 | 真机 iOS/Android | PASS/FAIL | 截图/录屏 |
| 游客拦截与登录回跳 | 真机 iOS/Android | PASS/FAIL | 截图/录屏 |
| guest merge 页面表现 | 真机 iOS/Android | PASS/FAIL | 截图/录屏 |
| 状态回读（已联系/已关闭） | 真机 iOS/Android | PASS/FAIL | 截图/录屏 |

---

## 4. 当前边界说明
1. 当前后端已支持 `BUYER_WECHAT_LOGIN_MODE=mock/real` 两种模式。
2. 在 `mock` 模式下可做流程自动化回归，但不能证明真实 `code2session` 正确性。
3. 发布前必须在 `real` 模式完成真机登录验收（`wx.login -> code2session -> buyer session`）。
