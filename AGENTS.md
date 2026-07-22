# Repository Guidelines

## Project Structure & Module Organization

This monorepo contains:

- `backend/`: Go 1.22 Gin/GORM API. `cmd/server/` is the entry point, `internal/` holds domain and HTTP logic, `migrations/` stores SQL, and `tests/` contains integration and security tests.
- `frontend/`: React, TypeScript, and Vite merchant/admin application. Keep code in `src/` and colocate tests such as `LoginPage.test.tsx` with their subject.
- `miniapp/`: Taro buyer miniapp. Pages, components, hooks, services, and assets live under `src/`; Vitest suites live in `tests/`.
- `docs/` contains product, API, data-model, and release documentation. `scripts/` contains smoke flows.

## Build, Test, and Development Commands

- `make backend-run`: run the API with repository-local Go caches.
- `make test`: run all backend Go tests (`go test ./...`).
- `cd frontend && npm install && npm run dev`: install dependencies and start Vite. Use `npm run test` for Vitest and `npm run build` for type-checking and production output.
- `cd miniapp && npm install && npm run dev:weapp`: watch-build the WeChat miniapp. Use `dev:tt` for Douyin, `npm test` for Vitest, and `build:weapp` or `build:tt` for release builds.

## Coding Style & Naming Conventions

Run `gofmt` on Go changes. Keep Go package names lowercase, exported identifiers in `PascalCase`, and tests in `*_test.go`. TypeScript is strict; follow existing two-space indentation, single quotes, extension conventions (`.tsx` for React), and the `@/` alias for `src/`. Name React components in `PascalCase`, hooks with `use...`, and utilities in descriptive lowercase filenames. No repository-wide linter is configured, so preserve local style and keep changes focused.

## Testing Guidelines

Backend tests use Go's `testing` package; frontend and miniapp tests use Vitest, with Testing Library available in the frontend. Add regression tests for behavioral changes. Keep frontend tests beside source and miniapp tests as `tests/*.test.ts`. There is no fixed coverage threshold; prioritize authentication, authorization, state transitions, uploads, and API integration paths.

## Commit & Pull Request Guidelines

Recent history follows Conventional Commits, for example `feat(store-guide): add navigation page`, `fix: ...`, and `docs: ...`. Use an imperative, focused subject; Chinese or English is acceptable. Pull requests should describe scope and user impact, link relevant issues, list verification commands, call out migrations or environment changes, and include screenshots for UI changes.

## Security & Configuration

Start from the examples in `backend/configs/` and the documented frontend/miniapp environment variables. Never commit real JWT secrets, database credentials, WeChat/Douyin secrets, local databases, or uploaded media.
