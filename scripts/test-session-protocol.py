#!/usr/bin/env python3
"""Run ordinary session protocol integration tests with disposable Docker resources."""
import argparse
import json
from pathlib import Path
import re
import subprocess
import sys
import time
import uuid

ROOT = Path(__file__).resolve().parents[1]
LABEL = "oj.session-protocol.owner"


class Docker:
    def __init__(self, context, owner):
        self.command = ["docker"] + (["--context", context] if context else [])
        self.owner = owner
        self.containers = []
        self.networks = []

    def run(self, *args, timeout=30, capture=True):
        result = subprocess.run([*self.command, *args], check=True, text=True,
                                capture_output=capture, timeout=timeout)
        return result.stdout.strip() if capture else ""

    def create(self, *args):
        ident = self.run("create", "--label", LABEL + "=" + self.owner, *args)
        self.containers.append(ident)
        return ident

    def network(self, name):
        ident = self.run("network", "create", "--internal", "--label",
                         LABEL + "=" + self.owner, name)
        self.networks.append(ident)
        return ident

    def cleanup(self):
        errors = []
        for kind, ids in (("container", self.containers), ("network", self.networks)):
            for ident in reversed(ids):
                try:
                    labels = json.loads(self.run(kind, "inspect", "--format",
                                                 "{{json .Config.Labels}}" if kind == "container"
                                                 else "{{json .Labels}}", ident, timeout=10))
                    if not isinstance(labels, dict) or labels.get(LABEL) != self.owner:
                        raise RuntimeError("owner label mismatch; retained")
                    self.run(kind, "rm", *(["-f"] if kind == "container" else []), ident, timeout=15)
                except (OSError, subprocess.SubprocessError, ValueError, RuntimeError) as exc:
                    errors.append(f"{kind} {ident}: {type(exc).__name__}: {exc}")
        return errors


def image_info(docker, name):
    parts = docker.run("image", "inspect", "--format", "{{.Id}} {{.Os}} {{.Architecture}}", name).split()
    if len(parts) != 3 or not re.fullmatch(r"sha256:[0-9a-f]{64}", parts[0]) or parts[1] != "linux":
        raise ValueError(f"invalid or non-Linux local image: {name}")
    return parts[0], parts[2]


def check_state(docker, ident, running):
    state = json.loads(docker.run("inspect", "--format", "{{json .State}}", ident))
    if (state.get("Running") is not running or state.get("OOMKilled") is not False or
            (not running and (type(state.get("ExitCode")) is not int or state["ExitCode"] != 0))):
        raise RuntimeError(f"unexpected container state for {ident}: {state}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--context", default="desktop-linux", help="Docker context (default: desktop-linux); empty string uses the CLI default")
    parser.add_argument("--toolchain-image", default="oj-go-quality-toolchain")
    parser.add_argument("--redis-image", default="redis:7.2.17@sha256:0312984fbfeb2ff1d4382ecef45b50e7d65478a6a58cf12ce28d6de7a4142e45", help="Existing local Redis image; never pulled")
    parser.add_argument("--build-cache-volume", default="onlinejudge_toolchain-cache", help="Existing Go build cache volume; empty string uses container /tmp")
    parser.add_argument("--module-cache", type=Path, help="Optional existing checksum-verified Go module cache; mounted read-only")
    args = parser.parse_args()
    if "," in str(ROOT) or (args.build_cache_volume and not re.fullmatch(r"[a-zA-Z0-9][a-zA-Z0-9_.-]*", args.build_cache_volume)):
        parser.error("invalid bind path or cache volume name")
    owner = uuid.uuid4().hex
    prefix = "oj-protocol-" + owner
    docker = Docker(args.context, owner)
    redis = None
    success = False
    try:
        toolchain, tool_arch = image_info(docker, args.toolchain_image)
        redis_image, redis_arch = image_info(docker, args.redis_image)
        host_arch = docker.run("info", "--format", "{{.Architecture}}")
        host_arch = {"x86_64": "amd64", "aarch64": "arm64"}.get(host_arch, host_arch)
        if tool_arch != redis_arch or tool_arch != host_arch:
            raise ValueError(f"image/daemon architecture mismatch: {tool_arch}, {redis_arch}, {host_arch}")
        if args.build_cache_volume:
            docker.run("volume", "inspect", args.build_cache_volume)
        network = docker.network(prefix)
        redis = docker.create("--name", prefix + "-redis", "--network", network, "--network-alias", "redis",
                              "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
                              "--memory", "128m", "--tmpfs", "/data:rw,size=16m", "--entrypoint", "redis-server",
                              redis_image, "--bind", "0.0.0.0", "--protected-mode", "no", "--save", "",
                              "--appendonly", "no", "--maxmemory", "32mb", "--maxmemory-policy", "noeviction")
        docker.run("start", redis)
        for _ in range(30):
            try:
                if docker.run("exec", redis, "redis-cli", "ping", timeout=5) == "PONG":
                    break
            except subprocess.SubprocessError:
                pass
            time.sleep(0.2)
        else:
            raise RuntimeError("Redis did not become ready")
        options = ["--name", prefix + "-test", "--network", network, "--cpus", "2", "--memory", "2g",
                   "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
                   "--mount", f"type=bind,src={ROOT},dst=/workspace,readonly",
                   "-e", "GOPROXY=off", "-e", "GOTOOLCHAIN=local", "-e", "GOCACHE=/tmp/go-build"]
        if args.build_cache_volume:
            options += ["--mount", f"type=volume,src={args.build_cache_volume},dst=/tmp/go-build"]
        if args.module_cache:
            cache = args.module_cache.resolve(strict=True)
            if not cache.is_dir() or "," in str(cache):
                raise ValueError("invalid existing module cache directory")
            options += ["--mount", f"type=bind,src={cache},dst=/verified-module-cache,readonly"]
        test = docker.create(*options, "--entrypoint", "sh", toolchain, "/workspace/scripts/test-session-protocol.sh")
        docker.run("start", "-a", test, timeout=300, capture=False)
        check_state(docker, test, False)
        check_state(docker, redis, True)
        docker.run("stop", "--time", "5", redis, timeout=15)
        check_state(docker, redis, False)
        success = True
    except (OSError, subprocess.SubprocessError, ValueError, RuntimeError) as exc:
        print(f"session protocol test failed: {type(exc).__name__}: {exc}", file=sys.stderr)
    finally:
        if redis and not success:
            try:
                docker.run("logs", redis, timeout=10, capture=False)
            except (OSError, subprocess.SubprocessError):
                print("Unable to retrieve Redis logs", file=sys.stderr)
        errors = docker.cleanup()
        for error in errors:
            print("cleanup failed: " + error, file=sys.stderr)
        if errors:
            success = False
    return 0 if success else 1


if __name__ == "__main__":
    sys.exit(main())
