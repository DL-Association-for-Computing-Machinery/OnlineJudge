#!/usr/bin/env bash
# Combine two verified fixtures in the same disposable Ubuntu receiver.
set -Eeuo pipefail
[[ $# == 8 && $(id -u) == 0 ]]
socket=$1 root=$2 app=$3 app_pin=$4 image_report=$5 template=$6 judge=$7 judge_pin=$8
[[ $socket == "$root/docker.sock" && $root == /tmp/oj-p6-*/engine.* && -S $socket ]]
[[ $(realpath -e "$root") == "$root" && $(stat -c '%u:%a' "$root") == 0:700 ]]
[[ $(findmnt -n -o PROPAGATION -T /run) == private ]]
[[ -z $(ip route show default) && -z $(ip -6 route show default) ]]
[[ $(ip -o link show | awk -F ': ' '$2 != "lo" {print $2}') =~ ^(docker0)?$ ]]
[[ -S /run/docker.sock ]]
scripts=$(cd -- "$(dirname -- "$0")" && pwd -P)
docker=(/usr/bin/docker --host "unix://$socket")
[[ $("${docker[@]}" info --format '{{.DockerRootDir}}') == "$root/data" ]]
[[ -z $("${docker[@]}" ps -aq) && -z $("${docker[@]}" volume ls -q) ]]

app_revision=$(python3 -B "$scripts/trial_app_bundle.py" verify --platform linux/arm64 --manifest-sha256 "$app_pin" "$app")
judge_result=$(python3 -B "$scripts/offline_judge_bundle.py" verify --manifest-sha256 "$judge_pin" "$judge")
[[ $judge_result == "Verified offline judging bundle: $app_revision" ]]

bash "$scripts/smoke-cold-trial.sh" "$socket" "$root" "$app" "$app_pin" "$image_report" "$template"
[[ -z $("${docker[@]}" ps -aq) && -z $("${docker[@]}" volume ls -q) ]]

# The Ubuntu Worker checks the standard socket path. Redirect it only inside
# this private mount namespace and prove the redirected daemon is our own.
mount --bind "$socket" /run/docker.sock
trap 'umount /run/docker.sock' EXIT
[[ $(/usr/bin/docker --host unix:///var/run/docker.sock info --format '{{.DockerRootDir}}') == "$root/data" ]]
OJ_SANDBOX_EXPERIMENT=1 python3 -B "$scripts/test-ubuntu-worker.py" "$judge" --manifest-sha256 "$judge_pin"
[[ -z $("${docker[@]}" ps -aq) && -z $("${docker[@]}" volume ls -q) ]]
[[ -z $(ip route show default) && -z $(ip -6 route show default) ]]
printf 'Cold trial and fixed judge experiment passed on the same isolated engine at %s\n' "$app_revision"
