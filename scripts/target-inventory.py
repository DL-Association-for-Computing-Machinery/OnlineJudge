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


def volume(path):
    if not path.is_absolute():
        return {"state": "unavailable"}
    try:
        if not path.is_dir():
            return {"state": "unavailable"}
        fs = os.statvfs(path)
        if fs.f_frsize <= 0:
            return {"state": "unavailable"}
        return {"state": "observed", "available_bytes": fs.f_bavail * fs.f_frsize,
                "total_bytes": fs.f_blocks * fs.f_frsize, "available_inodes": fs.f_favail,
                "read_only": bool(fs.f_flag & os.ST_RDONLY)}
    except OSError:
        return {"state": "unavailable"}


def inspect(args):
    directory = cgroup_dir()
    try:
        affinity = len(os.sched_getaffinity(0))
    except OSError:
        affinity = None
    return {
        "schema": "oj.target-inventory.v1",
        "scope": "current Linux process and visible cgroup hierarchy; not target admission",
        "distribution": distribution(),
        "kernel": platform.release(),
        "machine": platform.machine(),
        "cpu_affinity_count": affinity,
        "proc_meminfo_bytes": memory(),
        "cgroup_v2": {"state": "observed" if directory else "unavailable",
                      "visible_leaf_to_root": constraints(directory) if directory else []},
        "volumes": {label: volume(getattr(args, label + "_dir"))
                    for label in ("materials", "scratch", "database")},
    }


def main():
    p = argparse.ArgumentParser(description=__doc__)
    for label in ("materials", "scratch", "database"):
        p.add_argument(f"--{label}-dir", type=Path, required=True)
    args = p.parse_args()
    if platform.system() != "Linux":
        p.error("requires the Linux machine or namespace being observed")
    print(json.dumps(inspect(args), indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    sys.exit(main())
