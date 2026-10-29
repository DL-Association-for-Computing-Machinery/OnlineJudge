#!/bin/sh
# Runs inside the toolchain image, never on the developer host.
set -eu
build() {
    mkdir -p .local/bin
    for app in api worker worker-api migrate; do
        go build -trimpath -o ".local/bin/$app" "./cmd/$app"
    done
}
case "${1:-}" in
    backend)
        test -z "$(gofmt -l cmd internal db tests)"
        go vet ./...
        go test -race -count=1 ./...
        build ;;
    db) build; sh scripts/test-postgres.sh ;;
    sqlc)
        cp -r internal/submission/postgres/submissiondb /tmp/submissiondb-before
        sqlc generate
        diff -ru /tmp/submissiondb-before internal/submission/postgres/submissiondb ;;
    *) echo 'Expected backend, db or sqlc' >&2; exit 2 ;;
esac
