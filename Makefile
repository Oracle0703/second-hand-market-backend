.PHONY: test backend-test backend-run frontend-dev

backend-test:
	cd backend && mkdir -p .cache/go/mod .cache/go/build && GOMODCACHE=$$(pwd)/.cache/go/mod GOCACHE=$$(pwd)/.cache/go/build GOPROXY=https://goproxy.cn,direct go test ./...

test: backend-test

backend-run:
	cd backend && mkdir -p .cache/go/mod .cache/go/build && APP_ENV=development GOMODCACHE=$$(pwd)/.cache/go/mod GOCACHE=$$(pwd)/.cache/go/build GOPROXY=https://goproxy.cn,direct go run ./cmd/server

frontend-dev:
	cd frontend && npm run dev
