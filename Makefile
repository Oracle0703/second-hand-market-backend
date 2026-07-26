.PHONY: test backend-test backend-run frontend-dev acceptance-mysql-smoke acceptance-file-schema-smoke acceptance-file-binding-smoke acceptance-license-file-privacy-smoke acceptance-miniapp-auth-refresh-smoke

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

acceptance-file-schema-smoke:
	@test "$${FILE_SCHEMA_ACCEPTANCE_CONFIRM:-}" = "I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_FILE_SCHEMA_DATA" || { echo "set FILE_SCHEMA_ACCEPTANCE_CONFIRM for the isolated file schema smoke" >&2; exit 1; }
	@test "$${ACCEPTANCE_DB_ENGINE:-}" = "mysql8.4" || { echo "set ACCEPTANCE_DB_ENGINE=mysql8.4" >&2; exit 1; }
	./deploy/acceptance/file-record-schema-smoke.sh

acceptance-file-binding-smoke:
	@test "$${FILE_BINDING_ACCEPTANCE_CONFIRM:-}" = "I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_FILE_BINDING_DATA" || { echo "set FILE_BINDING_ACCEPTANCE_CONFIRM for the isolated file binding smoke" >&2; exit 1; }
	@test "$${ACCEPTANCE_DB_ENGINE:-}" = "mysql8.4" || { echo "set ACCEPTANCE_DB_ENGINE=mysql8.4" >&2; exit 1; }
	./deploy/acceptance/file-binding-authorization-smoke.sh

acceptance-license-file-privacy-smoke:
	@test "$${LICENSE_FILE_PRIVACY_ACCEPTANCE_CONFIRM:-}" = "I_UNDERSTAND_THIS_WRITES_ONLY_ISOLATED_LICENSE_PRIVACY_DATA" || { echo "set LICENSE_FILE_PRIVACY_ACCEPTANCE_CONFIRM for isolated license privacy smoke" >&2; exit 1; }
	@test "$${ACCEPTANCE_DB_ENGINE:-}" = "mysql8.4" || { echo "set ACCEPTANCE_DB_ENGINE=mysql8.4" >&2; exit 1; }
	./deploy/acceptance/license-file-privacy-smoke.sh

acceptance-miniapp-auth-refresh-smoke:
	@test "$${MINIAPP_AUTH_REFRESH_ACCEPTANCE_CONFIRM:-}" = "I_UNDERSTAND_THIS_RUNS_ONLY_ISOLATED_MINIAPP_TESTS" || { echo "set MINIAPP_AUTH_REFRESH_ACCEPTANCE_CONFIRM for isolated miniapp auth refresh tests" >&2; exit 1; }
	./deploy/acceptance/miniapp-auth-refresh-smoke.sh
