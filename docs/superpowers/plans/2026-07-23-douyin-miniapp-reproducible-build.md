# Douyin Miniapp Reproducible Build Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 固定抖音小程序已验证可用的构建工具链，并提供一份优先说明如何生成 `dist/tt` 的独立排障文档。

**Architecture:** `miniapp/.nvmrc` 和 `miniapp/package.json` 声明入口工具版本，npm lockfile 固定完整依赖图，npm override 阻止已确认不兼容的 Babel runtime 插件回升。Vitest 回归测试检查这些约束，独立文档提供从干净安装到产物验证的唯一标准流程。

**Tech Stack:** Node.js 22.22.2, npm 10.9.7, Taro 3.6.34, Babel 7, Vitest 3.2.4

---

## File Map

- Create `miniapp/.nvmrc`: 固定进入小程序目录后 `nvm use` 选择的 Node 版本。
- Modify `miniapp/package.json`: 声明 Node/npm 版本、固定 Babel core，并 override 有问题的 runtime 插件。
- Modify `miniapp/package-lock.json`: 保存与上述声明一致的精确依赖图。
- Create `miniapp/tests/build-toolchain-lock.test.ts`: 防止后续开发者或模型移除版本约束或重新引入不兼容插件。
- Create `docs/miniapp-douyin-build-troubleshooting.md`: 标准构建、产物检查、已知故障与证据化排查手册。
- Modify `README.md`: 在文档索引中暴露排障手册。

### Task 1: Lock The Build Toolchain

**Files:**
- Create: `miniapp/tests/build-toolchain-lock.test.ts`
- Create: `miniapp/.nvmrc`
- Modify: `miniapp/package.json`
- Modify: `miniapp/package-lock.json`

- [ ] **Step 1: Write the failing toolchain regression test**

```ts
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, test } from 'vitest'

const miniappRoot = resolve(__dirname, '..')
const packageJSON = JSON.parse(readFileSync(resolve(miniappRoot, 'package.json'), 'utf8'))
const packageLock = JSON.parse(readFileSync(resolve(miniappRoot, 'package-lock.json'), 'utf8'))

describe('小程序构建工具链版本', () => {
  test('固定 Node、npm 和 Babel runtime 插件版本', () => {
    expect(readFileSync(resolve(miniappRoot, '.nvmrc'), 'utf8').trim()).toBe('22.22.2')
    expect(packageJSON.packageManager).toBe('npm@10.9.7')
    expect(packageJSON.engines).toEqual({ node: '22.22.2', npm: '10.9.7' })
    expect(packageJSON.devDependencies['@babel/core']).toBe('7.29.0')
    expect(packageJSON.devDependencies['@babel/plugin-transform-runtime']).toBe('7.26.10')
    expect(packageJSON.overrides['@babel/plugin-transform-runtime']).toBe('$@babel/plugin-transform-runtime')
    expect(packageLock.packages['node_modules/@babel/plugin-transform-runtime'].version).toBe('7.26.10')
  })
})
```

- [ ] **Step 2: Run the test and verify it fails before configuration exists**

Run from `miniapp/`:

```bash
bash -lc 'source ~/.nvm/nvm.sh && nvm use 22.22.2 >/dev/null && npm test -- build-toolchain-lock.test.ts --pool=forks --maxWorkers=1 --minWorkers=1'
```

Expected: FAIL because `miniapp/.nvmrc` or the new `package.json` fields do not exist.

- [ ] **Step 3: Add the exact Node version file**

Create `miniapp/.nvmrc` with exactly:

```text
22.22.2
```

- [ ] **Step 4: Add package-manager and dependency constraints**

Add these top-level fields to `miniapp/package.json` and change the existing Babel core range:

```json
{
  "packageManager": "npm@10.9.7",
  "engines": {
    "node": "22.22.2",
    "npm": "10.9.7"
  },
  "devDependencies": {
    "@babel/core": "7.29.0",
    "@babel/plugin-transform-runtime": "7.26.10"
  },
  "overrides": {
    "@babel/plugin-transform-runtime": "$@babel/plugin-transform-runtime"
  }
}
```

Keep all existing scripts and dependencies unchanged.

- [ ] **Step 5: Regenerate only the dependency lock metadata**

Run from `miniapp/`:

```bash
bash -lc 'source ~/.nvm/nvm.sh && nvm use 22.22.2 >/dev/null && npm install --package-lock-only --ignore-scripts --registry=https://registry.npmmirror.com --no-audit --no-fund'
```

Expected: exit code 0 and `package-lock.json` records `node_modules/@babel/plugin-transform-runtime` as `7.26.10`.

- [ ] **Step 6: Run the regression test and verify it passes**

Run from `miniapp/`:

```bash
bash -lc 'source ~/.nvm/nvm.sh && nvm use 22.22.2 >/dev/null && npm test -- build-toolchain-lock.test.ts --pool=forks --maxWorkers=1 --minWorkers=1'
```

Expected: one test file and one test pass.

- [ ] **Step 7: Commit the toolchain constraints**

```bash
git add miniapp/.nvmrc miniapp/package.json miniapp/package-lock.json miniapp/tests/build-toolchain-lock.test.ts
git commit -m "build(miniapp): pin reproducible Douyin toolchain"
```

### Task 2: Add The Build-First Troubleshooting Guide

**Files:**
- Create: `docs/miniapp-douyin-build-troubleshooting.md`
- Modify: `README.md`

- [ ] **Step 1: Write the standalone guide with artifact generation first**

The first operational section must contain this exact path:

```bash
cd miniapp
nvm use
node --version
npm --version
npm ci --registry=https://registry.npmmirror.com --no-audit --no-fund
npm run build:tt
test -f dist/tt/app.json
test -f dist/tt/app.ttss
```

Document expected versions `v22.22.2` and `10.9.7`, expected output root `miniapp/dist/tt`, and the Douyin developer-tool import/reload target.

After the build section, document:

- the generated home-grid CSS verification command;
- symptoms at Taro initialization and `babel-loader`;
- the verified incompatibility between Taro 3.6.34 and `@babel/plugin-transform-runtime@7.29.0`;
- the minimal Babel transform reproduction and its expected completion on 7.26.10;
- process, npm-log, installed-version, and lockfile checks;
- the slow Tencent mirror evidence and use of `registry.npmmirror.com`;
- recovery with a clean `npm ci`;
- the rule that `npm install` is only for intentional dependency updates with lockfile review.

- [ ] **Step 2: Add the guide to the README documentation index**

Append this entry under `## 文档索引`:

```markdown
- [抖音小程序构建与排障](docs/miniapp-douyin-build-troubleshooting.md)
```

- [ ] **Step 3: Check documentation commands and formatting**

Run from the repository root:

```bash
rg -n 'npm ci|npm run build:tt|dist/tt|plugin-transform-runtime|7\.26\.10' docs/miniapp-douyin-build-troubleshooting.md
git diff --check -- README.md docs/miniapp-douyin-build-troubleshooting.md
```

Expected: each required build and diagnosis term is present, and `git diff --check` exits 0.

- [ ] **Step 4: Commit the troubleshooting guide**

```bash
git add README.md docs/miniapp-douyin-build-troubleshooting.md
git commit -m "docs: add Douyin miniapp build troubleshooting"
```

### Task 3: Verify A Clean Reproducible Build

**Files:**
- Verify: `miniapp/package.json`
- Verify: `miniapp/package-lock.json`
- Verify: `miniapp/dist/tt/app.json`
- Verify: `miniapp/dist/tt/app.ttss`

- [ ] **Step 1: Reinstall exactly from the lockfile**

Run from `miniapp/`:

```bash
bash -lc 'source ~/.nvm/nvm.sh && nvm use && npm ci --registry=https://registry.npmmirror.com --no-audit --no-fund'
```

Expected: exit code 0. This replaces `node_modules` with the graph from `package-lock.json`.

- [ ] **Step 2: Verify installed toolchain versions**

Run from `miniapp/`:

```bash
node --version
npm --version
node -p "require('./node_modules/@babel/core/package.json').version"
node -p "require('./node_modules/@babel/plugin-transform-runtime/package.json').version"
```

Expected output, in order: `v22.22.2`, `10.9.7`, `7.29.0`, `7.26.10`.

- [ ] **Step 3: Run focused regression tests**

Run from `miniapp/`:

```bash
npm test -- build-toolchain-lock.test.ts home-grid-layout.test.ts --pool=forks --maxWorkers=1 --minWorkers=1
```

Expected: two test files and two tests pass.

- [ ] **Step 4: Build the Douyin artifact**

Run from `miniapp/`:

```bash
npm run build:tt
```

Expected: exit code 0 and Webpack reports `Compiled successfully`.

- [ ] **Step 5: Verify artifact entry files and home-grid CSS**

Run from `miniapp/`:

```bash
test -f dist/tt/app.json
test -f dist/tt/app.ttss
rg -o '\.home-grid\{[^}]*\}|\.home-product-card\{[^}]*\}' dist/tt/app.ttss
```

Expected: `.home-grid` includes `display:flex`, `width:100%`, and `justify-content:space-between`; `.home-product-card` includes `flex:0 0 48.5%`, `width:48.5%`, `max-width:48.5%`, and `min-width:0`; neither rule contains `gap` or `calc(`.

- [ ] **Step 6: Perform the final repository checks**

Run from the repository root:

```bash
git diff --check
git status --short --branch
```

Expected: `git diff --check` exits 0. `AGENTS.md` remains untracked and is not included in either implementation commit.
