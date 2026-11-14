#!/usr/bin/env python3
"""Read Linux namespace, cgroup v2 and existing volume capacity; no writes."""

import argparse
import json
import os
from pathlib import Path, PurePosixPath
import platform
import re
import sys

CG = Path("/sys/fs/cgroup")
VOLUME_ROLES = ("materials", "scratch", "database")


def read(path):
    try:
        with path.open(encoding="ascii") as stream:
            data = stream.read((1 << 20) + 1)
        return data if len(data) <= 1 << 20 else None
    except (OSError, UnicodeError):
        return None


def positive(s):
    if not re.fullmatch(r"[1-9][0-9]{0,18}", s):
        raise ValueError("invalid positive integer")
    return int(s)


def cli_positive(value):
    try:
        return positive(value)
    except ValueError:
        raise argparse.ArgumentTypeError("expected a positive integer of at most 19 digits") from None


def distribution():
    data = read(Path("/etc/os-release")) or ""
    result = {}
    for key in ("ID", "VERSION_ID"):
        m = re.search(rf"(?m)^{key}=(.*)$", data)
        value = m[1].strip('"') if m else ""
        result[key] = value if re.fullmatch(r"[A-Za-z0-9_.-]{1,64}", value) else None
    return result


def memory():
    data = read(Path("/proc/meminfo")) or ""
    out = {}
    for key in ("MemTotal", "MemAvailable", "SwapTotal"):
        m = re.search(rf"(?m)^{key}:\s+([0-9]{{1,19}}) kB$", data)
        out[key] = int(m[1]) * 1024 if m else None
    return out


def cgroup_dir():
    memberships = (read(Path("/proc/self/cgroup")) or "").splitlines()
    matches = [line[3:] for line in memberships if line.startswith("0::")]
    mount = []
    for line in (read(Path("/proc/self/mountinfo")) or "").splitlines():
        pre, sep, post = line.partition(" - ")
        fields = pre.split()
        if sep and len(fields) >= 6 and fields[4] == str(CG) and post.split()[:1] == ["cgroup2"]:
            mount.append(fields[3])
    if len(matches) != 1 or len(mount) != 1:
        return None
    root, member = PurePosixPath(mount[0]), PurePosixPath(matches[0])
    if (not root.is_absolute() or not member.is_absolute() or ".." in root.parts or ".." in member.parts
            or "\\" in str(root) or "\\" in str(member)):
        return None
    try:
        directory = CG / member.relative_to(root)
        directory.relative_to(CG)
        if not directory.is_dir() or directory.resolve() != directory:
            return None
    except (OSError, ValueError):
        return None
    return directory


def constraints(directory):
    levels = []
    for path in (directory, *directory.parents):
        if not path.is_relative_to(CG):
            break
        row = {}
        for name in ("cpu.max", "memory.max", "pids.max"):
            raw = read(path / name)
            if raw is None:
                row[name] = {"state": "unavailable"}
                continue
            fields = raw.split()
            try:
                if name == "cpu.max" and len(fields) == 2:
                    period = positive(fields[1])
                    row[name] = {"state": "unbounded"} if fields[0] == "max" else {
                        "state": "bounded", "quota_usec": positive(fields[0]), "period_usec": period}
                elif name != "cpu.max" and len(fields) == 1:
                    if fields[0] == "max":
                        row[name] = {"state": "unbounded"}
                    elif fields[0] == "0":
                        row[name] = {"state": "bounded", "limit": 0}
                    else:
                        row[name] = {"state": "bounded", "limit": positive(fields[0])}
                else:
                    raise ValueError("bad cgroup value")
            except ValueError:
                row[name] = {"state": "invalid"}
        levels.append(row)
    return levels


def observe_volume(path):
    if not path.is_absolute():
        return {"state": "unavailable"}, None
    try:
        # Use one directory descriptor so a path change cannot mix a filesystem
        # observation with another directory's device ID.
        fd = os.open(path, os.O_RDONLY | os.O_DIRECTORY)
        try:
            fs = os.fstatvfs(fd)
            device = os.fstat(fd).st_dev
        finally:
            os.close(fd)
    except OSError:
        return {"state": "unavailable"}, None
    if fs.f_frsize <= 0 or not 0 <= fs.f_bavail <= fs.f_blocks or fs.f_blocks == 0:
        return {"state": "unavailable"}, None
    return ({"state": "observed", "available_bytes": fs.f_bavail * fs.f_frsize,
             "total_bytes": fs.f_blocks * fs.f_frsize,
             "available_inodes": fs.f_favail if 0 <= fs.f_favail <= fs.f_files and fs.f_files > 0 else None,
             "read_only": bool(fs.f_flag & os.ST_RDONLY)}, device)


def storage_budget(observations, peak_bytes, peak_inodes, reserve_bytes, reserve_inodes):
    scratch_device = observations["scratch"][1]
    rows = {}
    for role, (observation, device) in observations.items():
        shared = None if scratch_device is None or device is None else device == scratch_device
        if shared is None or observation["available_inodes"] is None:
            rows[role] = {"state": "unavailable", "shares_scratch_device": shared}
            continue
        after_bytes = observation["available_bytes"] - (peak_bytes if shared else 0)
        after_inodes = observation["available_inodes"] - (peak_inodes if shared else 0)
        failures = []
        if role in ("scratch", "database") and observation["read_only"]:
            failures.append("read_only_volume")
        if after_bytes < reserve_bytes:
            failures.append("byte_reserve")
        if after_inodes < reserve_inodes:
            failures.append("inode_reserve")
        rows[role] = {"state": "insufficient" if failures else "sufficient",
                      "shares_scratch_device": shared, "bytes_after_peak": after_bytes,
                      "inodes_after_peak": after_inodes, "failures": failures}
    return {"state": "sufficient" if all(row["state"] == "sufficient" for row in rows.values()) else "blocked",
            "scratch_peak_bytes": peak_bytes, "scratch_peak_inodes": peak_inodes,
            "reserve_bytes": reserve_bytes, "reserve_inodes": reserve_inodes, "volumes": rows}


def inspect(args):
    directory = cgroup_dir()
    try:
        affinity = len(os.sched_getaffinity(0))
    except OSError:
        affinity = None
    observations = {label: observe_volume(getattr(args, label + "_dir")) for label in VOLUME_ROLES}
    report = {
        "schema": "oj.target-inventory.v1",
        "scope": "current Linux process and visible cgroup hierarchy; not target admission",
        "distribution": distribution(),
        "kernel": platform.release(),
        "machine": platform.machine(),
        "cpu_affinity_count": affinity,
        "proc_meminfo_bytes": memory(),
        "cgroup_v2": {"state": "observed" if directory else "unavailable",
                      "visible_leaf_to_root": constraints(directory) if directory else []},
        "volumes": {label: observation for label, (observation, _) in observations.items()},
    }
    if args.storage_budget is not None:
        report["storage_budget"] = storage_budget(observations, *args.storage_budget)
    return report


def main():
    p = argparse.ArgumentParser(description=__doc__)
    for label in VOLUME_ROLES:
        p.add_argument(f"--{label}-dir", type=Path, required=True)
    p.add_argument("--storage-budget", nargs=4, type=cli_positive,
                   metavar=("SCRATCH_PEAK_BYTES", "SCRATCH_PEAK_INODES", "RESERVE_BYTES", "RESERVE_INODES"),
                   help="check explicit scratch peak and post-peak reserve per volume; exit 2 if blocked")
    args = p.parse_args()
    if platform.system() != "Linux":
        p.error("requires the Linux machine or namespace being observed")
    report = inspect(args)
    print(json.dumps(report, indent=2, sort_keys=True))
    return 2 if report.get("storage_budget", {}).get("state") == "blocked" else 0


if __name__ == "__main__":
    sys.exit(main())
