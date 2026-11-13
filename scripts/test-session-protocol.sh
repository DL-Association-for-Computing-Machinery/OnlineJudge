#!/bin/sh
# Run only the protocol suite against a private, disposable PostgreSQL cluster.
set -eu
case "$#:${1:-ordinary}" in
    0:ordinary|1:ordinary) oj_pattern='^TestOrdinaryProtocol$' ;;
    1:email-lifecycle)
        export OJ_ISOLATED_EMAIL_LIFECYCLE=1
        oj_pattern='^TestIsolatedEmailLifecycle$' ;;
    *) echo "Invalid session protocol suite" >&2; exit 2 ;;
esac
umask 077
oj_protocol_dir=$(mktemp -d /tmp/oj-protocol.XXXXXX)
oj_pg_started=0
cleanup() {
    status=$?
    trap - EXIT
    if [ "$oj_pg_started" = 1 ] || pg_ctl -D "$oj_protocol_dir/data" status >/dev/null 2>&1; then
        pg_ctl -D "$oj_protocol_dir/data" -m fast -w stop || status=1
    fi
    if [ "$status" -ne 0 ]; then
        echo "Protocol test or PostgreSQL setup failed (exit $status)." >&2
    fi
    exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
initdb -D "$oj_protocol_dir/data" --username=oj_protocol --auth-local=trust --auth-host=reject --no-locale --encoding=UTF8 >"$oj_protocol_dir/init.log" 2>&1
pg_ctl -D "$oj_protocol_dir/data" -l "$oj_protocol_dir/server.log" -o "-k $oj_protocol_dir -c listen_addresses='' -c max_connections=30" -w start
oj_pg_started=1
export OJ_PROTOCOL_DATABASE_URL="host=$oj_protocol_dir user=oj_protocol dbname=postgres sslmode=disable"
export OJ_PROTOCOL_REDIS_ADDR=redis:6379
export OJ_PROTOCOL_REPLACEMENT_REDIS_ADDR=redis-recovery:6379
export OJ_PROTOCOL_INTERRUPTED_REDIS_ADDR=redis-recovery-interrupted:6379
export OJ_PROTOCOL_FIXTURE=1
mkdir "$oj_protocol_dir/workspace"
cp /workspace/go.mod /workspace/go.sum "$oj_protocol_dir/workspace/"
cp -R /workspace/db /workspace/internal /workspace/tests "$oj_protocol_dir/workspace/"
mkdir "$oj_protocol_dir/workspace/cmd"
cp -R /workspace/cmd/api /workspace/cmd/migrate "$oj_protocol_dir/workspace/cmd/"
cd "$oj_protocol_dir/workspace"
if [ -d /verified-module-cache ]; then
    # The image cache may be read-only to the fixture user.
    oj_image_cache=$(go env GOMODCACHE)
    export GOMODCACHE="$oj_protocol_dir/modules"
    mkdir "$GOMODCACHE"
    if [ -d "$oj_image_cache" ]; then
        cp -R "$oj_image_cache/." "$GOMODCACHE/"
        chmod -R u+rwX "$GOMODCACHE"
    fi
    cp -Rn /verified-module-cache/. "$GOMODCACHE/"
fi
go mod verify
golangci-lint run --config /workspace/.golangci.yml --build-tags=integration ./tests/sessionprotocol/...
go test -mod=readonly -race -count=1 -timeout=120s -tags=integration ./tests/sessionprotocol -run "$oj_pattern" -v
