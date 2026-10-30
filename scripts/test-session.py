#!/usr/bin/env python3
"""Run the test-only session experiment in disposable, private Docker resources."""
import argparse
import os
from pathlib import Path
import selectors
import subprocess
import sys
import tempfile
import time
import uuid


def wait_for_redis(docker, name):
    for attempt in range(30):
        ready = subprocess.run([*docker, "exec", name, "redis-cli", "ping"],
                               capture_output=True, text=True, timeout=5)
        if ready.returncode == 0 and ready.stdout.strip() == "PONG":
            return
        if attempt == 29:
            raise RuntimeError("disposable Redis did not become ready")
        time.sleep(0.2)


def run_test(docker, command, recovery_name, test_name, nonce):
    # Only an exact one-use nonce can request the fixed fault injection. Neither
    # the test nor its output can select a container, host path or shell command.
    request = ("OJ_SESSION_RESTORE_REQUEST " + nonce).encode()
    restored = False
    deadline = time.monotonic() + 300
    with subprocess.Popen([*docker, *command], stdout=subprocess.PIPE,
                          stderr=subprocess.STDOUT) as process:
        try:
            with selectors.DefaultSelector() as selector:
                selector.register(process.stdout, selectors.EVENT_READ)
                pending = b""
                while selector.get_map():
                    if time.monotonic() >= deadline:
                        raise TimeoutError("session experiment exceeded 300 seconds")
                    if not selector.select(timeout=0.2):
                        continue
                    chunk = os.read(process.stdout.fileno(), 65536)
                    if not chunk:
                        selector.unregister(process.stdout)
                        break
                    sys.stdout.buffer.write(chunk)
                    sys.stdout.buffer.flush()
                    pending += chunk
                    while b"\n" in pending:
                        line, pending = pending.split(b"\n", 1)
                        if line.rstrip(b"\r") != request:
                            continue
                        if restored:
                            raise RuntimeError("duplicate snapshot restore request")
                        # SIGKILL prevents shutdown from overwriting the older
                        # SAVE. The private bind mount survives container restart.
                        subprocess.run([*docker, "kill", "--signal", "KILL", recovery_name],
                                       check=True, stdout=subprocess.DEVNULL, timeout=15)
                        subprocess.run([*docker, "start", recovery_name], check=True,
                                       stdout=subprocess.DEVNULL, timeout=15)
                        wait_for_redis(docker, recovery_name)
                        subprocess.run([*docker, "exec", test_name, "touch",
                                        "/tmp/oj-session-restored-" + nonce],
                                       check=True, timeout=5)
                        restored = True
                status = process.wait(timeout=max(0.1, deadline - time.monotonic()))
                if status:
                    raise subprocess.CalledProcessError(status, process.args)
                if not restored:
                    raise RuntimeError("snapshot recovery scenario did not execute")
        finally:
            if process.poll() is None:
                process.kill()
                process.wait(timeout=10)


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
    nonce = uuid.uuid4().hex
    recovery_name = prefix + "-recovery"
    snapshot_dir = tempfile.TemporaryDirectory(prefix="oj-session-rdb-")
    docker = ["docker", "--context", args.context]
    # Existing local fixture, no image pull and no Redis Stack modules loaded.
    redis_image = args.redis_image
    created_network = False
    created_redis = False
    created_recovery = False
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
        run("run", "-d", "--name", recovery_name, "--network", prefix,
            "--network-alias", "redis-recovery", "--read-only", "--cap-drop", "ALL",
            "--security-opt", "no-new-privileges", "--memory", "128m",
            "--mount", f"type=bind,src={snapshot_dir.name},dst=/data",
            "--entrypoint", "redis-server", redis_image,
            "--bind", "0.0.0.0", "--protected-mode", "no", "--dir", "/data",
            "--dbfilename", "dump.rdb", "--save", "", "--appendonly", "no",
            "--maxmemory", "32mb", "--maxmemory-policy", "noeviction",
            stdout=subprocess.DEVNULL)
        created_recovery = True
        # No published ports, credentials, user database or Docker socket mount.
        wait_for_redis(docker, prefix)
        wait_for_redis(docker, recovery_name)
        started_test = True
        command = ["run", "--rm", "--name", prefix + "-test", "--network", prefix,
            "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
            "--mount", f"type=bind,src={root / 'tests/sessionexperiment'},dst=/experiment,readonly",
            "--mount", f"type=bind,src={cache},dst=/go/pkg/mod,readonly",
            "-e", "OJ_SESSION_REDIS_ADDR=redis:6379", "-e", "GOPROXY=off",
            "-e", "OJ_SESSION_RECOVERY_REDIS_ADDR=redis-recovery:6379",
            "-e", "OJ_SESSION_RECOVERY_NONCE=" + nonce,
            "-e", "GOTOOLCHAIN=local",
            "--entrypoint", "sh", args.toolchain_image, "-ec",
            "cp -R /experiment /tmp/session-test; cd /tmp/session-test; "
            "sh run.sh"]
        run_test(docker, command, recovery_name, prefix + "-test", nonce)
    finally:
        # Only unique resources created by this invocation are removed.
        if started_test:
            subprocess.run([*docker, "rm", "-f", prefix + "-test"], check=False,
                           stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=15)
        if created_redis:
            subprocess.run([*docker, "rm", "-f", prefix], check=False, stdout=subprocess.DEVNULL)
        if created_recovery:
            subprocess.run([*docker, "rm", "-f", recovery_name], check=False, stdout=subprocess.DEVNULL)
        if created_network:
            subprocess.run([*docker, "network", "rm", prefix], check=False, stdout=subprocess.DEVNULL)
        snapshot_dir.cleanup()


if __name__ == "__main__":
    main()
