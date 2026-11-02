#!/usr/bin/env python3
"""Start the persistent, isolated local HTTPS account environment with Docker."""

import argparse
import os
from pathlib import Path
import platform
import subprocess
import ssl
import sys
import tempfile
import urllib.request


ROOT = Path(__file__).resolve().parent.parent
PROJECT = "oj-local-dev"
URL = "https://localhost:8443"


def run(command, *, capture=False):
    """Keep failures readable without dumping runtime state or credentials."""
    try:
        result = subprocess.run(command, cwd=ROOT, check=False, text=True,
                                stdout=subprocess.PIPE if capture else None,
                                stderr=subprocess.PIPE if capture else None)
    except OSError:
        raise RuntimeError("无法执行 Docker；请确认已安装 Docker 并启动 Linux Engine。") from None
    if result.returncode:
        raise RuntimeError("Docker 操作失败；请检查 Docker Engine、Compose、构建结果及 8443 端口。"
                           "保留了已有容器和数据，未自动重置。")
    return result.stdout.strip() if capture else ""


def start(context):
    docker = ["docker"] + (["--context", context] if context else [])
    compose = docker + ["compose", "-p", PROJECT, "-f", str(ROOT / "compose.dev.yaml")]
    if run(docker + ["info", "--format", "{{.OSType}}"], capture=True) != "linux":
        raise RuntimeError("本地开发环境需要 Linux Docker Engine；请检查所选 context。")
    run(compose + ["version", "--short"], capture=True)
    run(compose + ["up", "-d", "--build", "--wait", "--wait-timeout", "120"])
    # Do not read or print account.json: only test that initialization completed.
    run(compose + ["exec", "-T", "localdev", "sh", "-ec",
                   "test -s /state/ready.json && test -s /state/ca.crt "
                   "&& test -s /state/server.crt && test -s /state/server.key "
                   "&& test -s /state/login-keys.json && test -s /state/account.json"],
        capture=True)
    container_id = run(compose + ["ps", "-q", "localdev"], capture=True)
    if not container_id or len(container_id.splitlines()) != 1:
        raise RuntimeError("本地 HTTPS 服务容器未就绪；已保留数据，请检查 Compose 状态。")
    if run(compose + ["port", "localdev", "8443"], capture=True) != "127.0.0.1:8443":
        raise RuntimeError("HTTPS 未正确绑定本机回环端口；已保留数据。")

    destination = ROOT / ".local" / "dev-auth"
    destination.mkdir(parents=True, exist_ok=True)
    # Copy only the public CA, then replace atomically so an interrupted copy
    # cannot leave a partial trust file. Private material stays in the volume.
    with tempfile.TemporaryDirectory(prefix="oj-local-ca-", dir=destination) as staging:
        certificate = Path(staging) / "ca.crt"
        run(docker + ["cp", container_id + ":/state/ca.crt", str(certificate)], capture=True)
        if not certificate.is_file() or certificate.stat().st_size == 0:
            raise RuntimeError("本地 CA 证书导出失败；未导出任何私钥或账号。")
        certificate.chmod(0o644)
        os.replace(certificate, destination / "ca.crt")

    try:
        context = ssl.create_default_context(cafile=str(destination / "ca.crt"))
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPSHandler(context=context))
        with opener.open(URL + "/__local/healthz", timeout=5) as response:
            if response.status != 204:
                raise RuntimeError("HTTPS readiness failed")
    except Exception:
        raise RuntimeError("本机 HTTPS 访问或证书校验失败；已保留服务与数据，请检查 8443 入口。") from None

    print("本地 HTTPS 与账号服务已就绪：" + URL + "/login")
    print("首次访问需在浏览器确认本地证书，或自行信任公开 CA：" + str(destination / "ca.crt"))
    print("账号秘密仅保存在 Docker 私有 volume；页面提供本地一键测试登录。")
    print("再次运行本命令会保留账号、密钥、数据库和 Redis 数据；不会自动重置。")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--context", default="desktop-linux" if platform.system() == "Darwin" else None,
                        help="Docker context；macOS 默认 desktop-linux，Linux 默认当前 context")
    args = parser.parse_args()
    try:
        start(args.context)
    except (RuntimeError, OSError) as error:
        # Fixed messages avoid accidentally exposing Docker/container output.
        message = str(error) if isinstance(error, RuntimeError) else "本地 CA 导出目录不可用；请检查 .local/dev-auth 权限。"
        print("启动失败：" + message, file=sys.stderr)
        return 1
    except KeyboardInterrupt:
        print("启动已中断；已保留容器与持久数据，可再次运行同一命令。", file=sys.stderr)
        return 130
    return 0


if __name__ == "__main__":
    sys.exit(main())
