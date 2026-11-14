#!/usr/bin/env bash
# Run only in the private, no-egress network namespace of a disposable receiver.
set -Eeuo pipefail
[[ $(id -u) == 0 ]]
release_mode=false
if [[ ${1:-} == --release ]]; then
    [[ $# == 7 ]]
    release_mode=true
    shift
    socket=$1 root=$2 app=$3 manifest=$4 release_digest=$5 archive=$6
else
    [[ $# == 5 ]]
    socket=$1 root=$2 app=$3 digest=$4 image_report=$5
fi
[[ $socket == "$root/docker.sock" && $root == /tmp/oj-p6-*/engine.* && -S $socket ]]
[[ $(realpath -e "$root") == "$root" && $(stat -c '%u:%a' "$root") == 0:700 ]]
[[ -z $(ip route show default) && -z $(ip -6 route show default) ]]
[[ $(ip -o link show | awk -F ': ' '$2 != "lo" {print $2}') =~ ^(docker0)?$ ]]
[[ $(findmnt -n -o PROPAGATION /) == private ]]
docker=(/usr/bin/docker --host "unix://$socket")
[[ $("${docker[@]}" info --format '{{.DockerRootDir}}') == "$root/data" ]]
[[ -z $("${docker[@]}" ps -aq) && -z $("${docker[@]}" volume ls -q) ]]
scripts=$(cd -- "$(dirname -- "$0")" && pwd -P)
if "$release_mode"; then
    image_report=$(mktemp "$root/release-report.XXXXXX")
    python3 -B "$scripts/cold_trial_release.py" "$manifest" --sha256 "$release_digest" --app "$app" \
        --runtime "$archive" --socket "$socket" --data-root "$root/data" > "$image_report"
    digest=$(python3 -c 'import json, sys; print(json.load(sys.stdin)["application_sha256"])' < "$image_report")
fi
python3 -B "$scripts/trial_app_bundle.py" verify --platform linux/arm64 --manifest-sha256 "$digest" "$app"

image_list=$(python3 - "$image_report" <<'PY'
import json, re, sys
report = json.load(open(sys.argv[1]))
assert report["platform"] == "linux/arm64" and report["initial_images"] == 0
for role in ("postgres", "redis", "nginx"):
    ident = report["image_ids"][role]
    assert re.fullmatch(r"sha256:[a-f0-9]{64}", ident)
    print(ident)
PY
)
mapfile -t images <<< "$image_list"
[[ ${#images[@]} == 3 ]]
for image in "${images[@]}"; do
    [[ $("${docker[@]}" image inspect --format '{{.Id}} {{.Os}}/{{.Architecture}}' "$image") == "$image linux/arm64" ]]
done

stage=$(mktemp -d "$root/fixture.XXXXXX")
umask 077
containers=()
api=
cleanup() {
    status=$?
    trap - EXIT
    if [[ -n $api ]]; then
        kill -TERM "$api" 2>/dev/null || true
        wait "$api" || status=1
    fi
    for ((i=${#containers[@]}-1; i>=0; i--)); do
        id=${containers[i]}
        "${docker[@]}" rm -f "$id" >/dev/null || status=1
    done
    if ((status != 0)); then tail -n 15 "$stage/api.log" "$stage/migration.log" 2>/dev/null || true; fi
    exit "$status"
}
trap cleanup EXIT

# No port publishing: the host namespace here belongs only to the experiment.
common=(--detach --pull=never --network host --read-only --cap-drop ALL
        --security-opt no-new-privileges --tmpfs '/tmp:rw,size=128m,mode=1777')
pg=$("${docker[@]}" run "${common[@]}" --user postgres --tmpfs /var/lib/postgresql/data \
    --entrypoint sh "${images[0]}" -ec '
        initdb -D /tmp/db --username=postgres --auth-local=trust --auth-host=trust --no-locale >/dev/null
        exec postgres -D /tmp/db -h 127.0.0.1 -k /tmp')
containers+=("$pg")
redis=$("${docker[@]}" run "${common[@]}" --tmpfs /data --entrypoint redis-server "${images[1]}" \
    --bind 127.0.0.1 --save "" --appendonly no --maxmemory 32mb --maxmemory-policy noeviction)
containers+=("$redis")

wait_for() {
    for _ in {1..50}; do
        if "$@"; then return; fi
        sleep .1
    done
    return 1
}
wait_for "${docker[@]}" exec "${containers[0]}" pg_isready -h 127.0.0.1 -q
wait_for "${docker[@]}" exec "${containers[1]}" redis-cli ping >/dev/null
OJ_DATABASE_URL='postgresql://postgres@127.0.0.1:5432/postgres?sslmode=disable' "$app/bin/migrate" up > "$stage/migration.log" 2>&1

openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj /CN=TrialFixture \
    -keyout "$stage/ca.key" -out "$stage/api-ca.pem" 2>/dev/null
for role in public backend proxy; do
    case "$role" in
        public) ext='subjectAltName=DNS:oj.example.invalid';;
        backend) ext='subjectAltName=DNS:oj-api.internal';;
        proxy) ext='extendedKeyUsage=clientAuth';;
    esac
    printf '%s\n' "$ext" > "$stage/extension"
    openssl req -new -newkey rsa:2048 -nodes -subj "/CN=$role" -keyout "$stage/$role-key.pem" \
        -out "$stage/leaf.csr" 2>/dev/null
    openssl x509 -req -in "$stage/leaf.csr" -CA "$stage/api-ca.pem" -CAkey "$stage/ca.key" -CAcreateserial \
        -days 1 -extfile "$stage/extension" -out "$stage/$role-chain.pem" 2>/dev/null
done
python3 - "$stage" <<'PY'
import hashlib, json, pathlib, secrets, ssl, sys

root = pathlib.Path(sys.argv[1])
pin = hashlib.sha256(ssl.PEM_cert_to_DER_cert((root / "proxy-chain.pem").read_text())).hexdigest()
bucket = dict(count=10, window="1m")
doc = dict(
    version=1, mode="proxy", namespace="cold-trial", secrets_file=str(root / "secrets.json"),
    origin="https://oj.example.invalid", login_window="1m", startup_timeout="5s", http_timeout="1s",
    database=dict(host="127.0.0.1", port=5432, database="postgres", username="postgres",
                  application_name="cold_trial_api", security="private", auth="none", connections=2,
                  timeout="1s", max_lifetime="1h", max_idle="1m", health_check="1s", tls={}),
    redis=dict(address="127.0.0.1:6379", username="", security="private", auth="none",
               connections=2, timeout="1s", tls={}),
    password=dict(current=dict(memory_kib=128, iterations=2, parallelism=2, salt_bytes=16, digest_bytes=32),
                  legacy=[], limits=dict(max_memory_kib=256, max_iterations=4, max_parallelism=4,
                                         max_password_bytes=128, max_salt_bytes=64, max_digest_bytes=64),
                  admission=dict(max_in_flight=2, max_waiting=0, max_queue_wait="0s")),
    login_limits={name: bucket for name in ("begin_global", "begin_source", "begin_email",
                                          "recovery_global", "recovery_source")},
    proxy_fingerprints=[pin],
    tls=dict(certificate_file=str(root / "backend-chain.pem"), key_file=str(root / "backend-key.pem"),
             ca_file=str(root / "api-ca.pem")),
    server=dict(read_header_timeout="2s", read_timeout="5s", write_timeout="5s", idle_timeout="1m",
                shutdown_timeout="3s", max_header_bytes=8192),
)
(root / "api.json").write_text(json.dumps(doc))
(root / "secrets.json").write_text(json.dumps(dict(version=1, current="fixture",
    sealing=dict(fixture=secrets.token_urlsafe(32)), limiting=secrets.token_urlsafe(32))))
PY
export OJ_RESTORED_SESSION_CONFIG_FILE="$stage/api.json"
closed=$("$app/bin/migrate" session-recovery close postgres)
[[ $closed =~ ^generation=([a-f0-9]{32})\ enabled=false$ ]]
"$app/bin/migrate" session-recovery open postgres "${BASH_REMATCH[1]}" --accounts-reviewed >/dev/null

OJ_SESSION_CONFIG_FILE="$stage/api.json" OJ_API_ADDR=127.0.0.1:9443 "$app/bin/api" > "$stage/api.log" 2>&1 &
api=$!
mkdir "$stage/edge"
for file in public-chain.pem public-key.pem proxy-chain.pem proxy-key.pem api-ca.pem; do
    cp "$stage/$file" "$stage/edge/"
done
chown -R 101:101 "$stage/edge"
cp -R "$app/site" "$stage/site"
install -m 0644 "$app/templates/nginx.conf.example" "$stage/nginx.conf"
find "$stage/site" -type d -exec chmod 755 {} +
find "$stage/site" -type f -exec chmod 644 {} +
nginx=$("${docker[@]}" run "${common[@]}" --user 101:101 \
    --mount "type=bind,src=$stage/edge,dst=/run/oj-edge,readonly" \
    --mount "type=bind,src=$stage/site,dst=/srv/oj/site,readonly" \
    --mount "type=bind,src=$stage/nginx.conf,dst=/trial.conf,readonly" \
    --entrypoint sh "${images[2]}" -ec 'nginx -t -c /trial.conf; exec nginx -c /trial.conf -g "daemon off;"')
containers+=("$nginx")
edge() {
    curl --noproxy '*' --silent --fail --max-time 2 --cacert "$stage/api-ca.pem" \
        --resolve oj.example.invalid:8443:127.0.0.1 -H 'Host: oj.example.invalid' \
        "https://oj.example.invalid:8443$1"
}
ready() { [[ $(edge /api/v1/readyz) == '{"status":"ready","service":"api","scope":"ordinary-session"}' ]]; }
wait_for ready
edge /login > "$stage/page.html"
cmp "$app/site/index.html" "$stage/page.html"
kill -TERM "$api"
wait "$api"
api=
grep -q '"msg":"API stopped gracefully"' "$stage/api.log"
"${docker[@]}" stop --signal SIGQUIT --time 5 "${containers[2]}" >/dev/null
[[ $("${docker[@]}" inspect --format '{{.State.ExitCode}}' "${containers[2]}") == 0 ]]
printf 'Cold trial passed: migration, closed-to-open session state, HTTPS readiness and frontend, API and edge shutdown\n'
if "$release_mode"; then
    printf 'Cold trial release SHA-256: %s\n' "$release_digest"
fi
