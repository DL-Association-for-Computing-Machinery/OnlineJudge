#!/usr/bin/env python3
"""Read-only development preflight; no fetch, installation or container startup."""

import argparse
import errno
import json
import os
from pathlib import Path
import platform
import re
import shutil
import socket
import subprocess
import sys


ROOT = Path(__file__).resolve().parent.parent
INFO_FORMAT = '{"OSType":{{json .OSType}},"NCPU":{{.NCPU}},"MemTotal":{{.MemTotal}}}'


def run(command, root):
    try:
        return subprocess.run(command, cwd=root, capture_output=True, text=True, timeout=10,
                              env={**os.environ, "GIT_OPTIONAL_LOCKS": "0"})
    except (OSError, subprocess.TimeoutExpired):
        return None


def inspect(root=ROOT, system=None, port=8080):
    system = system or platform.system()
    checks = []

    def add(name, status, message):
        checks.append({"name": name, "status": status, "message": message})

    for tool in ("git", "python3", "docker", "gh"):
        add("tool:" + tool, "PASS" if shutil.which(tool) else "FAIL",
            "可用" if shutil.which(tool) else "缺少命令；请按项目环境说明安装")

    def git(*args):
        return run(["git", *args], root)

    branch = git("symbolic-ref", "--quiet", "--short", "HEAD")
    name = branch.stdout.strip() if branch and branch.returncode == 0 else ""
    add("branch", "FAIL" if name in ("", "main", "master") else "PASS",
        "需要任务分支，当前为主线或游离 HEAD" if name in ("", "main", "master") else "当前为任务分支")
    base = git("rev-parse", "--verify", "refs/remotes/origin/main")
    if base and base.returncode == 0:
        ancestor = git("merge-base", "--is-ancestor", "origin/main", "HEAD")
        if ancestor and ancestor.returncode in (0, 1):
            add("base", "PASS" if ancestor.returncode == 0 else "WARN",
                "包含缓存 origin/main；未联网验证新鲜度或创建时的 fetch" if ancestor.returncode == 0
                else "未包含缓存 origin/main；核对任务归属并更新基线，不自动重置")
        else:
            add("base", "FAIL", "无法验证主线祖先关系")
    else:
        add("base", "FAIL", "缺少 origin/main；新建分支前需成功 git fetch origin main")
    status = git("status", "--porcelain=v1", "-z")
    if status and status.returncode == 0:
        add("worktree", "WARN" if status.stdout else "PASS",
            "存在未提交/未跟踪改动；请确认归属，未输出文件内容" if status.stdout else "工作区干净")
    else:
        add("worktree", "FAIL", "无法读取工作区状态")
    hooks = git("config", "--get", "core.hooksPath")
    value = hooks.stdout.strip() if hooks and hooks.returncode == 0 else ""
    hooks_path = (root / value).resolve() if value else None
    expected = root / ".githooks"
    active = hooks_path == expected.resolve() and all(
        (expected / hook).is_file() and os.access(expected / hook, os.X_OK)
        for hook in ("pre-commit", "commit-msg", "pre-push")
    )
    add("hooks", "PASS" if active else "FAIL", "仓库三个 hooks 已启用且可执行" if active
        else "hooks 未指向本工作区或不可执行；检查 scripts/install-hooks.sh，保留既有配置")

    docker = ["docker"] + (["--context", "desktop-linux"] if system == "Darwin" else [])
    add("docker-context", "INFO", "显式使用 desktop-linux" if system == "Darwin" else "使用当前 Docker context")
    info = run(docker + ["info", "--format", INFO_FORMAT], root)
    try:
        data = json.loads(info.stdout) if info and info.returncode == 0 else {}
        if not isinstance(data, dict) or data.get("OSType") != "linux":
            raise ValueError("Linux engine unavailable")
        cpus, memory = int(data["NCPU"]), int(data["MemTotal"])
        if cpus <= 0 or memory <= 0:
            raise ValueError("Invalid resources")
        add("docker-engine", "PASS", "Linux Engine 可访问")
        add("docker-resources", "INFO", f"{cpus} CPU，{memory / 2**30:.1f} GiB 内存；不是容量验收")
    except (ValueError, TypeError, KeyError):
        add("docker-engine", "FAIL", "无法取得 Linux Engine 信息；检查 Docker Desktop/context/权限")
    compose = run(docker + ["compose", "version", "--short"], root)
    version = compose.stdout.strip() if compose and compose.returncode == 0 else ""
    match = re.fullmatch(r"v?(\d+)\.\d+\.\d+(?:[-+][\w.-]+)?", version)
    add("compose", "PASS" if match and int(match[1]) >= 2 else "FAIL",
        "Compose v2+ 可用" if match and int(match[1]) >= 2 else "需要可用的 Compose v2+")

    try:
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as probe:
            probe.bind(("127.0.0.1", port))
        add("port", "PASS", f"127.0.0.1:{port} 可绑定（瞬时探测，不启动监听）")
    except OSError as error:
        message = ("当前权限不允许端口探测；端口可用性未知" if error.errno in (errno.EACCES, errno.EPERM)
                   else f"127.0.0.1:{port} 不可绑定；可能已有应用，启动前核对或更换 OJ_WEB_PORT")
        add("port", "WARN", message)
    try:
        add("disk", "INFO", f"工作区所在磁盘剩余 {shutil.disk_usage(root).free / 2**30:.1f} GiB；不代表 Docker VM 磁盘空间")
    except OSError:
        add("disk", "WARN", "无法读取工作区剩余磁盘空间")
    return checks


def exit_code(checks, strict=False):
    return int(any(item["status"] == "FAIL" or (strict and item["status"] == "WARN") for item in checks))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--json", action="store_true", help="输出结构化结果")
    parser.add_argument("--strict", action="store_true", help="警告也返回失败")
    parser.add_argument("--port", type=int, default=os.environ.get("OJ_WEB_PORT", "8080"))
    args = parser.parse_args()
    if not 1 <= args.port <= 65535:
        parser.error("port must be between 1 and 65535")
    checks = inspect(port=args.port)
    if args.json:
        print(json.dumps({"checks": checks, "exit_code": exit_code(checks, args.strict)}, ensure_ascii=False, indent=2))
    else:
        for item in checks:
            print(f"[{item['status']}] {item['name']}: {item['message']}")
    return exit_code(checks, args.strict)


if __name__ == "__main__":
    sys.exit(main())
