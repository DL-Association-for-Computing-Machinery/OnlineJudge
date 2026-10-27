.PHONY: api worker worker-api go-check go-build web-check check db-test sqlc sandbox-test

# Keep downloaded toolchains, dependency caches and binaries out of source Git.
export GOCACHE := $(CURDIR)/.local/go-build
export GOMODCACHE := $(CURDIR)/.local/go-mod

api:
	go run ./cmd/api

worker:
	go run ./cmd/worker

worker-api:
	go run ./cmd/worker-api

go-check:
	test -z "$$(gofmt -l cmd internal db tests)"
	go vet ./...
	go test -race ./...

go-build:
	mkdir -p .local/bin
	go build -trimpath -o .local/bin/api ./cmd/api
	go build -trimpath -o .local/bin/worker ./cmd/worker
	go build -trimpath -o .local/bin/worker-api ./cmd/worker-api
	go build -trimpath -o .local/bin/migrate ./cmd/migrate

sqlc:
	.local/bin/sqlc generate

db-test: go-build
	sh scripts/test-postgres.sh

sandbox-test:
	go test -count=1 -tags=sandbox ./tests/sandbox -v

web-check:
	pnpm --dir web lint
	pnpm --dir web typecheck
	pnpm --dir web test
	pnpm --dir web build

check: go-check go-build web-check
	python3 scripts/repo_policy.py repository
	python3 -B -m unittest discover -s tests/governance -v
