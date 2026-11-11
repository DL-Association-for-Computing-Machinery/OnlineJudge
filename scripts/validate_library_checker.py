#!/usr/bin/env python3
"""Container-only validation of pinned official source; no production judging."""

import importlib.util
import json
import logging
from pathlib import Path
import re
import resource
import signal
import subprocess
import sys
import tempfile
import time
import tomllib
import types

from library_checker_quality import (
    SLOW_CPU_SECONDS, SLOW_HARD_CPU_SECONDS, SLOW_SOLUTION, SLOW_SOURCE, SLOW_WALL_SECONDS,
    WRONG_SOLUTIONS, control_result, input_controls, slow_solution_result, wrong_solution_result,
)
from library_checker_oracles import MAX_OUTPUT_BYTES, WALL_SECONDS, oracle_result, small_cases
from library_checker_wrong_cases import (
    LOCAL_WRONG_CASES, LOCAL_WRONG_SCHEMA, MAX_INPUT_BYTES as WRONG_MAX_INPUT_BYTES,
    MAX_OUTPUT_BYTES as WRONG_MAX_OUTPUT_BYTES, WALL_SECONDS as WRONG_WALL_SECONDS, fixed_cases, local_wrong_result,
)
from problem_source_common import digest, expected_materials, load_lock, read_regular, snapshot


def checked(command, **kwargs):
    """All compiler, generator, solution and checker children have wall limits."""
    kwargs.setdefault("timeout", 120)
    kwargs.setdefault("stdout", subprocess.DEVNULL)
    kwargs.setdefault("stderr", subprocess.DEVNULL)
    kwargs.setdefault("check", True)
    return subprocess.run(command, **kwargs)


def run_input_controls(verifier, source_id):
    observations = []
    for name, payload, expected_exit in input_controls(source_id):
        result = checked([str(verifier)], input=payload, check=False)
        if result.returncode != expected_exit:
            raise ValueError(f"{source_id}: input validator control {name} exited {result.returncode}, expected {expected_exit}")
        observations.append(control_result(name, payload, result.returncode))
    return observations


def run_wrong_solution(base, item, inventory, output):
    name = WRONG_SOLUTIONS[item["path"]]
    for position, case in enumerate(item["cases"], 1):
        if case["sample"]:
            continue
        infile = base / "in" / (case["id"] + ".in")
        answer = base / "out" / (case["id"] + ".out")
        with infile.open("rb") as stream, output.open("wb") as actual:
            candidate = checked([str(base / "sol" / Path(name).stem)], stdin=stream,
                                stdout=actual, timeout=15, check=False)
        if candidate.returncode != 0:
            raise ValueError(f"{item['source_id']}: wrong solution exited {candidate.returncode} at point {position}")
        result = checked([str(base / "checker"), str(infile), str(output), str(answer)], timeout=15, check=False)
        if result.returncode == 1:
            return wrong_solution_result(item, position, inventory)
        if result.returncode != 0:
            raise ValueError(f"{item['source_id']}: checker exited {result.returncode} at point {position}")
    raise ValueError(f"{item['source_id']}: no formal point rejected the wrong solution")


def cpu_limit():
    resource.setrlimit(resource.RLIMIT_CPU, (SLOW_CPU_SECONDS, SLOW_HARD_CPU_SECONDS))


def run_cpu_bounded(program, infile, output):
    # This driver is single threaded and waits for each child before starting
    # the next. The delta measures this invocation, not compiler/checker work.
    before = resource.getrusage(resource.RUSAGE_CHILDREN)
    with infile.open("rb") as stream, output.open("wb") as actual:
        result = checked([str(program)], stdin=stream, stdout=actual, timeout=SLOW_WALL_SECONDS,
                         check=False, preexec_fn=cpu_limit)
    after = resource.getrusage(resource.RUSAGE_CHILDREN)
    cpu_ms = round(1000 * (after.ru_utime - before.ru_utime + after.ru_stime - before.ru_stime))
    return result.returncode, cpu_ms


def run_slow_solution(base, item, inventory, output):
    limit = tomllib.loads((base / "info.toml").read_text())["timelimit"]
    if limit != SLOW_CPU_SECONDS:
        raise ValueError("slow-solution budget differs from pinned source")
    for position, case in enumerate(item["cases"], 1):
        if case["sample"]:
            continue
        infile = base / "in" / (case["id"] + ".in")
        answer = base / "out" / (case["id"] + ".out")
        status, candidate_cpu_ms = run_cpu_bounded(base / "sol" / Path(SLOW_SOLUTION).stem, infile, output)
        if status == -signal.SIGXCPU:
            status, reference_cpu_ms = run_cpu_bounded(base / "sol" / "correct", infile, output)
            if status != 0:
                raise ValueError(f"reference failed under the same CPU limit at point {position}")
            witness = slow_solution_result(item, position, inventory, candidate_cpu_ms, reference_cpu_ms)
        elif status == 0:
            witness = None
        else:
            raise ValueError(f"slow solution exited {status} at point {position}")
        result = checked([str(base / "checker"), str(infile), str(output), str(answer)],
                         timeout=SLOW_WALL_SECONDS, check=False)
        if result.returncode != 0:
            raise ValueError(f"slow-solution control checker exited {result.returncode} at point {position}")
        if witness is not None:
            return witness
    raise ValueError("no formal point exhausted the slow solution's CPU budget")


def tokens(data):
    return re.split(rb"[ \t\r\n]+", data.strip(b" \t\r\n"))


def run_small_oracles(base, item, inventory, directory):
    probes = small_cases(item["source_id"])
    infile, output, expected = (directory / name for name in ("probe.in", "actual.out", "expected.out"))
    for case, data, answer in probes:
        try:
            checked([str(base / "verifier")], input=data, timeout=WALL_SECONDS)
            infile.write_bytes(data)
            expected.write_bytes(answer)
            with output.open("wb") as stream:
                checked([str(base / "sol" / "correct")], input=data, stdout=stream, timeout=WALL_SECONDS)
            if tokens(read_regular(directory, output.name, MAX_OUTPUT_BYTES)) != tokens(answer):
                raise ValueError("reference and independent oracle disagree")
            checked([str(base / "checker"), str(infile), str(output), str(expected)], timeout=WALL_SECONDS)
        except (OSError, ValueError, subprocess.SubprocessError) as exc:
            raise ValueError(f"{item['source_id']}: independent oracle {case} failed: {exc}") from exc
    return oracle_result(item, inventory, probes)


def run_local_wrong_case(base, item, inventory, output):
    for position, case in fixed_cases(item):
        infile = base / "in" / (case + ".in")
        answer = base / "out" / (case + ".out")
        data = read_regular(infile.parent, infile.name, WRONG_MAX_INPUT_BYTES)
        with output.open("wb") as stream:
            checked([sys.executable, "-B", str(Path(__file__).with_name("library_checker_wrong_cases.py")), item["source_id"]],
                    input=data, stdout=stream, timeout=WRONG_WALL_SECONDS)
        actual = tokens(read_regular(output.parent, output.name, WRONG_MAX_OUTPUT_BYTES))
        expected = tokens(read_regular(answer.parent, answer.name, WRONG_MAX_OUTPUT_BYTES))
        if len(actual) != len(expected) or not actual or any(not token.isdigit() for token in actual):
            raise ValueError(f"{item['source_id']}: malformed wrong-candidate output at formal point {position}")
        result = checked([str(base / "checker"), str(infile), str(output), str(answer)],
                         timeout=WRONG_WALL_SECONDS, check=False)
        if result.returncode == 1:
            return local_wrong_result(item, inventory, position)
        if result.returncode != 0:
            raise ValueError(f"{item['source_id']}: checker exited {result.returncode} at formal point {position}")
        print(item["source_id"], "local wrong candidate accepted at formal position", position, flush=True)
    raise ValueError(f"{item['source_id']}: fixed formal probes did not reject the wrong candidate")


def main():
    if Path.cwd() != Path("/work") or not Path("/input/source-lock.json").is_file():
        raise ValueError("run only through prepare-library-checker.py")
    resource.setrlimit(resource.RLIMIT_FSIZE, (128 << 20, 128 << 20))
    resource.setrlimit(resource.RLIMIT_CORE, (0, 0))
    source = Path("/work/source")
    source.mkdir()
    lock = load_lock(Path("/input/source-lock.json"))
    snapshot(Path("/input/source"), source, lock)
    expected, catalog = expected_materials(source, lock)
    # The pinned generator only uses toml.load(path). Use Python's TOML parser
    # through that narrow interface so no package download or source edit is needed.
    sys.modules["toml"] = types.SimpleNamespace(load=lambda path: tomllib.loads(Path(path).read_text()))
    spec = importlib.util.spec_from_file_location("pinned_lc_generator", source / "generate.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    module.check_call = lambda command, **kw: checked(command, **kw)
    module.run = checked
    logging.disable(logging.CRITICAL)
    report = []
    empty = Path("/work/invalid.out")
    empty.write_text("__OJ_EXTRA_TOKEN__\n")
    start = time.monotonic()
    for item in catalog:
        base = source / item["path"]
        problem = module.Problem(source, base)
        problem.generate(module.Problem.Mode.DEFAULT)
        problem.compile_verifier()
        for case in item["cases"]:
            name = case["id"]
            infile = base / "in" / (name + ".in")
            answer = base / "out" / (name + ".out")
            with infile.open("rb") as stream:
                checked([str(base / "verifier")], stdin=stream)
            checked([str(base / "checker"), str(infile), str(answer), str(answer)])
        sample = next(c["id"] for c in item["cases"] if c["sample"])
        result = checked([str(base / "checker"), str(base / "in" / (sample + ".in")),
                          str(empty), str(base / "out" / (sample + ".out"))], check=False)
        if result.returncode != 1:
            raise ValueError("negative checker control did not return WA")
        controls = run_input_controls(base / "verifier", item["source_id"])
        wrong_solution = None
        if name := WRONG_SOLUTIONS.get(item["path"]):
            module.compile(base / "sol" / name, source)
            with tempfile.TemporaryDirectory(dir="/work") as directory:
                wrong_solution = run_wrong_solution(base, item, expected, Path(directory) / "actual.out")
            print(item["source_id"], "wrong solution rejected at formal position",
                  wrong_solution["case_position"], flush=True)
        slow_solution = None
        if item["path"] == SLOW_SOURCE:
            module.compile(base / "sol" / SLOW_SOLUTION, source)
            with tempfile.TemporaryDirectory(dir="/work") as directory:
                slow_solution = run_slow_solution(base, item, expected, Path(directory) / "actual.out")
            print(item["source_id"], "slow solution exceeded CPU budget at formal position",
                  slow_solution["case_position"], flush=True)
        with tempfile.TemporaryDirectory(dir="/work") as directory:
            independent = run_small_oracles(base, item, expected, Path(directory))
        local_wrong = None
        if item["source_id"] in LOCAL_WRONG_CASES:
            with tempfile.TemporaryDirectory(dir="/work") as directory:
                local_wrong = run_local_wrong_case(base, item, expected, Path(directory) / "wrong.out")
            print(item["source_id"], "local wrong candidate rejected at formal position",
                  local_wrong["case_position"], flush=True)
        report.append({"source_id": item["source_id"], "cases": len(item["cases"]),
                       "input_validator": "passed-all", "reference_checker": "passed-all",
                       "invalid_output_checker_exit": result.returncode, "input_controls": controls,
                       "wrong_solution": wrong_solution, "slow_solution": slow_solution, "independent": independent,
                       "local_wrong_solution": local_wrong})
        print(item["source_id"], len(item["cases"]), "formal and", len(independent["cases"]),
              "independent cases validated", flush=True)
    export = Path("/work/export")
    export.mkdir()
    for path, entry in expected.items():
        relative = path.removeprefix("source/")
        if path.startswith("source/"):
            original = source / relative
        else:
            _, pid, folder, name = path.split("/")
            base = next(p["path"] for p in catalog if p["source_id"] == "library-checker:" + pid)
            original = source / base / folder / name
        data = read_regular(original.parent, original.name, 128 << 20)
        if digest(data) != entry["sha256"]:
            raise ValueError("export digest differs from pinned source")
        target = export / path
        target.parent.mkdir(parents=True, exist_ok=True)
        # Hard links avoid a second copy of the generated data in bounded tmpfs.
        target.hardlink_to(original)
    validation = {"schema": LOCAL_WRONG_SCHEMA, "commit": lock["commit"],
                  "elapsed_seconds": round(time.monotonic() - start, 3),
                  "compiler": subprocess.check_output(["g++", "--version"], text=True).splitlines()[0],
                  "python": sys.version.split()[0], "problems": report}
    (export / "validation.json").write_text(json.dumps(validation, indent=2) + "\n")


if __name__ == "__main__":
    main()
