.PHONY: setup dev stop logs check integration runtime-test build

setup:
	node scripts/setup.mjs

dev:
	docker compose up --build -d --wait

stop:
	docker compose down

logs:
	docker compose logs -f api web

check:
	cd sg-1/backend && go test -race ./... && go vet ./...
	cd sg-1/frontend && npm ci && npm run typecheck && npm run lint && npm test && npm run build

integration:
	cd sg-1/backend && SG1_TEST_DATABASE_URL=postgres://sg1:sg1_local_only@127.0.0.1:55432/sg1?sslmode=disable go test -race -count=1 ./...

build:
	docker compose build

runtime-test:
	node --env-file=.env scripts/test-runtime.mjs
