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
