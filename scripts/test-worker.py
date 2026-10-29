#!/usr/bin/env python3
"""Build on Docker Desktop, then drive owned offline sandboxes from the host.

Only Python and Docker are required on the host. No Docker socket enters images.
The exported host executables are synthetic test drivers, not Linux sandboxes.
"""
import argparse
import fcntl
import os
from pathlib import Path
import platform
import subprocess
import time
import uuid

ROOT = Path(__file__).resolve().parent.parent
DOCKER = ["docker", "--context", "desktop-linux"]
OWNER = "onlinejudge.desktop-test"
SLOTS = ["oj-m0-sandbox", "oj-m0-sandbox-secondary"]


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


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("suite", choices=["all", "sandbox", "report", "dual-worker", "worker-fault"])
    args = parser.parse_args()
    if platform.system() != "Darwin" or platform.machine() != "arm64":
        raise RuntimeError("Pinned Desktop experiment requires macOS arm64; other profiles need validation")
    docker("build", "--target", "host-artifacts", "--output", "type=local,dest=.local/bin", ".")
    docker("build", "--provenance=false", "-t", "oj-m0-sandbox:local", "-f",
           "deploy/experiments/go-judge.Dockerfile", "deploy/experiments")
    owned = []
    db = "oj-worker-db-" + uuid.uuid4().hex[:12]
    db_created = False
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
        docker("run", "-d", "--name", db, "-p", "127.0.0.1::5432",
               "-e", "POSTGRES_USER=oj_test", "-e", "POSTGRES_HOST_AUTH_METHOD=trust",
               "--tmpfs", "/var/lib/postgresql/data", "postgres:14-alpine")
        db_created = True
        for _ in range(100):
            if subprocess.run(DOCKER + ["exec", db, "pg_isready", "-U", "oj_test"],
                              stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0:
                break
            time.sleep(.2)
        else:
            raise RuntimeError("PostgreSQL readiness timeout")
        port = docker("port", db, "5432", capture=True).strip().split(":")[-1]
        suites = ["sandbox", "report", "dual-worker", "worker-fault"] if args.suite == "all" else [args.suite]
        for i, suite in enumerate(suites):
            for slot in owned:
                ready(slot)
            database = "test_" + str(i)
            docker("exec", db, "createdb", "-U", "oj_test", database)
            env = {k: v for k, v in os.environ.items() if not k.startswith("OJ_")}
            env.update(OJ_SANDBOX_EXPERIMENT="1",
                       OJ_TEST_DATABASE_URL=f"host=127.0.0.1 port={port} user=oj_test dbname={database} sslmode=disable")
            selection = {"report": ("OJ_SANDBOX_REPORT", "TestExecutionReport"),
                         "dual-worker": ("OJ_SANDBOX_DUAL", "TestDualWorkerRecovery"),
                         "worker-fault": ("OJ_SANDBOX_FAULT", "TestWorkerFaults")}
            command = [str(ROOT / ".local/bin/sandbox.test"), "-test.v", "-test.count=1", "-test.timeout=10m"]
            if suite in selection:
                flag, test = selection[suite]
                env[flag] = "1"
                command += ["-test.run=^" + test + "$"]
            subprocess.run(command, cwd=ROOT / "tests/sandbox", env=env, check=True)
    finally:
        for slot in owned:
            subprocess.run(DOCKER + ["stop", slot], check=False)
        if db_created:
            subprocess.run(DOCKER + ["rm", "-f", db], check=False)


if __name__ == "__main__":
    ROOT.joinpath(".local").mkdir(exist_ok=True)
    # Serialize all checkouts sharing the fixed Desktop slots.
    with open(Path("/tmp") / ("oj-desktop-tests-" + str(os.getuid()) + ".lock"), "a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        main()
