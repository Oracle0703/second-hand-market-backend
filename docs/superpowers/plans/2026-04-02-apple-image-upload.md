# Apple Image Upload Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让后端支持 `jpg/jpeg/png/webp/heic/heif` 上传，统一执行服务端压缩，并按“原图不超过 40MB 即可上传，压缩目标 20MB 但不是强制门槛”的规则落盘与存储。

**Architecture:** 继续保留现有 `presign -> upload -> confirm` 链路，在 `files/upload` 中接入独立的图片处理模块。图片处理模块通过调用容器内 `vips` CLI 完成 MIME 探测、压缩、缩放和格式保持，避免给 Go 构建链引入 CGO 编译要求。上传规则、压缩策略和处理器路径通过配置统一管理，测试通过 fake processor 和回归接口测试双层覆盖。

**Tech Stack:** Go 1.22, Gin, GORM, libvips/libheif CLI, React + TypeScript, 本地文件存储

---

## File Map

| 路径 | 类型 | 责任 |
| --- | --- | --- |
| `backend/internal/app/config.go` | 修改 | 新增图片上传上限、压缩目标、处理器二进制路径等配置 |
| `backend/internal/app/server.go` | 修改 | 将图片处理器注入 `Server`，便于 handler 调用和测试替身注入 |
| `backend/internal/app/file_handlers.go` | 修改 | 调整 presign/upload 规则，接入真实 MIME 探测和压缩落盘 |
| `backend/go.mod` | 修改 | 显式声明图片 MIME 探测或处理所需依赖 |
| `backend/go.sum` | 修改 | 锁定新增或转正后的依赖版本 |
| `backend/internal/media/processor.go` | 新建 | 定义图片处理接口、请求/响应结构、格式与阈值策略 |
| `backend/internal/media/vips_cli_processor.go` | 新建 | 通过 `vips` CLI 执行压缩、缩放、格式保持 |
| `backend/internal/media/processor_test.go` | 新建 | 单元测试压缩策略、格式判定和命令 fallback 逻辑 |
| `backend/tests/file_upload_test.go` | 修改 | 覆盖上传大小规则、非法格式、压缩后落盘元数据 |
| `backend/tests/integration_flow_test.go` | 修改 | 保持主流程能走通，适配新的上传规则 |
| `backend/tests/restricted_and_security_test.go` | 修改 | 补充 onboarding/权限边界下的新上传限制不回归 |
| `backend/configs/.env.example` | 修改 | 暴露新图片上传与处理配置 |
| `backend/configs/.env.production.mysql.example` | 修改 | 暴露生产环境图片处理配置 |
| `backend/configs/.env.production.sqlite.example` | 修改 | 暴露生产环境图片处理配置 |
| `backend/Dockerfile` | 新建 | 安装 `libvips/libheif` 运行依赖，定义 Linux Docker 运行环境 |
| `README.md` | 修改 | 更新上传大小、苹果图片支持和 Docker 依赖说明 |
| `docs/backend-api-checklist.md` | 修改 | 更新文件上传模块接口约束和失败场景 |
| `frontend/src/pages/auth/RegisterPage.tsx` | 修改 | 更新营业执照上传提示文案 |
| `frontend/src/pages/merchant/products/CreatePage.tsx` | 修改 | 更新商品图上传提示文案 |
| `frontend/src/pages/merchant/products/EditPage.tsx` | 修改 | 更新商品图编辑页上传提示文案 |

## Task 1: 锁定上传规则与接口回归

**Files:**
- Modify: `backend/tests/file_upload_test.go`
- Modify: `backend/tests/integration_flow_test.go`
- Modify: `backend/tests/restricted_and_security_test.go`

- [ ] **Step 1: 写失败中的上传规则测试**

```go
func TestFilePresignAllowsImageUpTo40MB(t *testing.T) {
	srv := newTestServer(t)

	resp := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type":  "MERCHANT_LICENSE",
		"file_name": "license.heic",
		"file_size": 40 * 1024 * 1024,
		"mime_type": "image/heic",
	}, nil)

	if resp.Code != 0 {
		t.Fatalf("presign should allow 40MB image: %+v", resp)
	}
}

func TestFilePresignRejectsImageOver40MB(t *testing.T) {
	srv := newTestServer(t)

	resp := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type":  "MERCHANT_LICENSE",
		"file_name": "huge.jpg",
		"file_size": 40*1024*1024 + 1,
		"mime_type": "image/jpeg",
	}, nil)

	if resp.Code != 10008 {
		t.Fatalf("presign should reject image > 40MB: %+v", resp)
	}
}
```

- [ ] **Step 2: 运行测试，确认按当前实现失败**

Run: `go test ./tests -run "TestFilePresignAllowsImageUpTo40MB|TestFilePresignRejectsImageOver40MB" -v`  
Expected: 当前 `maxUploadSizeBytes` 仍是 `5MB`，至少第一个测试失败。

- [ ] **Step 3: 写上传阶段的回归测试桩，先描述目标行为**

```go
func TestFileUploadRejectsLivePhotoVideo(t *testing.T) {
	srv := newTestServer(t)

	presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type":  "MERCHANT_LICENSE",
		"file_name": "live.mov",
		"file_size": 1024,
		"mime_type": "video/quicktime",
	}, nil)

	if presign.Code != 10008 {
		t.Fatalf("live photo video should be rejected: %+v", presign)
	}
}
```

- [ ] **Step 4: 运行上传相关测试，确认失败原因是规则尚未实现**

Run: `go test ./tests -run "TestFileUploadLocalPublicLicense|TestFileUploadRejectsLivePhotoVideo" -v`  
Expected: `video/quicktime` 场景失败或行为与目标不一致。

- [ ] **Step 5: 提交测试基线**

```bash
git add backend/tests/file_upload_test.go backend/tests/integration_flow_test.go backend/tests/restricted_and_security_test.go
git commit -m "test: lock image upload contract"
```

## Task 2: 实现图片处理器与压缩策略

**Files:**
- Create: `backend/internal/media/processor.go`
- Create: `backend/internal/media/vips_cli_processor.go`
- Create: `backend/internal/media/processor_test.go`
- Modify: `backend/go.mod`
- Modify: `backend/go.sum`
- Modify: `backend/internal/app/config.go`
- Modify: `backend/internal/app/server.go`

- [ ] **Step 1: 先写压缩策略和格式判定的失败测试**

```go
func TestPolicyAllowsUploadWhenOriginalWithin40MB(t *testing.T) {
	policy := UploadPolicy{
		MaxOriginalBytes: 40 * 1024 * 1024,
		TargetBytes:      20 * 1024 * 1024,
	}

	if err := policy.ValidateOriginalSize(40 * 1024 * 1024); err != nil {
		t.Fatalf("expected 40MB to be accepted: %v", err)
	}
}

func TestPolicyRejectsOriginalOver40MB(t *testing.T) {
	policy := UploadPolicy{
		MaxOriginalBytes: 40 * 1024 * 1024,
		TargetBytes:      20 * 1024 * 1024,
	}

	if err := policy.ValidateOriginalSize(40*1024*1024 + 1); err == nil {
		t.Fatal("expected oversize original file to be rejected")
	}
}

func TestAllowedImageMIMERejectsQuickTime(t *testing.T) {
	if IsAllowedImageMIME("video/quicktime") {
		t.Fatal("quicktime video must not be treated as image")
	}
}
```

- [ ] **Step 2: 运行单元测试，确认当前包和符号不存在**

Run: `go test ./internal/media -run Test -v`  
Expected: 包或类型不存在，测试失败。

- [ ] **Step 3: 写最小实现，先把策略和接口落出来**

```go
type UploadPolicy struct {
	MaxOriginalBytes int64
	TargetBytes      int64
	MinWidth         int
	MinHeight        int
	MinQuality       int
}

type ProcessRequest struct {
	FileName string
	InputMIME string
	Content  []byte
}

type ProcessResult struct {
	OutputMIME string
	OutputExt  string
	Content    []byte
	Width      int
	Height     int
}

type Processor interface {
	Process(ctx context.Context, req ProcessRequest) (ProcessResult, error)
}
```

- [ ] **Step 4: 实现 `vips` CLI 处理器，保持“目标 20MB、但不是拒绝门槛”**

```go
func (p *VipsCLIProcessor) Process(ctx context.Context, req ProcessRequest) (ProcessResult, error) {
	if err := p.policy.ValidateOriginalSize(int64(len(req.Content))); err != nil {
		return ProcessResult{}, err
	}

	detected := DetectImageMIME(req.Content, req.InputMIME)
	if !IsAllowedImageMIME(detected) {
		return ProcessResult{}, common.ErrInvalidUpload
	}

	// 伪代码：循环尝试质量和尺寸
	// 1. 使用 detected 对应扩展名创建临时输入文件
	// 2. 调用 vips CLI 输出同格式文件
	// 3. 若输出 > 20MB，降低质量或缩小尺寸后重试
	// 4. 到达最小质量/尺寸后仍 > 20MB，则直接返回当前最优结果
}
```

- [ ] **Step 5: 运行单元测试，确认策略测试转绿**

Run: `go test ./internal/media -run Test -v`  
Expected: 包内测试全部通过。

- [ ] **Step 6: 提交图片处理器基础设施**

```bash
git add backend/go.mod backend/go.sum backend/internal/media/processor.go backend/internal/media/vips_cli_processor.go backend/internal/media/processor_test.go backend/internal/app/config.go backend/internal/app/server.go
git commit -m "feat: add image processing pipeline"
```

## Task 3: 将处理器接入上传 handler

**Files:**
- Modify: `backend/internal/app/file_handlers.go`
- Modify: `backend/internal/app/server.go`
- Modify: `backend/tests/file_upload_test.go`
- Modify: `backend/tests/integration_flow_test.go`

- [ ] **Step 1: 为 handler 接入写失败测试，验证落盘的是处理后的结果**

```go
func TestFileUploadStoresProcessedMetadata(t *testing.T) {
	srv := newTestServerWithProcessor(t, fakeProcessor{
		result: media.ProcessResult{
			OutputMIME: "image/heic",
			OutputExt:  ".heic",
			Content:    bytes.Repeat([]byte("x"), 1024),
		},
	})

	presign := requestJSON(t, srv.Router, http.MethodPost, "/api/v1/files/presign", map[string]interface{}{
		"biz_type":  "MERCHANT_LICENSE",
		"file_name": "license.heic",
		"file_size": 2048,
		"mime_type": "image/heic",
	}, nil)

	upload := requestMultipart(t, srv.Router, http.MethodPost, "/api/v1/files/upload", map[string]string{
		"file_id": fmt.Sprintf("%d", numToUint64(presign.Data["file_id"])),
		"object_key": str(presign.Data["object_key"]),
	}, "file", "license.heic", []byte("original"), nil)

	if upload.Code != 0 {
		t.Fatalf("upload failed: %+v", upload)
	}
}
```

- [ ] **Step 2: 运行 handler 相关测试，确认当前实现尚未使用处理器**

Run: `go test ./tests -run "TestFileUploadLocalPublicLicense|TestFileUploadStoresProcessedMetadata" -v`  
Expected: 新测试失败，因为当前 `handleUploadFile` 仍直接拷贝原始文件。

- [ ] **Step 3: 实现最小接入**

```go
func (s *Server) handleUploadFile(c *gin.Context) {
	// 1. 校验 file_id / object_key / 授权
	// 2. 将 multipart 内容全部读入内存（已被 40MB 上限保护）
	// 3. 调用 s.imageProcessor.Process(...)
	// 4. 以 ProcessResult.Content 落盘
	// 5. 用最终 MIME、大小、URL 更新 files 记录
}
```

- [ ] **Step 4: 同步调整 presign 与 upload 的大小校验常量**

```go
const (
	maxUploadSizeBytes       int64 = 40 * 1024 * 1024
	targetCompressedSizeBytes int64 = 20 * 1024 * 1024
)
```

- [ ] **Step 5: 运行上传与主流程测试**

Run: `go test ./tests -run "TestFileUpload|TestMainFlow" -v`  
Expected: 上传与主流程测试全部通过。

- [ ] **Step 6: 提交 handler 集成**

```bash
git add backend/internal/app/file_handlers.go backend/internal/app/server.go backend/tests/file_upload_test.go backend/tests/integration_flow_test.go
git commit -m "feat: compress uploaded images on server"
```

## Task 4: 配置、Docker 与文档同步

**Files:**
- Modify: `backend/configs/.env.example`
- Modify: `backend/configs/.env.production.mysql.example`
- Modify: `backend/configs/.env.production.sqlite.example`
- Create: `backend/Dockerfile`
- Modify: `README.md`
- Modify: `docs/backend-api-checklist.md`

- [ ] **Step 1: 先写 Docker 与配置的最小约束**

```dockerfile
FROM golang:1.22-bookworm AS build
WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/server ./cmd/server

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends libvips-tools libheif1 ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /srv
COPY --from=build /out/server /srv/server
CMD ["/srv/server"]
```

- [ ] **Step 2: 更新配置样例，明确默认阈值**

```env
FILE_UPLOAD_MAX_MB=40
IMAGE_COMPRESS_TARGET_MB=20
IMAGE_PROCESSOR_BIN=vips
```

- [ ] **Step 3: 运行静态检查与后端全量测试**

Run: `go test ./...`  
Expected: 所有 Go 测试通过。

- [ ] **Step 4: 提交部署与文档变更**

```bash
git add backend/configs/.env.example backend/configs/.env.production.mysql.example backend/configs/.env.production.sqlite.example backend/Dockerfile README.md docs/backend-api-checklist.md
git commit -m "docs: document apple image upload pipeline"
```

## Task 5: 前端提示与最终验证

**Files:**
- Modify: `frontend/src/pages/auth/RegisterPage.tsx`
- Modify: `frontend/src/pages/merchant/products/CreatePage.tsx`
- Modify: `frontend/src/pages/merchant/products/EditPage.tsx`

- [ ] **Step 1: 写最小 UI 变更，补充上传规则文案**

```tsx
<Typography.Text type="secondary">
  支持 JPG、PNG、WebP、HEIC、HEIF，原图最大 40MB，服务端会自动压缩。
</Typography.Text>
```

- [ ] **Step 2: 如前端有本地大小提示，同步更新到 40MB**

```tsx
if (file.size > 40 * 1024 * 1024) {
  message.error('图片原图不能超过 40MB')
  return
}
```

- [ ] **Step 3: 运行前端构建验证**

Run: `npm run build`  
Workdir: `frontend`  
Expected: 前端构建通过。

- [ ] **Step 4: 运行最终回归**

Run: `go test ./...`  
Workdir: `backend`  
Expected: 后端测试通过。

Run: `npm run build`  
Workdir: `frontend`  
Expected: 前端构建通过。

- [ ] **Step 5: 提交前端提示改动**

```bash
git add frontend/src/pages/auth/RegisterPage.tsx frontend/src/pages/merchant/products/CreatePage.tsx frontend/src/pages/merchant/products/EditPage.tsx
git commit -m "feat: update image upload hints"
```

## Notes

| 主题 | 说明 |
| --- | --- |
| HEIC/HEIF 测试 | 本地 CI 未必具备真实 `heic/heif` 样本和 `vips` 运行环境，接口测试应优先用 fake processor 锁定业务行为；真实格式回归可在 Docker 环境补一轮端到端验证 |
| 产物选择 | 当压缩结果比原图更大时，保留更优产物，但元数据必须始终与最终落盘内容一致 |
| 内存占用 | 单次读入 40MB 文件会提高内存占用，handler 接入阶段需要记录日志并避免重复拷贝 |
| 错误码 | 始终复用 `10008` 作为上传非法错误码，不引入新业务码 |
