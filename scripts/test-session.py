#!/usr/bin/env python3
"""Run the test-only session experiment in disposable, private Docker resources."""
import argparse
from pathlib import Path
import subprocess
import time
import uuid


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--context", default="desktop-linux")
    parser.add_argument("--toolchain-image", default="oj-toolchain:local")
    parser.add_argument("--redis-image", default=
                        "sha256:92dbfb0bc408385074c83eb3a868b5f273d425f079f2af808403d22dcd94b667",
                        help="Already available Redis image; never pulled automatically")
    parser.add_argument("--module-cache", required=True,
                        help="Go module cache prepared with normal TLS and checksum verification")
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    cache = Path(args.module_cache).resolve(strict=True)
    prefix = "oj-session-" + uuid.uuid4().hex[:12]
    docker = ["docker", "--context", args.context]
    # Existing local fixture, no image pull and no Redis Stack modules loaded.
    redis_image = args.redis_image
    created_network = False
    created_redis = False
    started_test = False
    def run(*command, **kwargs):
        return subprocess.run([*docker, *command], check=True, **kwargs)
    try:
        run("image", "inspect", args.toolchain_image, stdout=subprocess.DEVNULL)
        run("image", "inspect", redis_image, stdout=subprocess.DEVNULL)
        run("network", "create", "--internal", prefix, stdout=subprocess.DEVNULL)
        created_network = True
        run("run", "-d", "--name", prefix, "--network", prefix,
            "--network-alias", "redis", "--read-only", "--cap-drop", "ALL",
            "--security-opt", "no-new-privileges", "--memory", "128m",
            "--tmpfs", "/data:rw,size=16m", "--entrypoint", "redis-server", redis_image,
            "--bind", "0.0.0.0", "--protected-mode", "no", "--save", "",
            "--appendonly", "no", "--maxmemory", "32mb", "--maxmemory-policy", "noeviction",
            stdout=subprocess.DEVNULL)
        created_redis = True
        # No published ports, credentials, user database or Docker socket mount.
        for attempt in range(30):
            ready = subprocess.run([*docker, "exec", prefix, "redis-cli", "ping"],
                                   capture_output=True, text=True, timeout=5)
            if ready.returncode == 0 and ready.stdout.strip() == "PONG":
                break
            if attempt == 29:
                raise RuntimeError("disposable Redis did not become ready")
            time.sleep(0.2)
        started_test = True
        run("run", "--rm", "--name", prefix + "-test", "--network", prefix,
            "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
            "--mount", f"type=bind,src={root / 'tests/sessionexperiment'},dst=/experiment,readonly",
            "--mount", f"type=bind,src={cache},dst=/go/pkg/mod,readonly",
            "-e", "OJ_SESSION_REDIS_ADDR=redis:6379", "-e", "GOPROXY=off",
            "-e", "GOTOOLCHAIN=local",
            "--entrypoint", "sh", args.toolchain_image, "-ec",
            "cp -R /experiment /tmp/session-test; cd /tmp/session-test; "
            "test -z \"$(gofmt -l .)\"; go mod verify; go vet ./...; "
            "go test -mod=readonly -race -count=1 -timeout=90s -v ./...", timeout=300)
    finally:
        # Only unique resources created by this invocation are removed.
        if started_test:
            subprocess.run([*docker, "rm", "-f", prefix + "-test"], check=False,
                           stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=15)
        if created_redis:
            subprocess.run([*docker, "rm", "-f", prefix], check=False, stdout=subprocess.DEVNULL)
        if created_network:
            subprocess.run([*docker, "network", "rm", prefix], check=False, stdout=subprocess.DEVNULL)


if __name__ == "__main__":
    main()
