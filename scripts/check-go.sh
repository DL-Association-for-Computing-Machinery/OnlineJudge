#!/bin/sh
# Static analysis only: sandbox-tagged contestant execution is never started here.
set -eu
sh scripts/test-go-quality.sh
golangci-lint run ./...
golangci-lint run --build-tags=sandbox ./tests/sandbox/...
(cd tests/sessionexperiment && golangci-lint run --config ../../.golangci.yml ./...)
