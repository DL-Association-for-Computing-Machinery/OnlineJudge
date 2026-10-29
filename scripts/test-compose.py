#!/usr/bin/env python3
"""Build and test an isolated Compose project, then remove only its containers."""

import json
import os
import platform
from pathlib import Path
import re
import subprocess
import time
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen
import uuid


ROOT = Path(__file__).resolve().parent.parent
ENV = {**os.environ, "OJ_WEB_PORT": "0"}
COMPOSE = ["docker"] + (["--context", "desktop-linux"] if platform.system() == "Darwin" else []) + ["compose", "-f", str(ROOT / "compose.yaml"),
           "-p", "oj-compose-test-" + uuid.uuid4().hex[:12]]


def compose(*args, capture=False):
    return subprocess.run(COMPOSE + list(args), cwd=ROOT, env=ENV, check=True,
                          text=True, stdout=subprocess.PIPE if capture else None).stdout


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def request(base, path, method="GET"):
    try:
        response = urlopen(Request(base + path, method=method), timeout=5)
    except HTTPError as error:
        response = error
    with response:
        return response.status, response.headers, response.read()


def check_health(base):
    status, headers, body = request(base, "/api/v1/healthz")
    require(status == 200, "API health failed through Nginx")
    require(headers.get("Cache-Control") == "no-store", "health cache policy lost")
    require(json.loads(body) == {"status": "ok", "service": "api", "scope": "process"},
            "health contract changed")


def web_base():
    address = compose("port", "web", "8080", capture=True).strip()
    require(re.fullmatch(r"127\.0\.0\.1:\d+", address), "web must bind loopback only")
    return "http://" + address


def main():
    try:
        compose("up", "-d", "--build", "--wait", "--wait-timeout", "60")
        base = web_base()
        status, headers, body = request(base, "/")
        require(status == 200 and "text/html" in headers.get("Content-Type", ""),
                "frontend HTML missing")
        assets = re.findall(rb'(?:src|href)="(/assets/[^" ]+)"', body)
        require(bool(assets), "built frontend assets missing")
        for asset in assets:
            code, _, content = request(base, asset.decode())
            require(code == 200 and bool(content), "frontend asset unavailable")
        require(request(base, "/training/example")[2] == body, "SPA fallback failed")
        require(request(base, "/assets/missing.js")[0] == 404, "missing asset returned HTML")
        check_health(base)
        for path, method, expected in [
            ("/api/v1/missing", "GET", 404),
            ("/api/v1/healthz", "POST", 405),
            ("/api/v1/submissions", "POST", 404),
        ]:
            code, response_headers, _ = request(base, path, method)
            require(code == expected and "application/problem+json" in
                    response_headers.get("Content-Type", ""), "API error routing failed")

        ids = compose("ps", "-q", capture=True).split()
        inspection = subprocess.run(["docker", "inspect", *ids], check=True,
                                    capture_output=True, text=True)
        for container in json.loads(inspection.stdout):
            require(container["Config"]["User"] not in ("", "root", "0", "0:0"),
                    "runtime container must be non-root")
            require(container["HostConfig"]["ReadonlyRootfs"], "root filesystem is writable")
            require(not container["HostConfig"]["Privileged"], "unexpected privileged container")
            if container["Config"]["Labels"]["com.docker.compose.service"] == "api":
                require(not container["HostConfig"]["PortBindings"], "API port published")

        compose("up", "-d", "--no-deps", "--force-recreate", "--wait", "api")
        deadline = time.monotonic() + 20
        while True:
            try:
                check_health(base)
                break
            except (RuntimeError, URLError):
                if time.monotonic() >= deadline:
                    raise
                time.sleep(1)
        compose("stop", "api")
        stopped_status, _, stopped_body = request(base, "/api/v1/healthz")
        # A stopped endpoint can refuse immediately (502) or time out (504).
        require(stopped_status in (502, 504),
                f"stopped API response: {stopped_status}, {stopped_body[:200]!r}")
        # Exercise the replacement path even on Compose versions that reuse web.
        compose("up", "-d", "--force-recreate", "--wait", "--wait-timeout", "60")
        # Port 0 is allocated again when web is replaced; the old URL is stale.
        check_health(web_base())
        print("Compose smoke passed: assets, SPA, API contracts, isolation, recreate and restart.")
    finally:
        # This unique project contains no volumes, databases or user data.
        compose("down", "--timeout", "15")


if __name__ == "__main__":
    main()
