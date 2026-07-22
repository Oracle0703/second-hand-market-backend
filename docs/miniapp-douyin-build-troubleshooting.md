# 抖音小程序构建与排障

本文先给出生成抖音小程序产物的标准步骤，再记录 2026-07-23 已验证的构建卡死根因和排查证据。以下命令默认从仓库根目录开始执行。

## 构建 `dist/tt` 产物

### 1. 切换到固定的 Node 和 npm

```bash
cd miniapp
nvm use
node --version
npm --version
```

预期版本：

```text
v22.22.2
10.9.7
```

`nvm use` 会读取 `miniapp/.nvmrc`。如果本机没有该 Node 版本，先执行：

```bash
nvm install 22.22.2
nvm use 22.22.2
```

### 2. 按锁文件干净安装依赖

```bash
npm ci --registry=https://registry.npmmirror.com --no-audit --no-fund
```

`npm ci` 会删除并重建 `miniapp/node_modules`，但不会修改源码。它严格使用 `miniapp/package-lock.json`，是开发者和编码模型执行构建前的标准安装方式。

安装完成后检查关键构建依赖：

```bash
node -p "require('./node_modules/@babel/core/package.json').version"
node -p "require('./node_modules/@babel/plugin-transform-runtime/package.json').version"
npm ls @babel/plugin-transform-runtime --all
```

前两条命令必须依次输出：

```text
7.29.0
7.26.10
```

`npm ls` 列出的所有 `@babel/plugin-transform-runtime` 都必须是 `7.26.10`，不能出现 `7.29.x`。

### 3. 执行抖音生产构建

```bash
npm run build:tt
```

成功时 Webpack 会输出：

```text
Compiled successfully
```

产物目录是：

```text
miniapp/dist/tt
```

### 4. 检查必要产物

仍在 `miniapp/` 目录执行：

```bash
test -f dist/tt/app.json && echo "app.json ok"
test -f dist/tt/app.ttss && echo "app.ttss ok"
find dist/tt -type f | wc -l
du -sh dist/tt
```

前两条命令必须分别输出 `app.json ok` 和 `app.ttss ok`。文件数量和目录大小会随功能变化，不应写死，只需确认不是空目录。

### 5. 检查首页商品双列样式

```bash
rg -o '\.home-grid\{[^}]*\}|\.home-product-card\{[^}]*\}' dist/tt/app.ttss
```

生成结果必须满足：

- `.home-grid` 包含 `display:flex`、`width:100%` 和 `justify-content:space-between`。
- `.home-product-card` 包含 `flex:0 0 48.5%`、`width:48.5%`、`max-width:48.5%` 和 `min-width:0`。
- 两条规则中没有 `gap`，商品卡片规则中没有 `calc(`。

自动检查命令：

```bash
node -e "const fs=require('node:fs');const css=fs.readFileSync('dist/tt/app.ttss','utf8');const grid=css.match(/\.home-grid\{([^}]*)\}/)?.[1]||'';const card=css.match(/\.home-product-card\{([^}]*)\}/)?.[1]||'';const ok=['display:flex','width:100%','justify-content:space-between'].every(v=>grid.includes(v))&&['flex:0 0 48.5%','width:48.5%','max-width:48.5%','min-width:0'].every(v=>card.includes(v))&&!grid.includes('gap:')&&!card.includes('calc(');if(!ok){console.error('home grid artifact check failed');process.exit(1)}console.log('home grid artifact ok')"
```

预期输出：

```text
home grid artifact ok
```

### 6. 在抖音开发者工具中加载

开发者工具的项目目录应指向 `miniapp/dist/tt`，不要指向 `miniapp/src`。

- 已导入该目录：点击“编译”或重新加载项目。
- 之前导入的是其他目录：重新导入 `miniapp/dist/tt`。
- 页面仍是旧效果：先确认 `dist/tt/app.ttss` 通过上一节检查，再清理开发者工具缓存并重新编译。

## 固定的构建版本

| 组件 | 固定版本 | 约束位置 |
| --- | --- | --- |
| Node.js | `22.22.2` | `miniapp/.nvmrc`、`miniapp/package.json#engines` |
| npm | `10.9.7` | `miniapp/package.json#packageManager`、`engines` |
| Taro | `3.6.34` | `miniapp/package.json`、`miniapp/package-lock.json` |
| `@babel/core` | `7.29.0` | 精确 devDependency |
| `@babel/plugin-transform-runtime` | `7.26.10` | 精确 devDependency 和 npm override |
| `@babel/runtime` | `7.28.6` | `miniapp/package-lock.json` |

版本约束的回归测试：

```bash
npm test -- build-toolchain-lock.test.ts --pool=forks --maxWorkers=1 --minWorkers=1
```

## 已验证的故障现象

故障可能表现为以下任一种情况：

1. `npm run build:tt` 打印 `Taro v3.6.34` 后长时间没有 webpack 进度。
2. Webpack 停在 `babel-loader > src/app.config.ts`。
3. Webpack继续后停在 `babel-loader > src/pages/home/index.tsx`。
4. 单独调用 Babel 转换首页文件也长时间不返回。

构建正常时，Taro 初始化后会进入 webpack 进度，并在数秒至数十秒内输出 `Compiled successfully`。不能只因为终端暂时没有新日志就换 Node 或改 Babel 配置，应先运行下面的最小复现和版本检查。

## 已验证的根因

Taro `3.6.34` 的 `babel-preset-taro` 对 `@babel/plugin-transform-runtime` 使用宽泛的 `^7.14.5` 版本范围。依赖树漂移到 `@babel/plugin-transform-runtime@7.29.0` 后，Babel 转换不再返回，导致完整构建看起来停在 Taro 初始化或 `babel-loader`。

本次定位的证据链：

1. TypeScript、React 和 preset-env 组合可在约半秒完成首页转换。
2. 加入 decorators 和 class-properties 插件后仍可正常完成。
3. 加入 `@babel/plugin-transform-runtime@7.29.x` 后同一转换卡住。
4. 只将该插件替换为 `7.26.10`，保留 `@babel/core@7.29.0` 和 `@babel/runtime@7.28.6`，完整 Taro preset 转换约 1.5 秒完成。
5. 相同依赖组合下 `npm run build:tt` 成功，Webpack 实测约 9 秒完成。

因此，`@babel/core@7.29.0` 不是本次卡死的根因，不能通过反复更换 Node 或只降级 Babel core 来判断问题已经解决。

## 最小 Babel 复现

在 `miniapp/` 目录执行：

```bash
node -e "const babel=require('@babel/core');console.log('versions',babel.version,require('@babel/plugin-transform-runtime/package.json').version,require('@babel/runtime/package.json').version);console.time('taro-preset');const result=babel.transformFileSync('src/pages/home/index.tsx');console.timeEnd('taro-preset');console.log(result&&result.code?'ok':'empty')"
```

固定版本下应在几秒内输出类似：

```text
versions 7.29.0 7.26.10 7.28.6
taro-preset: 1.5s
ok
```

如果 10 秒以上仍未返回，使用 `Ctrl-C` 停止，不要继续并行启动更多构建进程。随后执行：

```bash
node --version
npm --version
npm ls @babel/core @babel/plugin-transform-runtime @babel/runtime --all
git diff -- package.json package-lock.json
```

先确认实际版本和依赖文件是否被改动。

## 构建卡住时的准确排查顺序

### 1. 确认没有重复构建进程

```bash
pgrep -fl 'taro|webpack|vitest|node'
```

只检查进程归属。要停止当前终端启动的构建，优先回到该终端按 `Ctrl-C`，不要根据模糊的进程名批量杀进程。

### 2. 检查工作目录和版本

```bash
pwd
node --version
npm --version
npm config get registry
```

预期工作目录以 `/miniapp` 结尾，Node/npm 版本分别为 `v22.22.2` 和 `10.9.7`。

### 3. 检查安装版本和 lockfile

```bash
npm ls @babel/plugin-transform-runtime --all
node -e "const lock=require('./package-lock.json');const rows=Object.entries(lock.packages).filter(([path])=>path.includes('plugin-transform-runtime')).map(([path,meta])=>[path,meta.version]);console.table(rows);if(rows.length===0||rows.some(([,version])=>version!=='7.26.10'))process.exit(1)"
```

这两条命令都必须只显示 `7.26.10`。

### 4. 运行最小 Babel 复现

执行上一节的 `babel.transformFileSync` 命令。最小复现也卡住时，问题在 Babel/Taro 依赖层，不在 webpack、SCSS 或业务接口。

### 5. 再运行完整构建

最小复现通过后再执行：

```bash
npm run build:tt
```

如果最小复现通过但构建仍卡住，记录最后一条 webpack 模块路径，并检查进程状态；不要直接修改 Babel 版本。

## npm 镜像问题

本次排查中，配置为 `http://mirrors.cloud.tencent.com/npm/` 时，首个包元数据请求耗时约 57 秒，随后请求 `@tarojs/react` 长时间不返回。切换到项目 lockfile 使用的 `https://registry.npmmirror.com` 后安装完成。

因此本文所有干净安装命令都显式指定：

```bash
--registry=https://registry.npmmirror.com
```

若安装仍慢，使用下面的命令保留详细证据：

```bash
npm ci --registry=https://registry.npmmirror.com --no-audit --no-fund --loglevel=verbose
```

npm 日志位于 `~/.npm/_logs/`。先查看最新文件列表，不要清空整个 npm 缓存：

```bash
ls -lt ~/.npm/_logs | head
```

`npm cache ls` 在缓存文件很多时可能自身触发 `EMFILE`，该错误不能直接证明 Taro 构建耗尽了文件描述符。

## 恢复流程

当依赖目录被不同开发者或模型改动后，执行：

```bash
cd miniapp
nvm use
git diff -- package.json package-lock.json
npm ci --registry=https://registry.npmmirror.com --no-audit --no-fund
npm test -- build-toolchain-lock.test.ts home-grid-layout.test.ts --pool=forks --maxWorkers=1 --minWorkers=1
npm run build:tt
```

先阅读 `git diff`，确认依赖文件中的改动是否是有意修改。`npm ci` 只重建 `node_modules`，不会替你撤销 `package.json` 或 `package-lock.json` 中已经存在的修改。

## 多人和多模型协作规则

1. 日常安装和 CI 一律使用 `npm ci`。
2. 不执行 `npm update`，不使用无版本号的依赖安装命令。
3. 只有明确的依赖升级任务才执行 `npm install`，并必须审查 `package.json` 和 `package-lock.json` 的 diff。
4. 修改 Node、npm、Taro 或 Babel 版本时，必须同步修改版本约束测试和本文档，并重新执行干净安装与 `build:tt`。
5. 不提交 `node_modules` 或 `dist/tt`；开发者工具使用本地重新生成的 `miniapp/dist/tt`。
6. 构建卡住时先保留版本、最小复现和日志证据，不通过连续尝试多个版本来碰运气。
