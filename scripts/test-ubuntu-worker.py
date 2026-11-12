#!/usr/bin/env python3
"""Run the fixed HTTP/Worker fixture in the dedicated local Ubuntu arm64 VM."""
import argparse
import os
from pathlib import Path
import platform
import re
import stat
import subprocess
import tarfile
import time
import uuid

from offline_judge_bundle import ARCHIVE, IMAGES, POSTGRES, SANDBOX, verify


DOCKER = ("/usr/bin/docker", "--host=unix:///var/run/docker.sock")
ENV = {"HOME": "/", "PATH": "/usr/bin:/bin", "LANG": "C"}
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


def image_inventory():
    images = docker("image", "ls", "--all", "--no-trunc", "--format", "{{.ID}}").splitlines()
    if any(not re.fullmatch(r"sha256:[0-9a-f]{64}", image) for image in images):
        raise ValueError("invalid Docker image inventory")
    return set(images)


def load_bundle(layout, expected, require_empty=False):
    revision = verify(layout, expected)
    print(f"Verified offline judging bundle: {revision}", flush=True)
    if require_empty:
        if image_inventory():
            raise ValueError("cold reception requires an empty Docker image store")
        print("Before import: 0 images", flush=True)
    docker("image", "load", "--input", str(layout / ARCHIVE), timeout=180)
    if require_empty and image_inventory() != set(IMAGES):
        raise ValueError("cold import inventory differs from the sealed image set")
    for image in IMAGES:
        if docker("image", "inspect", "--format", "{{.Id}} {{.Os}}/{{.Architecture}}", image) != image + " linux/arm64":
            raise RuntimeError("wrong offline image")
    print("Loaded both pinned Linux arm64 images; fixture starts with --pull=never", flush=True)


def require_receiver_parents(layout):
    # A private child can still be swapped through a writable parent. The root
    # receiver may use /tmp: sticky directories protect root-owned entries.
    if ".." in layout.parts:
        raise ValueError("receiver path must not contain parent traversal")
    for parent in reversed(layout.parents):
        info = parent.lstat()
        if (not stat.S_ISDIR(info.st_mode) or info.st_uid != 0
                or (info.st_mode & 0o022 and not info.st_mode & stat.S_ISVTX)):
            raise ValueError("receiver ancestors must be root controlled")


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
    parser.add_argument("layout", type=Path, help="private receiver directory with a sealed offline bundle")
    parser.add_argument("--manifest-sha256", required=True, help="trusted sender manifest digest")
    parser.add_argument("--require-empty-image-store", action="store_true",
                        help="refuse any existing images and check the complete inventory after import")
    args = parser.parse_args()
    require_guest()
    layout = args.layout.absolute()
    try:
        require_receiver_parents(layout)
        load_bundle(layout, args.manifest_sha256, args.require_empty_image_store)
    except (OSError, ValueError, tarfile.TarError) as exc:
        parser.exit(1, f"offline bundle: {exc}\n")
    binaries = layout / ".local/bin"
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
