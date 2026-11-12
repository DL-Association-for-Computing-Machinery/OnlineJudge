"""Run the pinned private bundle through the real Worker and a disposable mTLS fixture."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import signal
import stat
import subprocess
import sys
import time
import uuid


ROOT = Path(__file__).resolve().parents[2]
LOCK = ROOT / "config/problem-sources/library-checker.json"
DOCKER = ["docker", "--context", "desktop-linux"]
SOURCES = ("aplusb", "unionfind", "zalgorithm", "suffixarray", "primality_test",
           "static_range_sum", "associative_array", "point_add_range_sum", "staticrmq", "convolution_mod")
SLOTS = ("oj-m0-sandbox", "oj-m0-sandbox-secondary", "oj-reference-sandbox",
         "oj-reference-medium-sandbox", "oj-reference-convolution-sandbox")
CREDS = ("ca.pem", "cert.pem", "key.pem")
FIXTURE_OWNER = "onlinejudge.live-reference-test"
BUILD = """
set -eu
export GOPROXY=off GOSUMDB=off
go mod verify
go vet -tags=integration,live_reference ./tests/integration
go test -c -tags=integration,live_reference -o /build/reference.test ./tests/integration
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -o /build/worker ./cmd/worker
"""
FIXTURE = """
set -eu
umask 077
test "$(id -u)" != 0
test "$(stat -c '%u %a' "$OJ_PRIVATE_DRAFT_BUNDLE_DIR")" = "$(id -u) 700"
d=$(mktemp -d /tmp/oj-postgres.XXXXXX)
trap 'pg_ctl -D "$d/data" -m fast -w stop >/dev/null 2>&1' EXIT
initdb -D "$d/data" --username=oj_test --auth-local=trust --auth-host=reject --no-locale --encoding=UTF8 >/dev/null
pg_ctl -D "$d/data" -l "$d/server.log" -o "-k $d -c listen_addresses='' -c max_connections=30" -w start >/dev/null
export OJ_TEST_DATABASE_URL="host=$d user=oj_test dbname=postgres sslmode=disable"
/build/reference.test -test.run '^TestLiveReferenceReports$' -test.v -test.timeout=15m
"""


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def docker(*args):
    return subprocess.check_output([*DOCKER, *args], text=True, timeout=20).strip()


def inspect_slots():
    states = docker("inspect", "--format", "{{.Name}} {{.State.Status}} {{.State.Pid}} {{json .ExecIDs}} {{.Config.Labels.owner}}", *SLOTS)
    expected = [f"/{name} exited 0 null onlinejudge.desktop-test" for name in SLOTS]
    require(states.splitlines() == expected, "expected all five owned sandbox slots stopped")
    ownership = Path.home() / ".onlinejudge-m0"
    locks = ("host-execution.lock", "slot.lock", "slot-secondary.lock", "slot-reference.lock",
             "slot-reference-medium.lock", "slot-reference-convolution.lock")
    require(all(not (ownership / name).exists() or (ownership / name).stat().st_size == 0 for name in locks),
            "host ownership markers need maintenance")
    return states + "\n"


def bind(source, target, readonly=True):
    require("," not in str(source), "bind path cannot contain a comma")
    return ["--mount", f"type=bind,src={source},dst={target}" + (",readonly" if readonly else "")]


def await_file(path, fixture, timeout=90):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if path.is_file():
            return json.loads(path.read_text())
        require(fixture.poll() is None, "fixture ended before writing " + path.name)
        time.sleep(0.25)
    raise RuntimeError("fixture timed out waiting for " + path.name)


def run_worker(worker, args, env, log):
    with log.open("xb") as output:
        with subprocess.Popen([worker, "run-reference", *args], env=env, stdout=output,
                              stderr=subprocess.STDOUT, start_new_session=True) as proc:
            try:
                return proc.wait()
            except KeyboardInterrupt:
                # Let the Worker handle cancellation and complete its own Stop.
                if proc.poll() is None:
                    proc.send_signal(signal.SIGTERM)
                proc.wait()
                raise


def stop_fixture(fixture, cidfile, owner):
    if fixture.poll() is not None:
        return
    if cidfile.is_file():
        cid = cidfile.read_text().strip()
        require(re.fullmatch(r"[0-9a-f]{64}", cid), "missing fixture container identity")
        actual = docker("inspect", "--format", "{{.Config.Labels.owner}} {{.Config.Labels.run}}", cid)
        require(actual == f"{FIXTURE_OWNER} {owner}", "refusing to stop a foreign fixture")
        docker("stop", "--time", "8", cid)
    fixture.wait(timeout=25)


def run(bundle, toolchain, out, supplement):
    subprocess.run([sys.executable, "-B", ROOT / "scripts/import-library-checker.py",
                    "--verify-bundle", bundle], cwd=ROOT, check=True)
    (out / "slots-before.txt").write_text(inspect_slots())
    image = docker("image", "inspect", "--format", "{{.Id}}", toolchain)
    require(re.fullmatch(r"sha256:[0-9a-f]{64}", image), "expected one existing local toolchain image")
    build, reports = out / "build", out / "reports"
    build.mkdir(mode=0o700)
    reports.mkdir(mode=0o700)
    common = [*DOCKER, "run", "--rm", "--pull", "never", "--cap-drop", "ALL",
              "--security-opt", "no-new-privileges:true", *bind(ROOT, "/src")]
    with (out / "build.log").open("xb") as log:
        subprocess.run([*common, "--network", "none", *bind(build, "/build", False),
                        "--workdir", "/src", "--entrypoint", "/bin/sh", image, "-ec", BUILD],
                       stdout=log, stderr=subprocess.STDOUT, check=True, timeout=300)
    owner = uuid.uuid4().hex
    name, cidfile = "oj-live-reference-" + owner[:12], out / "fixture.cid"
    with (out / "fixture.log").open("xb") as log:
        fixture = subprocess.Popen([*common, "--init", "--name", name, "--cidfile", cidfile,
                                    "--label", "owner=" + FIXTURE_OWNER, "--label", "run=" + owner,
                                    "--network", "bridge", "--publish", "127.0.0.1::18443",
                                    "--read-only", "--cpus", "2", "--memory", "4g", "--pids-limit", "128",
                                    "--tmpfs", "/tmp:rw,exec,nosuid,size=3g,mode=1777",
                                    *bind(build, "/build"), *bind(bundle.parent, "/inputs"),
                                    *bind(reports, "/reports", False), "--workdir", "/src/tests/integration",
                                    "-e", "OJ_LIVE_REFERENCE_FIXTURE=1", "-e", "OJ_LIVE_REFERENCE_OUTPUT=/reports",
                                    "-e", "OJ_LIVE_REFERENCE_SUPPLEMENT=" + supplement,
                                    "-e", "OJ_PRIVATE_DRAFT_BUNDLE_DIR=/inputs/" + bundle.name,
                                    "--entrypoint", "/bin/sh", image, "-ec", FIXTURE],
                                   stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
        try:
            result = []
            selected_source = "zalgorithm" if supplement == "zalgorithm-alternating" else supplement
            for source in ((selected_source,) if selected_source else SOURCES):
                ready = await_file(reports / f"ready-{source}.json", fixture)
                port = docker("port", name, "18443/tcp")
                require(re.fullmatch(r"127\.0\.0\.1:[0-9]+", port), "fixture must publish only to loopback")
                sid = "library-checker:" + source
                require(ready["source_id"] == sid and re.fullmatch(r"[a-f0-9]{32}", ready["request_id"])
                        and re.fullmatch(r"[a-f0-9]{64}", ready["plan_sha256"]), "unexpected fixture request")
                env = os.environ.copy()
                for name_to_clear in ("OJ_WORKER_M0_SLOT", "OJ_WORKER_M0_DUAL_EXPERIMENT"):
                    env.pop(name_to_clear, None)
                env.update(OJ_WORKER_CONTROLLER_URL="https://" + port,
                           OJ_WORKER_CA_FILE=str(reports / CREDS[0]),
                           OJ_WORKER_CERT_FILE=str(reports / CREDS[1]),
                           OJ_WORKER_KEY_FILE=str(reports / CREDS[2]))
                print("Running", sid, flush=True)
                code = run_worker(build / "worker", (ready["request_id"], sid, ready["plan_sha256"]),
                                  env, out / f"worker-{source}.log")
                require(code == 0, f"{sid} Worker failed; batch stopped")
                report = await_file(reports / f"accepted-{source}.json", fixture)
                require(report["source_id"] == sid and report["formal_plan_sha256"] == ready["plan_sha256"],
                        "accepted report does not match the dispatched plan")
                result.append(report)
                print(f"Accepted {report['formal_ac']} formal points", flush=True)
            require(fixture.wait(timeout=30) == 0, "fixture did not complete successfully")
            (out / "slots-after.txt").write_text(inspect_slots())
            require(not docker("ps", "-a", "--filter", "label=run=" + owner, "--format", "{{.ID}}"),
                    "temporary fixture container was not removed")
            formal, comparison = {
                "suffixarray": (52, 0),
                "convolution_mod": (55, 24),
                "zalgorithm-alternating": (32, 0),
                "primality_test": (15, 0),
                "associative_array": (22, 0), "zalgorithm": (31, 0), "aplusb": (17, 0), "unionfind": (20, 0),
                "static_range_sum": (13, 0), "point_add_range_sum": (23, 0), "staticrmq": (28, 0), "": (254, 24),
            }[supplement]
            require(sum(row["formal_ac"] for row in result) == formal and sum(row["comparison_matched"] for row in result) == comparison,
                    "incomplete accepted coverage")
            (out / "summary.json").write_text(json.dumps({
                "index_sha256": hashlib.sha256((bundle / "index.json").read_bytes()).hexdigest(),
                "source_lock_sha256": hashlib.sha256(LOCK.read_bytes()).hexdigest(),
                "toolchain_image": image, "problems": result,
            }, indent=2) + "\n")
        finally:
            stop_fixture(fixture, cidfile, owner)
            for credential in CREDS:
                (reports / credential).unlink(missing_ok=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bundle", required=True, type=Path)
    parser.add_argument("--toolchain-image", required=True)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--start-stopped-slots", required=True, action="store_true")
    local = parser.add_mutually_exclusive_group()
    local.add_argument("--suffixarray-local-data", dest="supplement", action="store_const", const="suffixarray",
                       help="run only the pinned 52-point local suffix array revision")
    local.add_argument("--convolution-local-data", dest="supplement", action="store_const", const="convolution_mod",
                       help="run only the pinned 55-point local convolution revision")
    local.add_argument("--primality-test-local-data", dest="supplement", action="store_const", const="primality_test",
                       help="run only the pinned 15-point local primality test revision")
    local.add_argument("--associative-array-local-data", dest="supplement", action="store_const", const="associative_array",
                       help="run only the pinned 22-point local associative-array revision")
    local.add_argument("--zalgorithm-local-data", dest="supplement", action="store_const", const="zalgorithm",
                       help="run only the pinned 31-point local Z-algorithm revision")
    local.add_argument("--zalgorithm-alternating-data", dest="supplement", action="store_const",
                       const="zalgorithm-alternating", help="run only the pinned 32-point local Z-algorithm revision")
    local.add_argument("--aplusb-local-data", dest="supplement", action="store_const", const="aplusb",
                       help="run only the pinned 17-point local A+B revision")
    local.add_argument("--unionfind-local-data", dest="supplement", action="store_const", const="unionfind",
                       help="run only the pinned 20-point local unionfind revision")
    local.add_argument("--static-rmq-local-data", dest="supplement", action="store_const", const="staticrmq",
                       help="run only the pinned 28-point local static range minimum revision")
    local.add_argument("--point-add-range-sum-local-data", dest="supplement", action="store_const", const="point_add_range_sum",
                       help="run only the pinned 23-point local point-add range sum revision")
    local.add_argument("--static-range-sum-local-data", dest="supplement", action="store_const", const="static_range_sum",
                       help="run only the pinned 13-point local static range sum revision")
    args = parser.parse_args()
    require(platform.system() == "Darwin" and platform.machine() == "arm64", "requires macOS arm64")
    require(os.environ.get("OJ_SANDBOX_EXPERIMENT") == "1", "requires OJ_SANDBOX_EXPERIMENT=1")
    bundle = args.bundle.resolve(strict=True)
    out = args.output.resolve()
    parent = bundle.parent
    parent_info = parent.stat()
    require(bundle.is_dir() and parent_info.st_uid == os.getuid()
            and stat.S_IMODE(parent_info.st_mode) == 0o700
            and set(parent.iterdir()) == {bundle},
            "expected an owner-private parent containing only the verified bundle")
    require(not out.is_relative_to(ROOT) and not out.is_relative_to(bundle.parent),
            "output must be outside the repository and prepared bundle parent")
    out.mkdir(mode=0o700)
    run(bundle, args.toolchain_image, out, args.supplement or "")


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, OSError, ValueError, KeyError, subprocess.SubprocessError) as exc:
        sys.exit(f"Live reference acceptance failed: {exc}")
