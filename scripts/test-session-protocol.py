#!/usr/bin/env python3
"""Run ordinary session protocol integration tests with disposable Docker resources."""
import argparse
import json
from pathlib import Path
import re
import subprocess
import sys
import tempfile
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


def start_redis(docker, prefix, network, image, alias, instances):
    ident = docker.create("--name", prefix + "-" + alias, "--network", network, "--network-alias", alias,
                          "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
                          "--memory", "128m", "--tmpfs", "/data:rw,size=16m", "--entrypoint", "redis-server",
                          image, "--bind", "0.0.0.0", "--protected-mode", "no", "--save", "",
                          "--appendonly", "no", "--maxmemory", "32mb", "--maxmemory-policy", "noeviction")
    instances.append(ident)
    docker.run("start", ident)
    for _ in range(30):
        try:
            if docker.run("exec", ident, "redis-cli", "ping", timeout=5) == "PONG":
                return
        except subprocess.SubprocessError:
            pass
        time.sleep(0.2)
    raise RuntimeError(f"Redis fixture {alias} did not become ready")


def trial_tls(root):
    # Only disposable keys enter the two local containers. The enclosing host
    # directory stays private; fixture users inside the containers differ.
    mount = root / "mount"
    mount.mkdir(mode=0o755)

    def openssl(*args):
        subprocess.run(["openssl", *args], check=True, cwd=root, capture_output=True, timeout=15)

    openssl("req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1", "-subj", "/CN=Trial Fixture",
            "-keyout", "ca.key", "-out", "mount/api-ca.pem")
    (mount / "public-ca.pem").write_bytes((mount / "api-ca.pem").read_bytes())
    for role, extension in (
        ("public", "subjectAltName=DNS:oj.example.invalid\nextendedKeyUsage=serverAuth"),
        ("backend", "subjectAltName=DNS:oj-api.internal\nextendedKeyUsage=serverAuth"),
        ("proxy", "extendedKeyUsage=clientAuth"),
    ):
        (root / "extension").write_text(extension + "\n")
        openssl("req", "-new", "-newkey", "rsa:2048", "-nodes", "-subj", "/CN=Trial " + role,
                "-keyout", f"mount/{role}-key.pem", "-out", "leaf.csr")
        openssl("x509", "-req", "-in", "leaf.csr", "-CA", "mount/api-ca.pem", "-CAkey", "ca.key",
                "-CAcreateserial", "-days", "1", "-extfile", "extension", "-out", f"mount/{role}-chain.pem")
    for path in mount.iterdir():
        path.chmod(0o644)
    return mount


def run_trial_edge(docker, options, toolchain, nginx, assets, prefix):
    with tempfile.TemporaryDirectory(prefix="oj-trial-edge-") as tmp:
        tls = trial_tls(Path(tmp))
        test = docker.create(*options, "--mount", f"type=bind,src={tls},dst=/trial-tls,readonly",
                             "--entrypoint", "sleep", toolchain, "infinity")
        docker.run("start", test)
        edge = docker.create("--name", prefix + "-edge", "--network", f"container:{test}",
                             "--read-only", "--tmpfs", "/tmp:rw,size=16m,mode=1777",
                             "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
                             "--mount", f"type=bind,src={tls},dst=/run/oj-edge,readonly",
                             "--mount", f"type=bind,src={assets},dst=/srv/oj/site,readonly",
                             "--mount", f"type=bind,src={ROOT}/deploy/trial/nginx.conf.example,dst=/trial.conf,readonly",
                             "--entrypoint", "sh", nginx, "-ec",
                             'nginx -t -c /trial.conf; exec nginx -c /trial.conf -g "daemon off;"')
        docker.run("start", edge)
        docker.run("exec", test, "sh", "/workspace/scripts/test-session-protocol.sh", "trial-edge",
                   timeout=300, capture=False)
        check_state(docker, test, True)
        check_state(docker, edge, True)
        docker.run("stop", "--signal", "SIGQUIT", "--time", "5", edge, timeout=15)
        check_state(docker, edge, False)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--context", default="desktop-linux", help="Docker context (default: desktop-linux); empty string uses the CLI default")
    parser.add_argument("--toolchain-image", default="oj-go-quality-toolchain")
    parser.add_argument("--redis-image", default="redis:7.2.17@sha256:0312984fbfeb2ff1d4382ecef45b50e7d65478a6a58cf12ce28d6de7a4142e45", help="Existing local Redis image; never pulled")
    parser.add_argument("--build-cache-volume", default="onlinejudge_toolchain-cache", help="Existing Go build cache volume; empty string uses container /tmp")
    parser.add_argument("--module-cache", type=Path, help="Optional existing checksum-verified Go module cache; mounted read-only")
    parser.add_argument("--suite", choices=("ordinary", "email-lifecycle", "trial-edge"), default="ordinary",
                        help="Select one suite per fresh PostgreSQL and Redis fixture")
    parser.add_argument("--nginx-image", help="Existing local non-root Nginx image for trial-edge")
    parser.add_argument("--trial-site", type=Path, help="Exported production frontend site directory for trial-edge")
    args = parser.parse_args()
    trial = args.suite == "trial-edge"
    if (trial and not (args.nginx_image and args.trial_site)) or (
            not trial and (args.nginx_image or args.trial_site)):
        parser.error("trial-edge requires --nginx-image and --trial-site")
    if "," in str(ROOT) or (args.build_cache_volume and not re.fullmatch(r"[a-zA-Z0-9][a-zA-Z0-9_.-]*", args.build_cache_volume)):
        parser.error("invalid bind path or cache volume name")
    owner = uuid.uuid4().hex
    prefix = "oj-protocol-" + owner
    docker = Docker(args.context, owner)
    redis = []
    success = False
    try:
        toolchain, tool_arch = image_info(docker, args.toolchain_image)
        redis_image, redis_arch = image_info(docker, args.redis_image)
        host_arch = docker.run("info", "--format", "{{.Architecture}}")
        host_arch = {"x86_64": "amd64", "aarch64": "arm64"}.get(host_arch, host_arch)
        if tool_arch != redis_arch or tool_arch != host_arch:
            raise ValueError(f"image/daemon architecture mismatch: {tool_arch}, {redis_arch}, {host_arch}")
        if trial:
            nginx, nginx_arch = image_info(docker, args.nginx_image)
            assets = args.trial_site.resolve(strict=True)
            if nginx_arch != host_arch or "," in str(assets) or not (assets / "index.html").is_file():
                raise ValueError("invalid trial frontend or Nginx platform")
        if args.build_cache_volume:
            docker.run("volume", "inspect", args.build_cache_volume)
        network = docker.network(prefix)
        aliases = ("redis", "redis-recovery", "redis-recovery-interrupted") if args.suite == "ordinary" else ("redis",)
        for alias in aliases:
            start_redis(docker, prefix, network, redis_image, alias, redis)
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
        if trial:
            run_trial_edge(docker, options, toolchain, nginx, assets, prefix)
        else:
            test = docker.create(*options, "--entrypoint", "sh", toolchain, "/workspace/scripts/test-session-protocol.sh", args.suite)
            docker.run("start", "-a", test, timeout=300, capture=False)
            check_state(docker, test, False)
        for ident in redis:
            check_state(docker, ident, True)
            docker.run("stop", "--time", "5", ident, timeout=15)
            check_state(docker, ident, False)
        success = True
    except (OSError, subprocess.SubprocessError, ValueError, RuntimeError) as exc:
        print(f"session protocol test failed: {type(exc).__name__}: {exc}", file=sys.stderr)
    finally:
        if not success:
            for ident in redis:
                try:
                    docker.run("logs", ident, timeout=10, capture=False)
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
