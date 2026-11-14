#!/bin/sh
# Exercise the real configured detector, including its success and rejection exits.
set -eu
config="$(pwd)/.golangci.yml"
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT HUP INT TERM
cd "$scratch"
printf 'module qualityfixture\n\ngo 1.26.0\n' > go.mod
cat > fixture.go <<'EOF'
package fixture
func Value() int { return 1 }
EOF
golangci-lint run --config "$config" ./...
cat > fixture.go <<'EOF'
package fixture
import (
    "os"
    "strings"
)
func unusedFunction() {}
func Broken() string {
    os.Chdir(".")
    value := "lost"
    value = "used"
    return strings.Replace(value, "u", "U", 0)
}
EOF
status=0
golangci-lint run --config "$config" ./... > result.txt 2>&1 || status=$?
cat result.txt
test "$status" -eq 1
for rule in errcheck ineffassign staticcheck unused; do
    grep -q "($rule)" result.txt
done
echo 'Go quality detector accepted valid code and rejected all four defect fixtures.'

# Install the same config beside a minimal module so its file globs are real.
cp "$config" .golangci.yml
module=github.com/DL-Association-for-Computing-Machinery/OnlineJudge
printf 'module %s\n\ngo 1.26.0\n' "$module" > go.mod
mkdir -p internal/problem internal/submission/postgres internal/session/runtime cmd/api
printf 'package runtime\nconst Value = 1\n' > internal/session/runtime/runtime.go
printf 'package main\nimport "%s/internal/session/runtime"\nfunc main() { _ = runtime.Value }\n' "$module" > cmd/api/main.go
printf 'package problem\nimport "strings"\nvar Label = strings.TrimSpace(" label ")\n' > internal/problem/problem.go
golangci-lint run ./cmd/api ./internal/problem
for dir in internal/problem internal/submission/postgres; do
    printf 'package %s\nimport "%s/internal/session/runtime"\nvar Fixture = runtime.Value\n' "${dir##*/}" "$module" > "$dir/boundary.go"
done
status=0
golangci-lint run ./internal/... > result.txt 2>&1 || status=$?
cat result.txt
test "$status" -eq 1
for dir in internal/problem internal/submission/postgres; do
    grep -F "$dir/boundary.go:" result.txt | grep -q '(depguard)'
done
echo 'Go import guard accepted API assembly and rejected the business package imports.'
