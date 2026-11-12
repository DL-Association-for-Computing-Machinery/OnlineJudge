#!/usr/bin/env python3
"""Build on Docker Desktop, then drive owned offline sandboxes from the host.

Only Python and Docker are required on the host. No Docker socket enters images.
The exported host executables are synthetic test drivers, not Linux sandboxes.
"""
import argparse
import fcntl
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import time
import uuid

ROOT = Path(__file__).resolve().parent.parent
DOCKER = ["docker", "--context", "desktop-linux"]
OWNER = "onlinejudge.desktop-test"
SLOTS = ["oj-m0-sandbox", "oj-m0-sandbox-secondary"]


def browser_environment(args):
    supplied = any((args.playwright_module, args.chromium_executable, args.output_dir))
    if args.suite != "browser-loop":
        if supplied:
            raise ValueError("browser options require browser-loop")
        return {}
    if not args.playwright_module or not args.output_dir:
        raise ValueError("browser-loop requires existing Playwright and a new external output directory")
    module = args.playwright_module.resolve(strict=True)
    package = module / "package.json" if module.is_dir() else module.parent / "package.json"
    if json.loads(package.read_text()).get("name") not in {"playwright", "playwright-core", "@playwright/test"}:
        raise ValueError("existing Playwright package required")
    node = shutil.which("node")
    if not node:
        raise ValueError("existing host Node required; no dependencies are installed")
    output = args.output_dir.resolve()
    if args.output_dir.is_symlink() or output.exists() or output.is_relative_to(ROOT) or not output.parent.is_dir():
        raise ValueError("output must be a new directory outside this repository")
    env = {"OJ_PLAYWRIGHT_MODULE": str(module), "OJ_BROWSER_NODE": str(Path(node).resolve()), "OJ_BROWSER_OUTPUT": str(output)}
    if args.chromium_executable:
        binary = args.chromium_executable.resolve(strict=True)
        if not binary.is_file() or not os.access(binary, os.X_OK):
            raise ValueError("existing Chromium executable required")
        env["OJ_BROWSER_CHROMIUM"] = str(binary)
    output.mkdir(mode=0o700)
    return env


def docker(*args, capture=False):
    return subprocess.run(DOCKER + list(args), cwd=ROOT, check=True, text=True,
                          stdout=subprocess.PIPE if capture else None).stdout


def ready(name):
    docker("start", name)
    for _ in range(100):
        result = subprocess.run(DOCKER + ["exec", name, "curl", "-fsS", "--max-time", "1",
                                         "http://127.0.0.1:5050/version"],
                                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        if result.returncode == 0:
            return
        time.sleep(.1)
    raise RuntimeError("Sandbox readiness timeout")


def start_database(owned):
    name = "oj-worker-db-" + uuid.uuid4().hex[:12]
    docker("run", "-d", "--name", name, "--label", "owner=" + OWNER,
           "-p", "127.0.0.1::5432", "-e", "POSTGRES_USER=oj_test", "-e", "POSTGRES_HOST_AUTH_METHOD=trust",
           "--tmpfs", "/var/lib/postgresql/data", "postgres:14-alpine")
    owned.append(name)
    for _ in range(100):
        # The image briefly starts a socket-only server to initialize the DB.
        if subprocess.run(DOCKER + ["exec", name, "pg_isready", "-h", "127.0.0.1", "-U", "oj_test"],
                          stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0:
            return name, docker("port", name, "5432", capture=True).strip().split(":")[-1]
        time.sleep(.2)
    raise RuntimeError("PostgreSQL readiness timeout")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("suite", choices=["all", "sandbox", "report", "dual-worker", "worker-fault", "slot-recovery", "network-fault", "compile-lease", "resource-bounds", "http-loop", "browser-loop", "restore-loop"])
    parser.add_argument("--playwright-module", type=Path)
    parser.add_argument("--chromium-executable", type=Path)
    parser.add_argument("--output-dir", type=Path)
    args = parser.parse_args()
    if platform.system() != "Darwin" or platform.machine() != "arm64":
        raise RuntimeError("Pinned Desktop experiment requires macOS arm64; other profiles need validation")
    browser_env = browser_environment(args)
    docker("build", "--target", "host-artifacts", "--output", "type=local,dest=.local/bin", ".")
    if args.suite in {"all", "restore-loop"}:
        docker("build", "--target", "restore-artifacts", "--output", "type=local,dest=.local/bin", ".")
    docker("build", "--provenance=false", "-t", "oj-m0-sandbox:local", "-f",
           "deploy/experiments/go-judge.Dockerfile", "deploy/experiments")
    owned = []
    databases = []
    try:
        for slot in SLOTS:
            exists = subprocess.run(DOCKER + ["inspect", slot], stdout=subprocess.DEVNULL,
                                    stderr=subprocess.DEVNULL).returncode == 0
            if exists:
                label = docker("inspect", "--format", '{{index .Config.Labels "owner"}}', slot, capture=True).strip()
                running = docker("inspect", "--format", '{{.State.Running}}', slot, capture=True).strip()
                if label != OWNER or running != "false":
                    raise RuntimeError("Refusing to take over unowned or running sandbox: " + slot)
            else:
                docker("create", "--name", slot, "--label", "owner=" + OWNER,
                       "--privileged", "--network", "none", "--read-only", "--tmpfs",
                       "/tmp:rw,nosuid,size=128m", "--shm-size=128m", "--memory=1g",
                       "--cpus=1", "--pids-limit=128", "oj-m0-sandbox:local")
            owned.append(slot)
        db, port = start_database(databases)
        suites = ["sandbox", "report", "dual-worker", "worker-fault", "slot-recovery", "network-fault", "compile-lease", "resource-bounds", "http-loop", "restore-loop"] if args.suite == "all" else [args.suite]
        for i, suite in enumerate(suites):
            for slot in owned:
                ready(slot)
            database = "test_" + str(i)
            docker("exec", db, "createdb", "-U", "oj_test", database)
            env = {k: v for k, v in os.environ.items() if not k.startswith("OJ_")}
            env.update(OJ_SANDBOX_EXPERIMENT="1",
                       OJ_TEST_DATABASE_URL=f"host=127.0.0.1 port={port} user=oj_test dbname={database} sslmode=disable")
            env.update(browser_env)
            if suite == "restore-loop":
                target, _ = start_database(databases)
                docker("exec", target, "createdb", "-U", "oj_test", database)
                env.update(OJ_RESTORE_SOURCE_CONTAINER=db, OJ_RESTORE_TARGET_CONTAINER=target, OJ_RESTORE_DATABASE=database)
            selection = {"report": ("OJ_SANDBOX_REPORT", "TestExecutionReport"),
                         "dual-worker": ("OJ_SANDBOX_DUAL", "TestDualWorkerRecovery"),
                         "worker-fault": ("OJ_SANDBOX_FAULT", "TestWorkerFaults"),
                         "slot-recovery": ("OJ_SANDBOX_RECOVERY", "TestWorkerSlotRecovery"),
                         "network-fault": ("OJ_SANDBOX_NETWORK", "TestWorkerNetworkFaults"),
                         "compile-lease": ("OJ_SANDBOX_COMPILE_LEASE", "TestCompileLeaseFaults"),
                         "resource-bounds": ("OJ_SANDBOX_RESOURCES", "TestSandboxResourceBounds"),
                         "http-loop": ("OJ_SANDBOX_HTTP", "TestHTTPJudgeLoop"),
                         "restore-loop": ("OJ_SANDBOX_RESTORE", "TestRestoreJudgeLoop"),
                         "browser-loop": ("OJ_SANDBOX_BROWSER", "TestBrowserJudgeLoop")}
            command = [str(ROOT / ".local/bin/sandbox.test"), "-test.v", "-test.count=1", "-test.timeout=10m"]
            if suite in selection:
                flag, test = selection[suite]
                env[flag] = "1"
                command += ["-test.run=^" + test + "$"]
            subprocess.run(command, cwd=ROOT / "tests/sandbox", env=env, check=True)
    finally:
        for slot in owned:
            subprocess.run(DOCKER + ["stop", slot], check=False)
        for db in reversed(databases):
            subprocess.run(DOCKER + ["rm", "-f", db], check=False)


if __name__ == "__main__":
    ROOT.joinpath(".local").mkdir(exist_ok=True)
    # Serialize all checkouts sharing the fixed Desktop slots.
    with open(Path("/tmp") / ("oj-desktop-tests-" + str(os.getuid()) + ".lock"), "a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        main()
