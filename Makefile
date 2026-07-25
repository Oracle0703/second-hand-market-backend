.PHONY: test backend-test backend-run frontend-dev acceptance-mysql-smoke

backend-test:
	cd backend && mkdir -p .cache/go/mod .cache/go/build && GOMODCACHE=$$(pwd)/.cache/go/mod GOCACHE=$$(pwd)/.cache/go/build GOPROXY=https://goproxy.cn,direct go test ./...

test: backend-test

backend-run:
	cd backend && mkdir -p .cache/go/mod .cache/go/build && GOMODCACHE=$$(pwd)/.cache/go/mod GOCACHE=$$(pwd)/.cache/go/build GOPROXY=https://goproxy.cn,direct go run ./cmd/server

frontend-dev:
	cd frontend && npm run dev

acceptance-mysql-smoke:
	@test "$${ACCEPTANCE_CONFIRM_ISOLATED:-}" = "I_UNDERSTAND_THIS_WRITES_TEST_DATA" || { echo "set ACCEPTANCE_CONFIRM_ISOLATED for the isolated destructive smoke" >&2; exit 1; }
	@test "$${ACCEPTANCE_DB_ENGINE:-}" = "mysql8.4" || { echo "set ACCEPTANCE_DB_ENGINE=mysql8.4" >&2; exit 1; }
	node scripts/smoke-mysql-concurrency.mjs
