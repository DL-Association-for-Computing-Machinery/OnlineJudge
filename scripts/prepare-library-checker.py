#!/usr/bin/env python3
"""Prepare a private, unpublished corpus from the fixed Library Checker source.

Requires an existing checkout and local image with Python >=3.11 and GNU C++17.
Never downloads, installs, publishes, or runs upstream code on the host.
"""

import argparse
import json
import os
from pathlib import Path
import resource
import re
import shutil
import subprocess
import sys
import tempfile
import uuid

sys.dont_write_bytecode = True

from library_checker_quality import SLOW_SOLUTION_SCHEMA
from problem_source_common import MAX_EXPORT, digest, expected_materials, parse_lock, read_regular, snapshot
from problem_source_export import extract, validate_report


ROOT = Path(__file__).resolve().parent.parent
DOCKER = ["docker", "--context", "desktop-linux"]
OWNER = "onlinejudge.library-checker-source"


def docker(*args, **kwargs):
    kwargs.setdefault("timeout", 30)
    kwargs.setdefault("check", True)
    kwargs.setdefault("preexec_fn", diagnostic_limit)
    # Keep failure diagnostics bounded without exposing upstream program output.
    with tempfile.TemporaryFile() as errors:
        kwargs["stderr"] = errors
        try:
            return subprocess.run(DOCKER + list(args), **kwargs)
        except subprocess.CalledProcessError as error:
            errors.seek(0)
            detail = errors.read(8192).decode("utf-8", errors="replace")
            raise RuntimeError("Docker " + args[0] + " failed: " + detail) from error


def archive_limit():
    resource.setrlimit(resource.RLIMIT_FSIZE, (MAX_EXPORT + (64 << 20),) * 2)


def diagnostic_limit():
    resource.setrlimit(resource.RLIMIT_FSIZE, (1 << 20,) * 2)


def isolated(container):
    config, host = container["Config"], container["HostConfig"]
    if (config["User"] != "65532:65532" or host["Privileged"] or not host["ReadonlyRootfs"]
            or host["NetworkMode"] != "none" or host["Memory"] != 4 << 30
            or host["PidsLimit"] != 128 or host["NanoCpus"] != 2_000_000_000
            or host["CapDrop"] != ["ALL"] or "no-new-privileges:true" not in host["SecurityOpt"]):
        raise ValueError("container isolation differs from required profile")
    binds = {m["Destination"] for m in container["Mounts"] if m["Type"] == "bind" and not m["RW"]}
    if binds != {"/input", "/driver"} or any(m["RW"] for m in container["Mounts"]):
        raise ValueError("unexpected writable container mount")


def remove_owned(name):
    result = docker("inspect", name, stdout=subprocess.PIPE, check=False)
    if result.returncode != 0:
        raise RuntimeError("could not confirm container cleanup; inspect task container " + name)
    info = json.loads(result.stdout)[0]
    if info["Config"]["Labels"].get("owner") != OWNER or not re.fullmatch(r"[0-9a-f]{64}", info["Id"]):
        raise ValueError("refusing to remove an unowned container")
    docker("rm", "--force", info["Id"], stdout=subprocess.DEVNULL)
    remaining = docker("ps", "--all", "--quiet", "--no-trunc", "--filter", "id=" + info["Id"],
                       stdout=subprocess.PIPE).stdout.strip()
    if remaining:
        raise RuntimeError("task container still exists after cleanup")


def run(args):
    lock_path = ROOT / "config/problem-sources/library-checker.json"
    lock_bytes = read_regular(lock_path.parent, lock_path.name)
    lock = parse_lock(lock_bytes)
    source = args.source.resolve(strict=True)
    output = args.output.parent.resolve(strict=True) / args.output.name
    # Data and private programs never enter a tracked worktree by default.
    if output.is_relative_to(ROOT) or output.exists() or output.is_symlink():
        raise ValueError("output must be a new directory outside this checkout")
    raw = docker("image", "inspect", args.image, stdout=subprocess.PIPE).stdout
    image = json.loads(raw)[0]
    if image["Config"].get("Volumes"):
        raise ValueError("image-declared writable volumes are forbidden")
    image_id = image["Id"]
    # Reserve a private output directory atomically; failures keep only INCOMPLETE.
    output.mkdir(mode=0o700)
    incomplete = output / "INCOMPLETE"
    incomplete.write_text("Source preparation has not completed. Do not import or publish.\n")
    name = "oj-source-" + uuid.uuid4().hex[:12]
    removed = False
    with tempfile.TemporaryDirectory(prefix="oj-lc-") as temporary:
        staging = Path(temporary)
        inputs = staging / "input"
        (inputs / "source").mkdir(parents=True)
        snapshot(source, inputs / "source", lock)
        expected, catalog = expected_materials(inputs / "source", lock)
        (inputs / "source-lock.json").write_bytes(lock_bytes)
        drivers = staging / "driver"
        drivers.mkdir()
        for script in ("problem_source_common.py", "library_checker_quality.py", "validate_library_checker.py"):
            shutil.copyfile(ROOT / "scripts" / script, drivers / script)
        for directory in (inputs, drivers):
            directory.chmod(0o755)
            for path in directory.rglob("*"):
                path.chmod(0o755 if path.is_dir() else 0o444)
        try:
            created = docker("create", "--name", name, "--label", "owner=" + OWNER,
                   "--network", "none", "--read-only", "--cap-drop", "ALL",
                   "--security-opt", "no-new-privileges:true", "--user", "65532:65532",
                   "--memory", "4g", "--memory-swap", "4g", "--cpus", "2", "--pids-limit", "128",
                   "--ulimit", "nofile=256:256", "--ulimit", "core=0:0", "--no-healthcheck",
                   "--tmpfs", "/work:rw,exec,nosuid,nodev,size=3g,mode=1777",
                   "--tmpfs", "/tmp:rw,noexec,nosuid,nodev,size=128m,mode=1777",
                   "--mount", f"type=bind,src={inputs},dst=/input,readonly",
                   "--mount", f"type=bind,src={drivers},dst=/driver,readonly",
                   "--workdir", "/work", "--entrypoint", "/bin/sleep", image_id, "infinity",
                   stdout=subprocess.PIPE).stdout.decode().strip()
            if not re.fullmatch(r"[0-9a-f]{64}", created):
                raise ValueError("invalid created container identity")
            name = created
            inspection = json.loads(docker("inspect", name, stdout=subprocess.PIPE).stdout)[0]
            if inspection["Config"]["Labels"].get("owner") != OWNER:
                raise ValueError("container ownership mismatch")
            isolated(inspection)
            docker("start", name, stdout=subprocess.DEVNULL)
            # A fixed environment prevents image ENV (CXXFLAGS, LD_PRELOAD, etc.)
            # from choosing different compilers or injecting host-like settings.
            docker("exec", name, "/usr/bin/env", "-i", "PATH=/usr/local/bin:/usr/bin:/bin",
                   "HOME=/tmp", "TMPDIR=/tmp", "LANG=C.UTF-8", "PYTHONDONTWRITEBYTECODE=1",
                   "python3", "-B", "/driver/validate_library_checker.py", timeout=3600)
            archive = staging / "export.tar"
            with archive.open("wb") as stream:
                docker("exec", name, "/usr/bin/env", "-i", "PATH=/usr/bin:/bin", "tar",
                       "--hard-dereference", "--format=ustar", "-cf", "-", "-C", "/work/export", ".",
                       stdout=stream, timeout=180, preexec_fn=archive_limit)
            # The producer is gone before the host validates the private tar.
            # Any changing container bytes can only cause a transfer/hash failure.
            remove_owned(name)
            removed = True
            candidate = output / "materials.partial"
            candidate.mkdir(mode=0o700)
            report, size = extract(archive, candidate, expected)
            validate_report(report, lock, catalog, inventory=expected, required_schema=SLOW_SOLUTION_SCHEMA)
            document = {"schema": "oj.private-source-corpus.v1", "status": "private-unpublished",
                        "repository": lock["repository"], "commit": lock["commit"],
                        "source_lock_sha256": digest(lock_bytes),
                        "image_id": image_id, "source_license": "Apache-2.0; bundled notices retained",
                        "image_architecture": image["Architecture"],
                        "validation_driver_sha256": digest((drivers / "validate_library_checker.py").read_bytes()),
                        "source_reader_sha256": digest((drivers / "problem_source_common.py").read_bytes()),
                        "input_controls_driver_sha256": digest((drivers / "library_checker_quality.py").read_bytes()),
                        "compiler_flags": "-O2 -std=c++17 -Wall -Wextra -Werror -Wno-unused-result -I common",
                        "isolation": {"network": "none", "user": "65532:65532", "cpus": 2,
                                      "memory_bytes": 4 << 30, "pids": 128, "read_only_root": True,
                                      "work_tmpfs_bytes": 3 << 30, "child_wall_seconds": 120,
                                      "total_validation_wall_seconds": 3600},
                        "hashes_host_verified": True,
                        "execution_report_trust": "pinned-reviewed-source; not an adversarial-code attestation",
                        "notices": ["source/LICENSE", "source/common/testlib.h", "source/*/checker.cpp"],
                        "validation_scope": ("pinned hashes, all input validators, input boundary controls, "
                                             "reference and negative checker controls, three fixed wrong solutions, "
                                             "one fixed slow-solution CPU control"),
                        "publication_blockers": ["P4 import/authorization/review/publish not implemented",
                                                 "statement templates not converted or rendered",
                                                 "checker and resource limits not admitted to production"],
                        "bytes": size, "problems": catalog}
            (candidate / "catalog.json").write_text(json.dumps(document, ensure_ascii=False, indent=2) + "\n")
            candidate.rename(output / "materials")
            incomplete.unlink()
            print(json.dumps({"output": str(output / "materials"), "problems": len(catalog),
                              "cases": sum(len(p["cases"]) for p in catalog), "bytes": size}))
        finally:
            # Check even when create timed out: the daemon might have committed
            # creation before its acknowledgment was lost. Never remove by tag.
            if not removed:
                remove_owned(name)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", type=Path, required=True)
    parser.add_argument("--image", required=True, help="existing local image; no automatic pull/build")
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    os.umask(0o077)
    run(args)


if __name__ == "__main__":
    main()
