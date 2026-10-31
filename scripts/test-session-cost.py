#!/usr/bin/env python3
"""Characterize test-only direct session Store costs in disposable containers."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import selectors
import subprocess
import sys
import tempfile
import time
import uuid

ROOT = Path(__file__).resolve().parents[1]
MAX_LINE = 128 * 1024
MAX_LOG = 1024 * 1024
COUNTS = ("success", "expected_reject", "stale", "timeout", "unknown", "error",
          "missing", "payload_mismatch")
PG_SCRIPT = """set -eu
initdb -D /tmp/pg -U oj_cost --auth-local=trust --auth-host=trust >/dev/null
# initdb's host entries cover loopback, not the new private Docker subnet.
cat >>/tmp/pg/pg_hba.conf <<'HBA'
host postgres oj_cost samenet trust
HBA
pg_ctl -D /tmp/pg -o '-h 0.0.0.0 -k /tmp' -w start >/dev/null
trap 'pg_ctl -D /tmp/pg -m immediate -w stop >/dev/null' EXIT TERM INT
psql -h /tmp -U oj_cost -d postgres -v ON_ERROR_STOP=1 -c "CREATE TABLE session_cost_owner(nonce text); INSERT INTO session_cost_owner VALUES ('$1')" >/dev/null
while :; do sleep 1; done
"""


def reject_constant(value):
    raise ValueError("non-finite JSON value: " + value)


def error_text(exc):
    text = f"{type(exc).__name__}: {exc}"
    for field in ("stdout", "stderr"):
        value = getattr(exc, field, None)
        if value:
            if isinstance(value, bytes):
                value = value.decode("utf-8", errors="replace")
            text += "\n" + field + ": " + value[-65536:]
    return text


def validate_result(result, config):
    if not isinstance(result, dict):
        raise ValueError("result must be an object")
    for key, expected in {**config, "schema": 1, "concurrency": 8,
                          "padding_bytes": 1024}.items():
        if type(result.get(key)) is not type(expected) or result[key] != expected:
            raise ValueError("invalid result field: " + key)
    if result.get("valid") is not True:
        raise ValueError("result is not valid")
    elapsed = result.get("elapsed_ns")
    if type(elapsed) is not int or elapsed <= 0:
        raise ValueError("invalid elapsed_ns")
    counts = result.get("counts")
    if not isinstance(counts, dict) or set(counts) != set(COUNTS) or any(type(counts.get(k)) is not int or
                                           counts[k] < 0 for k in COUNTS):
        raise ValueError("invalid counts")
    target = "expected_reject" if config["workload"] == "reject" else "success"
    if any(counts[k] != (config["operations"] if k == target else 0) for k in COUNTS):
        raise ValueError("unexpected operation counts")
    latency = result.get("latency_ns")
    if not isinstance(latency, dict) or set(latency) != set(COUNTS):
        raise ValueError("missing latency arrays")
    for key, values in latency.items():
        if (not isinstance(values, list) or len(values) != counts[key] or
                any(type(v) is not int or v <= 0 for v in values)):
            raise ValueError("invalid latency array")
    return result


class Protocol:
    def __init__(self, nonce, config):
        self.nonce, self.config, self.state = nonce, config, "ready"
        self.result = None

    def line(self, line):
        if not line.startswith(b"OJ_SESSION_COST_"):
            return None
        try:
            text = line.decode("utf-8")
        except UnicodeDecodeError as exc:
            raise ValueError("invalid protocol encoding") from exc
        parts = text.split(" ", 2)
        if len(parts) < 2 or parts[1] != self.nonce:
            raise ValueError("invalid protocol nonce")
        if parts[0] == "OJ_SESSION_COST_READY" and len(parts) == 2 and self.state == "ready":
            self.state = "measured"
            return "ready"
        if parts[0] == "OJ_SESSION_COST_MEASURED" and len(parts) == 2 and self.state == "measured":
            self.state = "result"
            return "measured"
        if parts[0] == "OJ_SESSION_COST_RESULT" and len(parts) == 3 and self.state == "result":
            self.result = validate_result(json.loads(parts[2], parse_constant=reject_constant), self.config)
            self.state = "done"
            return "result"
        raise ValueError("unexpected protocol message or order")


class Docker:
    def __init__(self, context, owner):
        self.command = ["docker", "--context", context]
        self.owner = owner
        self.containers = []
        self.networks = []
        self.deadline = None

    def run(self, *args, timeout=30):
        if self.deadline is not None:
            remaining = self.deadline - time.monotonic()
            if remaining <= 0:
                raise TimeoutError("experiment deadline exceeded")
            timeout = min(timeout, remaining)
        return subprocess.run([*self.command, *args], check=True, capture_output=True,
                              text=True, timeout=timeout).stdout.strip()

    def network(self, name):
        ident = self.run("network", "create", "--internal", "--label",
                         "oj.session-cost.owner=" + self.owner, name)
        self.networks.append(ident)
        return ident

    def create(self, *args):
        ident = self.run("create", "--label", "oj.session-cost.owner=" + self.owner, *args)
        self.containers.append(ident)
        return ident

    def cleanup(self):
        self.deadline = None
        errors = []
        for kind, ids in (("container", self.containers), ("network", self.networks)):
            while ids:
                ident = ids.pop()
                try:
                    self.run(kind, "rm", *( ["-f"] if kind == "container" else []), ident, timeout=15)
                except (subprocess.SubprocessError, OSError) as exc:
                    errors.append(f"{kind} {ident}: {type(exc).__name__}")
        return errors


def snapshot(docker, container):
    begin = time.monotonic_ns()
    values = {}
    for filename in ("cpu.stat", "memory.current", "memory.peak", "memory.stat"):
        values[filename] = docker.run("exec", container, "cat", "/sys/fs/cgroup/" + filename,
                                      timeout=5)
    end = time.monotonic_ns()
    def fields(raw):
        return dict(line.split() for line in raw.splitlines())
    cpu = fields(values["cpu.stat"])
    memory = fields(values["memory.stat"])
    result = {"capture_begin_monotonic_ns": begin, "capture_end_monotonic_ns": end,
            "raw": values, "usage_usec": int(cpu["usage_usec"]),
            "memory_current_bytes": int(values["memory.current"]),
            "memory_peak_bytes": int(values["memory.peak"]),
            "memory_stat_bytes": {k: int(memory[k]) for k in ("anon", "file", "shmem")}}
    if (end <= begin or min(result["usage_usec"], result["memory_current_bytes"],
                            result["memory_peak_bytes"], *result["memory_stat_bytes"].values()) < 0):
        raise ValueError("invalid cgroup snapshot")
    return result


def resource_windows(before, after):
    windows = {}
    for name, first in before.items():
        last = after[name]
        delta = last["usage_usec"] - first["usage_usec"]
        if (delta < 0 or not first["capture_begin_monotonic_ns"] < first["capture_end_monotonic_ns"]
                <= last["capture_begin_monotonic_ns"] < last["capture_end_monotonic_ns"]):
            raise ValueError("invalid cgroup resource window")
        windows[name] = {"cpu_usage_usec_delta": delta,
                         "window_begin_monotonic_ns": first["capture_begin_monotonic_ns"],
                         "window_end_monotonic_ns": last["capture_end_monotonic_ns"]}
    return windows


def check_container_state(docker, container, running):
    state = json.loads(docker.run("inspect", "--format", "{{json .State}}", container, timeout=5))
    if (state.get("Running") is not running or state.get("OOMKilled") is not False or
            (not running and (type(state.get("ExitCode")) is not int or state["ExitCode"] != 0))):
        raise RuntimeError("container did not reach expected healthy state")


def run_protocol(docker, loadgen, stores, nonce, config, record, deadline):
    protocol = Protocol(nonce, config)
    raw = bytearray()
    total = 0
    with subprocess.Popen([*docker.command, "start", "-a", loadgen], stdout=subprocess.PIPE,
                          stderr=subprocess.STDOUT) as process:
        try:
            with selectors.DefaultSelector() as selector:
                selector.register(process.stdout, selectors.EVENT_READ)
                pending = b""
                while selector.get_map():
                    if time.monotonic() >= deadline:
                        raise TimeoutError("sample deadline exceeded")
                    if not selector.select(timeout=0.2):
                        continue
                    chunk = os.read(process.stdout.fileno(), 65536)
                    if not chunk:
                        selector.unregister(process.stdout)
                        break
                    raw.extend(chunk[:max(0, MAX_LOG - len(raw))])
                    total += len(chunk)
                    if total > MAX_LOG:
                        raise ValueError("load generator log limit exceeded")
                    pending += chunk
                    while b"\n" in pending:
                        line, pending = pending.split(b"\n", 1)
                        if len(line) > MAX_LINE:
                            raise ValueError("load generator line limit exceeded")
                        event = protocol.line(line.rstrip(b"\r"))
                        if event == "ready":
                            record["before"] = {name: snapshot(docker, ident) for name, ident in stores.items()}
                            docker.run("exec", loadgen, "touch", "/tmp/oj-cost-start", timeout=5)
                        elif event == "measured":
                            record["after"] = {name: snapshot(docker, ident) for name, ident in stores.items()}
                            record["resource_windows"] = resource_windows(record["before"], record["after"])
                            docker.run("exec", loadgen, "touch", "/tmp/oj-cost-verify", timeout=5)
                        elif event == "result":
                            record["result"] = protocol.result
                            docker.run("exec", loadgen, "touch", "/tmp/oj-cost-ack", timeout=5)
                    if len(pending) > MAX_LINE:
                        raise ValueError("load generator line limit exceeded")
                if pending:
                    raise ValueError("unterminated load generator output")
                status = process.wait(timeout=max(0.1, deadline - time.monotonic()))
                if status or protocol.state != "done":
                    raise RuntimeError("load generator failed or protocol incomplete")
                check_container_state(docker, loadgen, False)
                for name, container in stores.items():
                    if name != "loadgen":
                        check_container_state(docker, container, True)
                docker.run("exec", stores["postgres"], "pg_isready", "-h", "/tmp", "-U", "oj_cost", timeout=5)
        except BaseException:
            record["failure_stdout"] = raw.decode("utf-8", errors="replace")
            record["failure_stdout_truncated"] = total > MAX_LOG
            raise
        finally:
            if process.poll() is None:
                process.kill()
                process.wait(timeout=10)


def bounded(value, low, high):
    number = int(value)
    if not low <= number <= high:
        raise argparse.ArgumentTypeError(f"must be between {low} and {high}")
    return number


def image_info(docker, image):
    info = docker.run("image", "inspect", "--format", "{{.Id}} {{.Os}} {{.Architecture}}", image).split()
    if len(info) != 3 or not re.fullmatch(r"sha256:[0-9a-f]{64}", info[0]):
        raise ValueError("invalid local image inspection")
    return {"id": info[0], "os": info[1], "architecture": info[2]}


def write_output(path, data):
    # The destination was exclusively created by this invocation. Replace it
    # atomically only with a sibling temporary file from this invocation.
    fd, name = tempfile.mkstemp(prefix=".oj-cost-", dir=path.parent)
    try:
        with os.fdopen(fd, "w") as stream:
            json.dump(data, stream, indent=2, allow_nan=False)
            stream.write("\n")
        os.replace(name, path)
    finally:
        if os.path.exists(name):
            os.unlink(name)


def wait_ready(docker, container, command, deadline):
    for _ in range(100):
        if time.monotonic() >= deadline:
            raise TimeoutError("sample deadline exceeded during storage startup")
        try:
            if docker.run("exec", container, *command, timeout=3):
                return
        except subprocess.SubprocessError:
            pass
        time.sleep(0.2)
    raise RuntimeError("storage did not become ready")


def completed_run(complete, samples, rounds, cleanup):
    return (complete and not cleanup and len(samples) == rounds * 14 and
            all(sample.get("status") == "passed" for sample in samples))


def sample(docker, images, binary, fixture, backend, workload, subjects, operations, deadline, record):
    if time.monotonic() >= deadline:
        raise TimeoutError("experiment deadline exceeded")
    docker.deadline = deadline
    nonce = uuid.uuid4().hex
    prefix = "oj-cost-" + uuid.uuid4().hex
    network = docker.network(prefix)
    common = ["--network", network, "--cpus", "1", "--cap-drop", "ALL",
              "--security-opt", "no-new-privileges"]
    pg = docker.create("--name", prefix + "-pg", *common, "--network-alias", "postgres",
                       "--memory", "512m", "--tmpfs", "/tmp:rw,size=512m,mode=1777",
                       "--entrypoint", "sh", images["toolchain"]["id"], "-ec", PG_SCRIPT, "sh", nonce)
    docker.run("start", pg)
    # Marker existence and matching nonce establish readiness, not pg_isready alone.
    wait_ready(docker, pg, ["psql", "-h", "/tmp", "-U", "oj_cost", "-d", "postgres", "-Atc",
                            "SELECT nonce FROM session_cost_owner WHERE nonce = '" + nonce + "'"], deadline)
    stores = {"postgres": pg}
    if backend == "redis":
        redis = docker.create("--name", prefix + "-redis", *common, "--network-alias", "redis",
                              "--memory", "512m", "--entrypoint", "redis-server", images["redis"]["id"],
                              "--bind", "0.0.0.0", "--protected-mode", "no", "--save", "",
                              "--appendonly", "no", "--maxmemory", "64mb", "--maxmemory-policy", "noeviction")
        docker.run("start", redis)
        wait_ready(docker, redis, ["redis-cli", "ping"], deadline)
        stores["redis"] = redis
    config = {"backend": backend, "workload": workload, "subjects": subjects, "operations": operations,
              "fixture_sha256": hashlib.sha256(fixture.read_bytes()).hexdigest()}
    env = {"OJ_SESSION_COST": "1", "OJ_SESSION_COST_NONCE": nonce, "OJ_SESSION_COST_BACKEND": backend,
           "OJ_SESSION_COST_WORKLOAD": workload, "OJ_SESSION_COST_SUBJECTS": str(subjects),
           "OJ_SESSION_COST_OPS": str(operations)}
    env_args = [item for key, value in env.items() for item in ("-e", key + "=" + value)]
    loadgen = docker.create("--name", prefix + "-load", *common, "--memory", "1g", *env_args,
                            "--mount", f"type=bind,src={binary},dst=/cost.test,readonly",
                            "--mount", f"type=bind,src={fixture},dst=/cost-fixture.json,readonly",
                            "--entrypoint", "/cost.test", images["toolchain"]["id"],
                            "-test.run", "^TestSessionCostMeasurement$", "-test.count=1",
                            "-test.timeout=90s", "-test.v")
    stores["loadgen"] = loadgen
    run_protocol(docker, loadgen, stores, nonce, config, record, deadline)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--context", default="desktop-linux")
    parser.add_argument("--toolchain-image", default="oj-toolchain:local")
    parser.add_argument("--redis-image", default="sha256:92dbfb0bc408385074c83eb3a868b5f273d425f079f2af808403d22dcd94b667")
    parser.add_argument("--module-cache", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--rounds", type=lambda x: bounded(x, 1, 3), default=3)
    parser.add_argument("--operations", type=lambda x: bounded(x, 8, 8192), default=2048)
    args = parser.parse_args()
    cache = Path(args.module_cache).resolve(strict=True)
    if not cache.is_dir() or any("," in str(p) for p in (cache, ROOT)):
        parser.error("module cache must be a directory; bind paths cannot contain commas")
    output = Path(args.output).absolute()
    fd = os.open(output, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    os.close(fd)
    owner = uuid.uuid4().hex
    docker = Docker(args.context, owner)
    data = {"schema": 1, "status": "running", "samples": [],
            "metadata": {"context": args.context, "owner_label": "oj.session-cost.owner=" + owner,
                         "rounds": args.rounds, "operations": args.operations,
                         "concurrency": 8, "padding_bytes": 1024,
                         "scope": "test-only direct Store operations; exploratory, not API or SLA",
                         "topology": "fresh private internal network and PostgreSQL per sample; Redis added for Redis samples",
                         "limits": {"storage": "1 CPU, 512 MiB each", "loadgen": "1 CPU, 1 GiB"},
                         "durability": "PostgreSQL ephemeral tmpfs; Redis SAVE disabled, appendonly disabled, noeviction, maxmemory 64mb",
                         "resource_note": "PostgreSQL included for both backends; memory.peak includes container lifetime and initialization; noisy endpoint windows include observer commands and waiting but end before semantic validation and storage statistics, and are wider than Go elapsed"}}
    complete = False
    deadline = time.monotonic() + 1800
    try:
        write_output(output, data)
        images = {"toolchain": image_info(docker, args.toolchain_image), "redis": image_info(docker, args.redis_image)}
        data["metadata"]["images"] = images
        with tempfile.TemporaryDirectory(prefix="oj-cost-build-") as tmp:
            binary = Path(tmp) / "cost.test"
            fixture = Path(tmp) / "fixture.json"
            build = docker.create("--name", "oj-cost-build-" + owner, "--network", "none", "--cpus", "1",
                                  "--memory", "1g", "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
                                  "--user", f"{os.getuid()}:{os.getgid()}",
                                  "--mount", f"type=bind,src={ROOT / 'tests/sessionexperiment'},dst=/experiment,readonly",
                                  "--mount", f"type=bind,src={cache},dst=/go/pkg/mod,readonly",
                                  "--mount", f"type=bind,src={tmp},dst=/out", "--workdir", "/experiment",
                                  "-e", "CGO_ENABLED=0", "-e", "GOPROXY=off", "-e", "GOTOOLCHAIN=local",
                                  "-e", "GOCACHE=/tmp/oj-cost-build-cache", "-e", "OJ_SESSION_COST_PREPARE=1",
                                  "-e", "OJ_SESSION_COST_OPS=" + str(args.operations),
                                  "-e", "OJ_SESSION_COST_NONCE=" + owner,
                                  "-e", "OJ_SESSION_COST_FIXTURE_OUT=/out/fixture.json",
                                  "--entrypoint", "sh", images["toolchain"]["id"], "-ec",
                                  "go test -mod=readonly -c -o /out/cost.test; "
                                  "/out/cost.test -test.run '^TestSessionCostPrepare$' -test.count=1 -test.v")
            docker.run("start", "-a", build, timeout=300)
            check_container_state(docker, build, False)
            binary.chmod(0o755)
            fixture.chmod(0o644)
            data["metadata"]["fixture_sha256"] = hashlib.sha256(fixture.read_bytes()).hexdigest()
            cases = [("find", 32), ("save", 32), ("admit", args.operations), ("reject", 32),
                     ("find", 1), ("save", 1), ("reject", 1)]
            for round_number in range(1, args.rounds + 1):
                for backend in (("redis", "postgres") if round_number % 2 else ("postgres", "redis")):
                    for workload, subjects in cases:
                        record = {"round": round_number, "backend": backend, "workload": workload,
                                  "subjects": subjects, "operations": args.operations, "status": "running"}
                        data["samples"].append(record)
                        print(f"round {round_number}: {backend} {workload} subjects={subjects}", flush=True)
                        try:
                            sample(docker, images, binary, fixture, backend, workload, subjects, args.operations,
                                   min(deadline, time.monotonic() + 120), record)
                            record["status"] = "passed"
                        except BaseException as exc:
                            record.update(status="failed", error=error_text(exc))
                            raise
                        finally:
                            cleanup = docker.cleanup()
                            if cleanup:
                                record["cleanup_errors"] = cleanup
                                record["status"] = "failed"
                            write_output(output, data)
                            if cleanup:
                                raise RuntimeError("resource cleanup failed")
            complete = True
    except BaseException as exc:
        complete = False
        data["error"] = error_text(exc)
        print("session cost experiment failed: " + data["error"], file=sys.stderr)
        if not isinstance(exc, Exception):
            raise
    finally:
        cleanup = docker.cleanup()
        if cleanup:
            data["cleanup_errors"] = cleanup
        data["status"] = "passed" if completed_run(complete, data["samples"], args.rounds, cleanup) else "failed"
        write_output(output, data)
    return 0 if data["status"] == "passed" else 1


if __name__ == "__main__":
    sys.exit(main())
