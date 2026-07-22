# Douyin Home Grid Small-Screen Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Keep the Douyin miniapp home-page product list at two columns on small-screen phones.

**Architecture:** Preserve the existing flex layout, but make each card's percentage size an explicit minimum and maximum contract that cannot grow from intrinsic content. Strengthen the source-level regression test, then verify that a fresh Douyin build emits percentage widths and no mixed-unit `calc()` rule.

**Tech Stack:** Taro 3.6.34, React 18, SCSS, Vitest

---

### Task 1: Lock the small-screen layout contract

**Files:**
- Modify: `miniapp/tests/home-grid-layout.test.ts`
- Test: `miniapp/tests/home-grid-layout.test.ts`

- [x] **Step 1: Write the failing test**

Extend the existing assertion set so the grid must own its full content width, the card must have a percentage `max-width`, and flex `gap` cannot be used to calculate column widths:

```ts
expect(appStyles).toContain('width: 100%;')
expect(appStyles).toContain('max-width: 48.5%;')
expect(appStyles).not.toMatch(/\.home-grid\s*\{[^}]*\bgap:/s)
```

- [x] **Step 2: Run the focused test to verify it fails**

Run:

```bash
cd miniapp
bash -lc 'source ~/.nvm/nvm.sh && nvm use >/dev/null && npm test -- home-grid-layout.test.ts --pool=forks --maxWorkers=1 --minWorkers=1'
```

Expected: FAIL because `.home-product-card` does not yet declare `max-width: 48.5%`.

### Task 2: Harden the two-column flex layout

**Files:**
- Modify: `miniapp/src/styles/app.scss:343`
- Test: `miniapp/tests/home-grid-layout.test.ts`

- [x] **Step 1: Implement the minimal CSS change**

Keep the existing two-column flex layout and add explicit container/card bounds:

```scss
.home-grid {
  display: flex;
  width: 100%;
  box-sizing: border-box;
  flex-wrap: wrap;
  justify-content: space-between;
  align-items: stretch;
}

.home-product-card {
  flex: 0 0 48.5%;
  width: 48.5%;
  max-width: 48.5%;
  min-width: 0;
}
```

- [x] **Step 2: Run the focused test to verify it passes**

Run the Task 1 command again.

Expected: one test file passes with zero failures.

- [x] **Step 3: Run the complete miniapp test suite**

Run:

```bash
cd miniapp
bash -lc 'source ~/.nvm/nvm.sh && nvm use >/dev/null && npm test -- --pool=forks --maxWorkers=1 --minWorkers=1'
```

Expected: all miniapp tests pass.

- [x] **Step 4: Build and inspect the Douyin artifact**

Run:

```bash
cd miniapp
bash -lc 'source ~/.nvm/nvm.sh && nvm use >/dev/null && npm run build:tt'
rg -o '\.home-grid\{[^}]*\}|\.home-product-card\{[^}]*\}' dist/tt/app.ttss
```

Expected: the generated card contains percentage `width`/`max-width`, and neither rule contains `calc(50% - 9rpx)` or a flex `gap`.

- [x] **Step 5: Commit the focused fix**

```bash
git add miniapp/src/styles/app.scss miniapp/tests/home-grid-layout.test.ts docs/superpowers/plans/2026-07-23-douyin-home-grid-small-screen.md
git commit -m "fix(miniapp): keep Douyin home products in two columns"
```
