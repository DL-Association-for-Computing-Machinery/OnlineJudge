#!/usr/bin/env python3
"""Probe fixed Testlib fixtures through an owned offline go-judge container."""

import argparse
import importlib.util
import json
import os
from pathlib import Path
import platform
import re
import subprocess

ROOT = Path(__file__).resolve().parent.parent
FIXTURES = ROOT / "tests/sandbox/testdata"
IMAGE = "sha256:e6b23d40935394adff7db3d21ac237a5921cc3b7e403acccc88158f759dfc2a3"
SPEC = importlib.util.spec_from_file_location("prepare", Path(__file__).with_name("prepare-testlib-spj.py"))
PREPARE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(PREPARE)


def docker(*args, body=None):
    return subprocess.run(
        ["/usr/bin/docker", "--host=unix:///var/run/docker.sock", *args],
        input=body, stdout=subprocess.PIPE, check=True, timeout=50,
    ).stdout


def request(slot, command):
    raw = docker(
        "exec", "-i", slot, "curl", "-fsS", "--max-time", "45", "-H",
        "Content-Type: application/json", "--data-binary", "@-", "http://127.0.0.1:5050/run",
        body=json.dumps({"cmd": [command]}).encode(),
    )
    [result] = json.loads(raw)
    return result


def command(args, cpu, clock, memory, processes):
    return {
        "args": args, "env": ["PATH=/usr/local/bin:/usr/bin:/bin", "LANG=C", "LD_LIBRARY_PATH=/usr/local/lib64"],
        "files": [{"content": ""}, {"name": "stdout", "max": 4096}, {"name": "stderr", "max": 4096}],
        "cpuLimit": cpu, "clockLimit": clock, "memoryLimit": memory, "procLimit": processes,
    }


def compile_fixture(slot, name, header):
    build = command(
        ["/usr/local/bin/g++", "-std=gnu++17", "-O2", "-pipe", *PREPARE.FLAGS, "checker.cpp", "-o", "checker"],
        20_000_000_000, 40_000_000_000, 512 << 20, 16,
    )
    build.update(copyIn={"checker.cpp": {"content": (FIXTURES / name).read_text()},
                         "testlib.h": {"content": header}}, copyOutCached=["checker"], copyOutMax=1 << 20)
    result = request(slot, build)
    if result.get("status") != "Accepted" or result.get("exitStatus") != 0 or result.get("fileError") or result.get("error"):
        raise RuntimeError(f"{name}: checker build failed ({result.get('status')})")
    ids = result["fileIds"]
    if set(ids) != {"checker"} or not re.fullmatch(r"[A-Za-z0-9_-]{1,128}", ids["checker"]):
        raise RuntimeError("unexpected checker artifact")
    return ids["checker"]


def verify_slot(slot):
    if not re.fullmatch(r"[a-f0-9]{64}", slot):
        raise ValueError("full owned container ID required")
    if (os.geteuid() != 0 or platform.release() != "6.8.0-106-generic" or
            subprocess.check_output(["systemd-detect-virt", "--vm"], text=True).strip() != "apple"):
        raise RuntimeError("run the checker probe only on the dedicated Ubuntu VM")
    mounts = subprocess.run(
        ["findmnt", "-rn", "-t", "virtiofs,9p,fuse.sshfs,sshfs", "-o", "TARGET"], capture_output=True,
    )
    if mounts.returncode != 1 or mounts.stdout or mounts.stderr:
        raise RuntimeError("checker probe requires a guest without shared host files")
    [state] = json.loads(docker("inspect", slot))
    config = state["HostConfig"]
    if (state["Image"] != IMAGE or not state["State"]["Running"] or state["Mounts"] or
            state["Config"]["Labels"].get("owner") != "onlinejudge.p7-testlib-probe" or
            config["NetworkMode"] != "none" or not config["ReadonlyRootfs"] or
            config["IpcMode"] != "private" or config["CgroupnsMode"] != "private" or
            config["Memory"] != 1 << 30 or config["MemorySwap"] != 1 << 30 or
            config["NanoCpus"] != 1_000_000_000 or config["PidsLimit"] != 128):
        raise RuntimeError("wrong checker probe image or isolation")
    version = json.loads(docker("exec", slot, "curl", "-fsS", "http://127.0.0.1:5050/version"))
    if (version.get("buildVersion"), version.get("os"), version.get("platform")) != ("v1.13.3", "linux", "arm64"):
        raise RuntimeError("unexpected checker probe executor")
    runner = json.loads(docker("exec", slot, "curl", "-fsS", "http://127.0.0.1:5050/config"))["runnerConfig"]
    if runner["cgroupType"] != 2 or not runner["seccomp"] or not {"cpu", "memory", "pids"} <= set(runner["cgroupControllers"]):
        raise RuntimeError("checker probe requires verified cgroup and seccomp")


def run_cases(slot, id_, cases):
    try:
        for name, test, output, answer, code in cases:
            checked_exit(slot, id_, name, test, output, answer, code)
    finally:
        remove(slot, id_)


def remove(slot, id_):
    docker("exec", slot, "curl", "-fsS", "--max-time", "3", "-X", "DELETE", f"http://127.0.0.1:5050/file/{id_}")


def checked_exit(slot, id_, name, test, output, answer, code):
    run = command(["./checker", "input", "output", "answer"], 1_000_000_000, 4_000_000_000, 128 << 20, 8)
    run["copyIn"] = {"checker": {"fileId": id_}, "input": {"content": test},
                     "output": {"content": output}, "answer": {"content": answer}}
    r = request(slot, run)
    if (r.get("status") != "Nonzero Exit Status" or r.get("exitStatus") != code or
            r.get("fileError") or r.get("error") or r.get("fileIds") or
            not 0 <= r.get("time", -1) <= run["cpuLimit"] or
            not 0 <= r.get("memory", -1) <= run["memoryLimit"]):
        raise RuntimeError(f"{name}: expected normal exit {code}; got {r.get('status')} / {r.get('exitStatus')}")
    print(f"PASS {name}: {code}")
    return r["exitStatus"]


def span_report(slot, header, bundle, directory, plan, runtime):
    import hashlib

    def sha(data):
        return hashlib.sha256(data).hexdigest()

    def read_ref(ref):
        data = (bundle / "objects/sha256" / ref["sha256"]).read_bytes()
        if len(data) != ref["size"] or sha(data) != ref["sha256"]:
            raise ValueError("fixture material does not match its digest")
        return data

    draft = json.loads((bundle / "drafts/span.json").read_text())
    src = (FIXTURES / "testlib_span_checker.cpp").read_bytes()
    if read_ref(draft["provenance"]["original_files"]["source/local/bounded_span/quality/checker.cpp"]) != src:
        raise ValueError("fixture checker source differs")
    if not 1 <= len(draft["test_cases"]) <= 16:
        raise ValueError("unsupported span test count")
    id_ = compile_fixture(slot, "testlib_span_checker.cpp", header)
    try:
        artifact = docker("exec", slot, "curl", "-fsS", "--max-time", "3", "--max-filesize", str(1 << 20),
                          f"http://127.0.0.1:5050/file/{id_}")
        if not artifact.startswith(b"\x7fELF") or len(artifact) > 1 << 20:
            raise ValueError("unexpected checker executable")
        points = []
        for point in draft["test_cases"]:
            test, answer = (read_ref(point[field]).decode() for field in ("input", "answer"))
            point_id = point["id"]
            points.append({
                "input_sha256": sha(test.encode()), "answer_sha256": sha(answer.encode()),
                "canonical_exit": checked_exit(slot, id_, point_id + " canonical", test, answer, answer, 50),
                "alternate_exit": checked_exit(slot, id_, point_id + " alternate", test, f"1 {int(answer)-1}", answer, 50),
                "invalid_exit": checked_exit(slot, id_, point_id + " invalid", test, str(int(answer)+1), answer, 51),
            })
    finally:
        remove(slot, id_)
    directory.mkdir(mode=0o700)
    report = {"schema": "testlib-0.9.45-oj-binary-v1", "source_sha256": sha(src), "artifact_sha256": sha(artifact),
              "image": IMAGE, "adapted_header_sha256": sha(header.encode()), "plan_sha256": plan,
              "runtime_sha256": runtime, "points": points}
    (directory / "checker").write_bytes(artifact)
    (directory / "report.json").write_text(json.dumps(report, separators=(",", ":")))
    print("SPJ report sha256:", sha((directory / "report.json").read_bytes()))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("slot", help="ID of an already running owned probe container in the dedicated VM")
    parser.add_argument("source", type=Path, help="original Testlib header")
    parser.add_argument("sha256")
    parser.add_argument("--span-bundle", type=Path)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--plan-sha256")
    parser.add_argument("--runtime-sha256")
    args = parser.parse_args()
    verify_slot(args.slot)
    with args.source.open("rb") as source:
        header = PREPARE.prepare(source.read(PREPARE.UPSTREAM_BYTES + 1), args.sha256).decode("utf-8")
    if args.span_bundle or args.output or args.plan_sha256 or args.runtime_sha256:
        if (not args.span_bundle or not args.output or not re.fullmatch(r"[a-f0-9]{64}", args.plan_sha256 or "") or
                not re.fullmatch(r"[a-f0-9]{64}", args.runtime_sha256 or "")):
            parser.error("span bundle, output directory and two trusted digests are required together")
        span_report(args.slot, header, args.span_bundle, args.output, args.plan_sha256, args.runtime_sha256)
        return
    run_cases(args.slot, compile_fixture(args.slot, "testlib_spj_checker.cpp", header), [
        ("valid order", "8", "3 5", "8", 50),
        ("alternate order", "8", "5 3", "8", 50),
        ("wrong sum", "8", "3 4", "8", 51),
        ("malformed output", "8", "x 5", "8", 52),
        ("extra output", "8", "3 5 extra", "8", 52),
        ("contestant EOF", "8", "3", "8", 54),
        ("input EOF", "", "3 5", "8", 55),
        ("invalid answer", "8", "3 5", "x", 55),
    ])
    run_cases(args.slot, compile_fixture(args.slot, "testlib_spj_scoring_checker.cpp", header), [
        ("points", "1", "", "", 55),
        ("points with extra output", "1", "extra", "", 55),
        ("partial", "2", "", "", 55),
        ("partial with extra output", "2", "extra", "", 55),
    ])


if __name__ == "__main__":
    main()
