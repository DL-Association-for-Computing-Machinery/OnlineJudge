#!/usr/bin/env python3
"""Import verified trial images into a disposable, empty local Docker Engine."""

import argparse
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import sys

from verify_trial_images import ROLES, verify


def linux_platform(architecture):
    return {"aarch64": "linux/arm64", "arm64": "linux/arm64",
            "x86_64": "linux/amd64", "amd64": "linux/amd64"}.get(architecture)


def receiver(socket, data_root, archive, digest, platform, roles):
    if (not socket.is_absolute() or not data_root.is_absolute()
            or not stat.S_ISSOCK(socket.lstat().st_mode)
            or data_root == Path("/var/lib/docker") or socket in (Path("/run/docker.sock"), Path("/var/run/docker.sock"))):
        raise ValueError("expected explicit disposable local engine")

    def docker(*args, timeout=15):
        return subprocess.run(("/usr/bin/docker", "--host", "unix://" + str(socket), *args),
                              check=True, capture_output=True, text=True, timeout=timeout,
                              env={"PATH": "/usr/bin:/bin", "HOME": "/", "LANG": "C"}).stdout.strip()

    def inventory():
        values = docker("image", "ls", "--all", "--no-trunc", "--format", "{{.ID}}").splitlines()
        if any(not re.fullmatch(r"sha256:[0-9a-f]{64}", value) for value in values):
            raise ValueError("invalid image inventory")
        return set(values)

    result = verify(archive, digest, platform, roles)
    info = docker("info", "--format", "{{.OSType}}/{{.Architecture}}\n{{.DockerRootDir}}").splitlines()
    if len(info) != 2 or info[1] != str(data_root):
        raise ValueError("engine data root does not match")
    if not info[0].startswith("linux/") or linux_platform(info[0][6:]) != platform:
        raise ValueError("engine platform does not match the release")
    if inventory() or docker("ps", "-aq") or docker("volume", "ls", "-q"):
        raise ValueError("receiver must start with empty images, containers and volumes")
    docker("image", "load", "--platform", platform, "--input", str(archive), timeout=180)
    actual = inventory()
    resolved = {}
    for role, configuration in roles.items():
        candidates = actual & {configuration, result["manifest_digests"][role]}
        if len(candidates) != 1:
            raise ValueError("imported image identity mismatch")
        ident = candidates.pop()
        if docker("image", "inspect", "--format", "{{.Id}} {{.Os}}/{{.Architecture}}", ident) != ident + " " + platform:
            raise ValueError("imported image platform mismatch")
        resolved[role] = ident
    if actual != set(resolved.values()) or len(actual) != len(roles):
        raise ValueError("imported inventory differs from verified images")

    smoke = {
        "nginx": ("nginx", "-v"),
        "postgres": ("sh", "-ec", 'initdb -D /tmp/db --auth-local=trust --auth-host=reject --no-locale >/dev/null; '
                     'pg_ctl -D /tmp/db -l /tmp/postgres.log -o "-k /tmp -c listen_addresses=" -w start >/dev/null; '
                     'trap "pg_ctl -D /tmp/db -m fast -w stop >/dev/null" EXIT; '
                     'test "$(psql -h /tmp -d postgres -Atc \'SELECT 1\')" = 1'),
        "redis": ("sh", "-ec", 'redis-server --bind 127.0.0.1 --save "" --appendonly no --daemonize yes '
                  '--pidfile /tmp/redis.pid --dir /tmp; trap "redis-cli shutdown nosave >/dev/null" EXIT; '
                  'test "$(redis-cli set trial-smoke ready)" = OK; test "$(redis-cli get trial-smoke)" = ready'),
    }
    for role, (entry, *args) in smoke.items():
        user = ("--user", "postgres", "--tmpfs", "/var/lib/postgresql/data") if role == "postgres" else (
            ("--tmpfs", "/data") if role == "redis" else ())
        docker("run", "--rm", "--pull=never", "--network", "none", "--cap-drop", "ALL",
               "--security-opt", "no-new-privileges", "--read-only", "--tmpfs", "/tmp:rw,size=128m,mode=1777",
               *user, "--entrypoint", entry, resolved[role], *args, timeout=30)
    if docker("ps", "-aq") or docker("volume", "ls", "-q"):
        raise ValueError("runtime smoke left containers or volumes")
    return {**result, "image_ids": resolved, "initial_images": 0, "smoke": sorted(ROLES)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("--socket", type=Path, required=True)
    parser.add_argument("--data-root", type=Path, required=True)
    parser.add_argument("--sha256", required=True)
    parser.add_argument("--platform", choices=("linux/arm64", "linux/amd64"), required=True)
    for role in sorted(ROLES | {"sandbox"}):
        parser.add_argument("--" + role, required=role in ROLES)
    args = parser.parse_args()
    roles = {role: getattr(args, role) for role in sorted(ROLES | {"sandbox"}) if getattr(args, role) is not None}
    try:
        print(json.dumps(receiver(args.socket, args.data_root, args.archive, args.sha256, args.platform, roles), sort_keys=True))
    except (OSError, ValueError, KeyError, TypeError, AttributeError, RecursionError, subprocess.SubprocessError) as exc:
        print("trial cold receiver refused or failed: " + type(exc).__name__, file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
