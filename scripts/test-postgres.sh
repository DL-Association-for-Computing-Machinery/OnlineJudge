#!/bin/sh
# Disposable, owner-private local cluster. Never connects to an existing database.
set -eu
for tool in initdb pg_ctl createdb go python3; do
    command -v "$tool" >/dev/null || { echo "Missing required tool: $tool" >&2; exit 1; }
done
umask 077
oj_test_dir=$(mktemp -d /tmp/oj-postgres.XXXXXX)
cleanup() {
    pg_ctl -D "$oj_test_dir/data" -m fast -w stop >/dev/null 2>&1 || :
    # Keep logs/data for diagnosis; do not remove directories supplied by users.
    echo "Disposable database stopped; local evidence retained at $oj_test_dir"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
initdb -D "$oj_test_dir/data" --username=oj_test --auth-local=trust --auth-host=reject --no-locale --encoding=UTF8 >"$oj_test_dir/init.log"
pg_ctl -D "$oj_test_dir/data" -l "$oj_test_dir/server.log" -o "-k $oj_test_dir -c listen_addresses='' -c max_connections=30" -w start >/dev/null
export OJ_TEST_DATABASE_URL="host=$oj_test_dir user=oj_test dbname=postgres sslmode=disable"
go test -race -count=1 -tags=integration ./tests/integration -v
createdb -h "$oj_test_dir" -U oj_test oj_process_test
export OJ_TEST_DATABASE_URL="host=$oj_test_dir user=oj_test dbname=oj_process_test sslmode=disable"
python3 tests/integration/api_processes.py
