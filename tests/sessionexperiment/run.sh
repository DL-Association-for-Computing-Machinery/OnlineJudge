#!/bin/sh
# Disposable PostgreSQL is local to the unprivileged test container.
set -eu
umask 077
oj_session_pg=$(mktemp -d /tmp/oj-session-pg.XXXXXX)
cleanup() {
    pg_ctl -D "$oj_session_pg/data" -m fast -w stop >/dev/null 2>&1 || :
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
initdb -D "$oj_session_pg/data" --username=oj_session --auth-local=trust --auth-host=reject --no-locale --encoding=UTF8 >"$oj_session_pg/init.log"
pg_ctl -D "$oj_session_pg/data" -l "$oj_session_pg/server.log" -o "-k $oj_session_pg -c listen_addresses='' -c max_connections=30" -w start >/dev/null
export OJ_SESSION_DATABASE_URL="host=$oj_session_pg user=oj_session dbname=postgres sslmode=disable"
export OJ_SESSION_DATABASE_FIXTURE=1
test -z "$(gofmt -l .)"
go mod verify
go vet ./...
# Explicit Redis pauses and real expiry checks accumulate across the suite.
# This aggregate watchdog is separate from each request/recovery deadline.
go test -mod=readonly -race -count=1 -timeout=120s -v ./...
