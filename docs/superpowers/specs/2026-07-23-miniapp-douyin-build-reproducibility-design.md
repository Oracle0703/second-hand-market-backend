# Douyin Miniapp Reproducible Build Design

## Goal

Make the Douyin miniapp build reproducible across developers and coding agents, and document the verified recovery procedure so future build failures start from evidence instead of version guessing.

## Scope

- Add a standalone troubleshooting guide at `docs/miniapp-douyin-build-troubleshooting.md`.
- Put the exact artifact build procedure first in that guide.
- Pin the Node and npm versions used by the verified build.
- Prevent `@babel/plugin-transform-runtime` from drifting to the incompatible release that caused Taro compilation to hang.
- Update the lockfile and document `npm ci` as the standard installation command.
- Add the troubleshooting guide to the repository documentation index.

This change does not upgrade Taro or alter miniapp runtime behavior.

## Version Policy

The verified toolchain is:

- Node.js: `22.22.2`
- npm: `10.9.7`
- Taro: `3.6.34` (already pinned exactly)
- `@babel/core`: `7.29.0`
- `@babel/plugin-transform-runtime`: `7.26.10`
- `@babel/runtime`: `7.28.6` from the lockfile

The repository will add `miniapp/.nvmrc`, set `packageManager` and `engines` in `miniapp/package.json`, pin `@babel/core` exactly, and use npm `overrides` to force `@babel/plugin-transform-runtime@7.26.10` throughout the dependency tree.

`package-lock.json` remains the authoritative dependency graph. Developers and agents must use `npm ci` for a clean, repeatable install. `npm install` is reserved for intentional dependency changes that include review of the resulting lockfile diff.

## Troubleshooting Guide Structure

The guide will be ordered for fast recovery:

1. Prerequisites and exact version checks.
2. Clean dependency installation with `npm ci`.
3. Douyin production build command.
4. Expected output directory and required artifact files.
5. Generated CSS verification for the home product grid.
6. Known failure symptoms and the verified root cause.
7. Minimal Babel reproduction used to isolate the failure.
8. Diagnostic decision tree for hangs at Taro startup or webpack's Babel stage.
9. Recovery steps and rules that prevent dependency drift.

Commands will be directly runnable from the repository root or will explicitly include `cd miniapp`. Every success condition will include observable output or a file assertion.

## Verified Failure And Root Cause

The failing dependency graph used Taro `3.6.34` with `@babel/plugin-transform-runtime@7.29.0`. A minimal `babel.transformFileSync` call using the repository Taro preset did not return, and `npm run build:tt` stalled in Babel compilation.

The investigation isolated the runtime plugin by proving:

- TypeScript, React, and environment presets completed normally.
- Decorator and class-property plugins completed normally.
- Adding `@babel/plugin-transform-runtime@7.29.x` reproduced the hang.
- Replacing only that plugin with `7.26.10` made the same Taro preset transform complete.
- A clean Taro build then completed successfully and generated `miniapp/dist/tt`.

## Verification

Implementation is complete only when all of the following succeed with the pinned toolchain:

```bash
cd miniapp
npm ci
npm test -- home-grid-layout.test.ts --pool=forks --maxWorkers=1 --minWorkers=1
npm run build:tt
```

The build output must contain `dist/tt/app.json` and `dist/tt/app.ttss`. The generated `.home-product-card` rule must use `48.5%` width constraints and must not contain the previous mixed-unit `calc()` expression.

## Documentation Quality Checks

- No `TODO`, `TBD`, or placeholder commands.
- No destructive cleanup command with an ambiguous target.
- Every command states its working directory.
- Temporary recovery steps are clearly distinguished from permanent repository configuration.
- The first usable section answers how to build the artifact without requiring readers to understand the incident history.
