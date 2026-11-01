#!/usr/bin/env python3
"""Verify the browser account panel using existing local tools and isolated stores."""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import re
import shutil
import ssl
import subprocess
import sys
import time
import urllib.request
import uuid

ROOT = Path(__file__).resolve().parents[1]
REDIS = "redis:7.2.17@sha256:0312984fbfeb2ff1d4382ecef45b50e7d65478a6a58cf12ce28d6de7a4142e45"
# Loading the shared driver must not leave bytecode artifacts in the checkout.
sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("session_protocol", ROOT / "scripts/test-session-protocol.py")
protocol = importlib.util.module_from_spec(spec)
spec.loader.exec_module(protocol)


class Parser(argparse.ArgumentParser):
    def error(self, message):
        raise ValueError("invalid command arguments")


class LoggedDocker(protocol.Docker):
    def __init__(self, context, owner, log):
        super().__init__(context, owner)
        self.log = log

    def run(self, *args, **kwargs):
        try:
            output = super().run(*args, **kwargs)
            self.log.write(output + "\n")
            self.log.flush()
            return output
        except subprocess.SubprocessError as error:
            self.log.write(str(getattr(error, "stdout", "") or "") + str(getattr(error, "stderr", "") or "") + "\n")
            self.log.flush()
            raise


def local_path(value, directory=False, executable=False):
    path = value.resolve(strict=True)
    if (directory and not path.is_dir()) or (not directory and not path.is_file()) or "," in str(path):
        raise ValueError("invalid local path")
    if executable and not os.access(path, os.X_OK):
        raise ValueError("local executable unavailable")
    return path


def arguments():
    parser = Parser(description=__doc__)
    parser.add_argument("--context", default="desktop-linux")
    parser.add_argument("--toolchain-image", default="oj-go-quality-toolchain")
    parser.add_argument("--frontend-image", default="oj-browser-frontend-source")
    parser.add_argument("--redis-image", default=REDIS, choices=[REDIS])
    parser.add_argument("--module-cache", type=Path)
    parser.add_argument("--build-cache-volume", default="onlinejudge_toolchain-cache")
    parser.add_argument("--playwright-module", type=Path, required=True)
    parser.add_argument("--chromium-executable", type=Path)
    parser.add_argument("--output-dir", type=Path, required=True)
    args = parser.parse_args()
    output = args.output_dir.resolve()
    if args.output_dir.is_symlink() or output.exists() or output.is_relative_to(ROOT) or not output.parent.is_dir() or "," in str(output) or "," in str(ROOT):
        raise ValueError("output must be a new directory outside the repository")
    if args.build_cache_volume and not re.fullmatch(r"[a-zA-Z0-9][a-zA-Z0-9_.-]*", args.build_cache_volume):
        raise ValueError("invalid cache volume")
    module = args.playwright_module.resolve(strict=True)
    package = module / "package.json" if module.is_dir() else module.parent / "package.json"
    if not (module.is_dir() or module.is_file()) or json.loads(package.read_text())["name"] not in {"playwright", "playwright-core", "@playwright/test"}:
        raise ValueError("existing Playwright package or entry required")
    args.playwright_module = module
    if args.module_cache:
        args.module_cache = local_path(args.module_cache, directory=True)
    if args.chromium_executable:
        args.chromium_executable = local_path(args.chromium_executable, executable=True)
    node = shutil.which("node")
    if not node:
        raise ValueError("existing host Node required")
    args.node = local_path(Path(node), executable=True)
    args.output_dir = output
    return args


def fixture_script():
    source = (ROOT / "scripts/test-session-protocol.sh").read_text()
    replacements = {
        "--build-tags=integration ./tests/sessionprotocol/...": "--build-tags=integration,browser ./tests/sessionprotocol/...",
        "go test -mod=readonly -race -count=1 -timeout=120s -tags=integration ./tests/sessionprotocol -v":
            "go test -mod=readonly -race -count=1 -timeout=17m -tags=integration,browser ./tests/sessionprotocol -run '^TestBrowserFixture$' -v",
    }
    for before, after in replacements.items():
        if source.count(before) != 1:
            raise ValueError("protocol script entry changed")
        source = source.replace(before, after)
    return source


def published_origin(docker, test, port, scheme):
    ports = json.loads(docker.run("inspect", "--format", "{{json .NetworkSettings.Ports}}", test))
    bindings = ports.get(str(port) + "/tcp")
    if not isinstance(bindings, list) or len(bindings) != 1 or not isinstance(bindings[0], dict) or bindings[0].get("HostIp") != "127.0.0.1":
        raise RuntimeError("non-loopback or missing published port")
    value = bindings[0].get("HostPort", "")
    if not re.fullmatch(r"[1-9][0-9]{0,4}", value) or int(value) > 65535:
        raise RuntimeError("invalid published port")
    return scheme + "://127.0.0.1:" + value


def wait_ready(process, log, origin):
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        if process.poll() is not None:
            raise RuntimeError("fixture exited before readiness")
        for line in log.read_text(errors="replace").splitlines():
            if line.startswith("BROWSER_READY "):
                ready = json.loads(line.removeprefix("BROWSER_READY "))
                if not isinstance(ready, dict) or ready.get("url") != origin or not re.fullmatch(r"[a-zA-Z0-9._+-]+@example\.invalid", ready.get("email", "")) or not re.fullmatch(r"[a-f0-9]{32}", ready.get("subject_id", "")):
                    raise RuntimeError("invalid fixture readiness")
                return ready
        time.sleep(0.5)
    raise RuntimeError("fixture readiness timed out")


def stop_fixture(origin):
    if origin:
        try:
            request = urllib.request.Request(origin + "/__fixture/stop", method="POST")
            # Ephemeral loopback test certificates are the only exception here.
            opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPSHandler(context=ssl._create_unverified_context()))
            with opener.open(request, timeout=3):
                pass
        except (OSError, ValueError):
            pass


def main():
    try:
        args = arguments()
    except (OSError, ValueError, KeyError, TypeError):
        print("FAIL PARAMETERS", file=sys.stderr)
        return 1
    out = args.output_dir
    try:
        out.mkdir(mode=0o700)
        log = (out / "driver.log").open("w")
    except OSError:
        print("FAIL OUTPUT", file=sys.stderr)
        return 1
    docker = LoggedDocker(args.context, uuid.uuid4().hex, log)
    prefix = "oj-browser-" + docker.owner
    phase, failure, origin, process = "PREFLIGHT", None, None, None
    containers, handles = {}, []
    result = {"success": False, "browser_ok": False, "fixture_ok": False, "container_ok": False, "redis_ok": False}
    try:
        print("Checking existing local tools and images", flush=True)
        version = subprocess.run([str(args.node), "--version"], check=True, capture_output=True, text=True, timeout=10).stdout.strip()
        if not re.fullmatch(r"v(?:2[2-9]|[3-9][0-9])\.[0-9]+\.[0-9]+", version):
            raise ValueError("host Node version unsupported")
        images = [protocol.image_info(docker, name) for name in (args.toolchain_image, args.frontend_image, args.redis_image)]
        arch = docker.run("info", "--format", "{{.Architecture}}")
        arch = {"x86_64": "amd64", "aarch64": "arm64"}.get(arch, arch)
        if any(item[1] != arch for item in images):
            raise ValueError("local image architecture mismatch")
        if args.build_cache_volume:
            docker.run("volume", "inspect", args.build_cache_volume)
        script = fixture_script()
        for name in ("src", "scripts", "index.html", "vite.config.ts", "tsconfig.json", "eslint.config.js", "package.json", "pnpm-lock.yaml", "pnpm-workspace.yaml", ".npmrc"):
            if not (ROOT / "web" / name).exists():
                raise ValueError("frontend source incomplete")
        expected_checks = re.findall(r"^\s+passed\('([^'\n]+)'\)", (ROOT / "tests/browser/account-qa.mjs").read_text(), re.M)
        if len(expected_checks) != 10 or len(set(expected_checks)) != 10:
            raise ValueError("browser driver check contract changed")
        phase = "FRONTEND"
        print("Building assets with cached dependencies", flush=True)
        assets = out / "assets"
        assets.mkdir(mode=0o755)
        build = """set -eu
cmp package.json /workspace/web/package.json
cmp pnpm-lock.yaml /workspace/web/pnpm-lock.yaml
rm -rf src scripts dist
cp -R /workspace/web/src /workspace/web/scripts .
for file in index.html vite.config.ts tsconfig.json eslint.config.js package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc; do cp /workspace/web/$file .; done
pnpm build
cp -R dist/. /out/
"""
        containers["frontend"] = docker.create("--pull", "never", "--network", "none", "--cpus", "2", "--memory", "2g", "--cap-drop", "ALL",
            "--security-opt", "no-new-privileges", "--workdir", "/src/web", "--mount", f"type=bind,src={ROOT},dst=/workspace,readonly",
            "--mount", f"type=bind,src={assets},dst=/out", "--entrypoint", "sh", images[1][0], "-ec", build)
        docker.run("start", "-a", containers["frontend"], timeout=90)
        protocol.check_state(docker, containers["frontend"], False)
        phase = "FIXTURE_START"
        network = docker.run("network", "create", "--driver", "bridge", "--label", protocol.LABEL + "=" + docker.owner, prefix)
        docker.networks.append(network)
        containers["redis"] = docker.create("--pull", "never", "--network", network, "--network-alias", "redis", "--read-only", "--cap-drop", "ALL",
            "--security-opt", "no-new-privileges", "--memory", "128m", "--tmpfs", "/data:rw,size=16m", "--entrypoint", "redis-server", images[2][0],
            "--bind", "0.0.0.0", "--protected-mode", "no", "--save", "", "--appendonly", "no", "--maxmemory", "32mb", "--maxmemory-policy", "noeviction")
        docker.run("start", containers["redis"])
        for _ in range(30):
            try:
                if docker.run("exec", containers["redis"], "redis-cli", "ping", timeout=5) == "PONG":
                    break
            except subprocess.SubprocessError:
                pass
            time.sleep(0.2)
        else:
            raise RuntimeError("fixture Redis unavailable")
        script_path = out / "browser-fixture.sh"
        script_path.write_text(script)
        script_path.chmod(0o644)
        options = ["--network", network, "--cpus", "2", "--memory", "2g", "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
            "--publish", "127.0.0.1::9443", "--publish", "127.0.0.1::9444", "--mount", f"type=bind,src={ROOT},dst=/workspace,readonly",
            "--mount", f"type=bind,src={assets},dst=/browser-assets,readonly", "--mount", f"type=bind,src={script_path},dst=/browser-fixture.sh,readonly",
            "-e", "GOPROXY=off", "-e", "GOTOOLCHAIN=local", "-e", "GOCACHE=/tmp/go-build"]
        if args.build_cache_volume:
            options += ["--mount", f"type=volume,src={args.build_cache_volume},dst=/tmp/go-build"]
        if args.module_cache:
            options += ["--mount", f"type=bind,src={args.module_cache},dst=/verified-module-cache,readonly"]
        containers["test"] = docker.create("--pull", "never", *options, "--entrypoint", "sh", images[0][0], "-ec", "while :; do sleep 30; done")
        docker.run("start", containers["test"])
        origin = published_origin(docker, containers["test"], 9443, "https")
        http_origin = published_origin(docker, containers["test"], 9444, "http")
        fixture_log = out / "fixture.log"
        handles.append(fixture_log.open("w"))
        process = subprocess.Popen([*docker.command, "exec", "-e", "OJ_BROWSER_ORIGIN=" + origin, "-e", "OJ_BROWSER_UI_DIR=/browser-assets", containers["test"], "sh", "/browser-fixture.sh"], stdout=handles[-1], stderr=subprocess.STDOUT)
        ready = wait_ready(process, fixture_log, origin)
        info = out / "fixture.json"
        info.write_text(json.dumps({"origin": origin, "http_origin": http_origin, "log": str(fixture_log), "email": ready["email"], "subject_id": ready["subject_id"]}, indent=2) + "\n")
        phase = "BROWSER_QA"
        print("Running real browser account checks", flush=True)
        env = {key: os.environ[key] for key in ("PATH", "HOME", "USER", "TMPDIR", "TMP", "TEMP", "LANG", "LC_ALL") if key in os.environ}
        env.update(OJ_BROWSER_QA_INFO=str(info), OJ_BROWSER_QA_OUTPUT=str(out), OJ_PLAYWRIGHT_MODULE=str(args.playwright_module), PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD="1")
        if args.chromium_executable:
            env["OJ_CHROMIUM_EXECUTABLE"] = str(args.chromium_executable)
        with (out / "node.log").open("w") as node_log:
            qa = subprocess.run([str(args.node), str(ROOT / "tests/browser/account-qa.mjs")], env=env, stdout=node_log, stderr=subprocess.STDOUT, timeout=120)
        report = json.loads((out / "browser-qa.json").read_text())
        result["browser_ok"] = qa.returncode == 0 and isinstance(report, dict) and report.get("success") is True and report.get("checks") == expected_checks
        if not result["browser_ok"]:
            failure = phase
    except (OSError, subprocess.SubprocessError, ValueError, KeyError, TypeError, RuntimeError, KeyboardInterrupt) as error:
        failure = "INTERRUPTED" if isinstance(error, KeyboardInterrupt) else phase
        log.write(type(error).__name__ + "\n" + str(getattr(error, "stdout", "") or "") + str(getattr(error, "stderr", "") or ""))
    finally:
        stop_fixture(origin)
        if process:
            try:
                result["fixture_exit"] = process.wait(timeout=15)
                result["fixture_ok"] = result["fixture_exit"] == 0
            except subprocess.TimeoutExpired:
                process.terminate()
                try:
                    process.wait(timeout=3)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
            if not result["fixture_ok"]:
                failure = failure or "FIXTURE_SHUTDOWN"
        try:
            if "test" in containers:
                protocol.check_state(docker, containers["test"], True)
                result["container_ok"] = True
            if "redis" in containers:
                protocol.check_state(docker, containers["redis"], True)
                docker.run("stop", "--time", "5", containers["redis"], timeout=15)
                protocol.check_state(docker, containers["redis"], False)
                result["redis_ok"] = True
        except (OSError, subprocess.SubprocessError, ValueError, RuntimeError):
            failure = failure or "CONTAINER_STATE"
        for name, ident in containers.items():
            try:
                with (out / (name + "-container.log")).open("w") as container_log:
                    subprocess.run([*docker.command, "logs", ident], stdout=container_log, stderr=subprocess.STDOUT, check=True, timeout=10)
            except (OSError, subprocess.SubprocessError):
                failure = failure or "LOG_CAPTURE"
        errors = docker.cleanup()
        (out / "cleanup.log").write_text("\n".join(errors) + ("\n" if errors else ""))
        for handle in handles:
            handle.close()
        result["cleanup_ok"] = not errors
        result["success"] = not failure and all(result[key] for key in ("browser_ok", "fixture_ok", "container_ok", "redis_ok", "cleanup_ok"))
        result["failure"] = "CLEANUP" if errors else failure
        (out / "result.json").write_text(json.dumps(result, indent=2) + "\n")
        log.close()
    print(("PASS" if result["success"] else "FAIL " + str(result["failure"])) + " " + str(out), flush=True)
    return 0 if result["success"] else 1


if __name__ == "__main__":
    sys.exit(main())
