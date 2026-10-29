"""Exercise built migration/API commands against a disposable local database.

Only synthetic source and a temporary random token are used. Never prints secrets.
"""

import concurrent.futures
import hashlib
import json
import os
from pathlib import Path
import secrets
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request


def free_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def main():
    root = Path(__file__).resolve().parents[2]
    dsn = os.environ["OJ_TEST_DATABASE_URL"]
    # The shell creates this empty database in its private temporary cluster.
    env = dict(os.environ, OJ_DATABASE_URL=dsn)
    for command in ("up", "up", "fixture", "fixture"):
        subprocess.run([str(root / ".local/bin/migrate"), command], env=env, check=True)

    with tempfile.TemporaryDirectory(prefix="oj-api-processes-") as temp:
        material_root = Path(temp) / "materials"
        material_root.mkdir(mode=0o700)
        fixture_env = dict(env, OJ_MATERIAL_ROOT=str(material_root))
        for _ in range(2):
            subprocess.run([str(root / ".local/bin/migrate"), "fixture"], env=fixture_env, check=True)
        for data in (b"1 2\n", b"3\n", b"-7 4\n", b"-3\n"):
            assert (material_root / hashlib.sha256(data).hexdigest()).read_bytes() == data
        assert len(list(material_root.iterdir())) == 4

        pinned_env = dict(fixture_env, OJ_EXPERIMENT_MATERIAL_VERSION="m0-sum-v3")
        for _ in range(2):
            subprocess.run([str(root / ".local/bin/migrate"), "fixture"], env=pinned_env, check=True)
        invalid_env = dict(fixture_env, OJ_EXPERIMENT_MATERIAL_VERSION="unknown")
        assert subprocess.run([str(root / ".local/bin/migrate"), "fixture"], env=invalid_env,
                              stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode != 0
        token = secrets.token_urlsafe(32)
        token_path = Path(temp) / "token"
        token_path.write_text(token)
        token_path.chmod(0o600)
        env["OJ_EXPERIMENT_TOKEN_FILE"] = str(token_path)
        processes = []
        logs = []
        ports = [free_port(), free_port()]

        def request(index, path, data=None, key="process-test-key-0001"):
            req = urllib.request.Request(
                f"http://127.0.0.1:{ports[index]}/api/v1/experiments{path}",
                data=data,
                headers={"Authorization": f"Bearer {token}",
                         "Content-Type": "application/json", "Idempotency-Key": key},
            )
            with urllib.request.urlopen(req, timeout=10) as response:
                assert response.headers["Cache-Control"] == "no-store"
                return response.status, json.load(response)

        def start(index, version=""):
            log = (Path(temp) / f"api-{index}-{len(logs)}.log").open("wb")
            logs.append(log)
            process = subprocess.Popen(
                [str(root / ".local/bin/api")],
                env=dict(env, OJ_API_ADDR=f"127.0.0.1:{ports[index]}",
                         OJ_EXPERIMENT_MATERIAL_VERSION=version),
                stdout=log, stderr=log,
            )
            processes.append(process)
            for _ in range(100):
                if process.poll() is not None:
                    raise RuntimeError("API exited during startup")
                try:
                    with urllib.request.urlopen(f"http://127.0.0.1:{ports[index]}/api/v1/healthz", timeout=.2):
                        return process
                except (urllib.error.URLError, TimeoutError):
                    time.sleep(.05)
            raise RuntimeError("API startup timeout")

        try:
            start(0)
            second = start(1)
            data = json.dumps({"material_version": "m0-sum-v1", "language": "gnu++17",
                               "source": "// synthetic process test; never executed\n"}).encode()
            with concurrent.futures.ThreadPoolExecutor(max_workers=12) as executor:
                results = list(executor.map(lambda i: request(i % 2, "/submissions", data), range(24)))
            assert sum(status == 202 for status, _ in results) == 1
            ids = {body["id"] for _, body in results}
            assert len(ids) == 1
            assert all(body["state"] == "queued" and body["verdict"] is None for _, body in results)
            second.terminate()
            assert second.wait(timeout=10) == 0
            restarted = start(1)
            status, receipt = request(1, "/submission-receipt")
            assert status == 200 and receipt["id"] in ids
            status, receipt = request(0, "/submissions/" + receipt["id"])
            assert status == 200 and "source" not in receipt
            restarted.terminate()
            assert restarted.wait(timeout=10) == 0
            start(1, "m0-sum-v3")
            pinned_data = json.dumps({"material_version": "m0-sum-v3", "language": "gnu++17",
                                      "source": "// synthetic pinned process fixture\n"}).encode()
            status, pinned = request(1, "/submissions", pinned_data, "pinned-process-key-01")
            assert status == 202 and pinned["material_version"] == "m0-sum-v3"
            status, repeated = request(1, "/submissions", pinned_data, "pinned-process-key-01")
            assert status == 200 and repeated["id"] == pinned["id"]
            status, original = request(1, "/submission-receipt")
            assert status == 200 and original["id"] in ids and original["material_version"] == "m0-sum-v1"
            print("PASS: two API processes, 24 concurrent requests, one submission, restart and receipt recovery")
            print("PASS: explicit v3 registration/acceptance, repeat and historical v1 receipt")
        finally:
            for process in processes:
                if process.poll() is None:
                    process.terminate()
                    try:
                        process.wait(timeout=10)
                    except subprocess.TimeoutExpired:
                        process.kill()
                        process.wait(timeout=5)
            for log in logs:
                log.close()


if __name__ == "__main__":
    main()
