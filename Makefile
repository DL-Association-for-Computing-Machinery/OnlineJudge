.PHONY: api worker go-check go-build web-check check

# Keep downloaded toolchains, dependency caches and binaries out of source Git.
export GOCACHE := $(CURDIR)/.local/go-build
export GOMODCACHE := $(CURDIR)/.local/go-mod

api:
	go run ./cmd/api

worker:
	go run ./cmd/worker

go-check:
	test -z "$$(gofmt -l cmd internal)"
	go vet ./...
	go test -race ./...

go-build:
	mkdir -p .local/bin
	go build -trimpath -o .local/bin/api ./cmd/api
	go build -trimpath -o .local/bin/worker ./cmd/worker

web-check:
	pnpm --dir web lint
	pnpm --dir web typecheck
	pnpm --dir web test
	pnpm --dir web build

check: go-check go-build web-check
	python3 scripts/repo_policy.py repository
	python3 -B -m unittest discover -s tests/governance -v
