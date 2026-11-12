#!/usr/bin/env python3
"""Run the fixed HTTP/Worker fixture in the dedicated local Ubuntu arm64 VM."""
import argparse
import os
from pathlib import Path
import platform
import re
import subprocess
import time
import uuid


DOCKER = ("/usr/bin/docker", "--host=unix:///var/run/docker.sock")
ENV = {"HOME": "/", "PATH": "/usr/bin:/bin", "LANG": "C"}
SANDBOX = "sha256:cd40ffdb32619806a8d97e59fe03e890fe4066835f57efa6ae13f919ef3262c5"
POSTGRES = "sha256:4ea9e5ed06591da7ea23eb65465e8d3187fe79f4d5ec3ae976d29a33b013e77a"
SLOT = "oj-m0-sandbox"
LABEL = "onlinejudge.ubuntu-test"
ID = re.compile(r"[0-9a-f]{64}")


def docker(*args, timeout=15, quiet=False):
    return subprocess.run((*DOCKER, *args), env=ENV, check=True, text=True,
                          stdout=subprocess.PIPE, stderr=subprocess.DEVNULL if quiet else None,
                          timeout=timeout).stdout.strip()


def wait_for(probe, expected):
    deadline = time.monotonic() + 15
    while time.monotonic() < deadline:
        try:
            if probe() == expected:
                return
        except (subprocess.CalledProcessError, subprocess.TimeoutExpired):
            pass
        time.sleep(.1)
    raise RuntimeError("fixture service did not become ready")


def require_guest():
    if (os.environ.get("OJ_SANDBOX_EXPERIMENT") != "1" or os.geteuid() != 0
            or platform.system() != "Linux" or platform.machine() != "aarch64"
            or platform.release() != "6.8.0-106-generic"):
        raise RuntimeError("requires explicit opt-in and the dedicated Ubuntu arm64 VM owner")
    detected = subprocess.check_output(("/usr/bin/systemd-detect-virt", "--vm"), env=ENV, text=True, timeout=3).strip()
    mounts = Path("/proc/self/mountinfo").read_text().splitlines()
    filesystems = {line.split(" - ", 1)[1].split()[0] for line in mounts}
    if detected != "apple" or not filesystems or filesystems & {"virtiofs", "9p", "fuse.sshfs", "sshfs"}:
        raise RuntimeError("requires the fixed local guest without host shares")
    if docker("info", "--format", "{{.OSType}}/{{.Architecture}}/{{.CgroupVersion}}") != "linux/aarch64/2":
        raise RuntimeError("wrong local Docker Engine")
    for image in (SANDBOX, POSTGRES):
        if docker("image", "inspect", "--format", "{{.Id}} {{.Os}}/{{.Architecture}}", image) != image + " linux/arm64":
            raise RuntimeError("wrong offline image")


def clean_owned(ident, owner, sandbox, passed):
    form = '{{.Id}} {{index .Config.Labels "' + LABEL + '"}} {{.State.Running}} {{.State.Pid}}'
    state = docker("inspect", "--format", form, ident).split()
    if len(state) != 4 or state[:2] != [ident, owner]:
        raise RuntimeError("refusing to clean up an unowned fixture container")
    if sandbox:
        if state[2] == "true":
            docker("kill", "--signal", "KILL", ident)
            state = docker("inspect", "--format", form, ident).split()
        if state != [ident, owner, "false", "0"]:
            raise RuntimeError("fixture sandbox stop is not confirmed")
        if passed:
            docker("rm", ident)
        else:
            print(f"Retained stopped fixture {ident} for owner recovery.", flush=True)
    else:
        docker("rm", "-f", ident)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("layout", type=Path, help="temporary root with .local/bin and tests/sandbox")
    args = parser.parse_args()
    require_guest()
    layout = args.layout.resolve(strict=True)
    binaries = layout / ".local/bin"
    for name in ("api", "worker", "worker-api", "migrate", "sandbox.test"):
        path = binaries / name
        if path.is_symlink() or not path.is_file() or not os.access(path, os.X_OK):
            raise RuntimeError("expected prebuilt Linux arm64 test artifacts")
    owner = uuid.uuid4().hex
    db = slot = None
    passed = False
    try:
        slot = docker("create", "--pull=never", "--name", SLOT, "--label", f"{LABEL}={owner}",
                      "--privileged", "--network", "none", "--read-only", "--ipc", "private",
                      "--cgroupns", "private", "--restart", "no", "--tmpfs", "/tmp:rw,nosuid,size=128m",
                      "--shm-size", "128m", "--memory", "1g", "--memory-swap", "1g",
                      "--cpus", "1", "--pids-limit", "128", SANDBOX)
        db = docker("run", "-d", "--pull=never", "--name", "oj-ubuntu-db-" + owner,
                    "--label", f"{LABEL}={owner}", "--memory", "512m", "--memory-swap", "512m", "--cpus", "1",
                    "-p", "127.0.0.1::5432", "-e", "POSTGRES_USER=oj_test",
                    "-e", "POSTGRES_HOST_AUTH_METHOD=trust", "--tmpfs", "/var/lib/postgresql/data", POSTGRES)
        if not ID.fullmatch(slot) or not ID.fullmatch(db):
            raise RuntimeError("missing owned container identity")
        wait_for(lambda: docker("exec", db, "psql", "-h", "127.0.0.1", "-U", "oj_test", "-Atqc", "SELECT 1",
                                timeout=2, quiet=True), "1")
        port = docker("port", db, "5432")
        if not re.fullmatch(r"127\.0\.0\.1:[1-9][0-9]{0,4}", port):
            raise RuntimeError("test database is not published on the guest loopback only")
        docker("start", slot)
        wait_for(lambda: docker("exec", slot, "curl", "--disable", "-fsS", "--max-time", "1", "-o", "/dev/null",
                                "-w", "%{http_code}", "http://127.0.0.1:5050/version", timeout=2, quiet=True), "200")
        env = dict(ENV, OJ_SANDBOX_EXPERIMENT="1", OJ_SANDBOX_UBUNTU_HTTP="1",
                   OJ_TEST_DATABASE_URL=f"host=127.0.0.1 port={port.split(':')[1]} user=oj_test dbname=oj_test sslmode=disable")
        subprocess.run((binaries / "sandbox.test", "-test.run=^TestUbuntuHTTPJudgeLoop$",
                        "-test.v", "-test.count=1", "-test.timeout=105s"),
                       cwd=layout / "tests/sandbox", env=env, check=True, timeout=120)
        passed = True
    finally:
        failures = []
        for ident, sandbox in ((slot, True), (db, False)):
            if ident is not None and ID.fullmatch(ident):
                try:
                    clean_owned(ident, owner, sandbox, passed)
                except (subprocess.SubprocessError, RuntimeError) as err:
                    failures.append(err)
        if failures:
            raise RuntimeError("fixture cleanup unconfirmed; inspect only this run's labeled containers") from failures[0]


if __name__ == "__main__":
    main()
