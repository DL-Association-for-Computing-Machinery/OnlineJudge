#!/usr/bin/env python3
"""Container-only validation of pinned official source; no production judging."""

import importlib.util
import json
import logging
from pathlib import Path
import resource
import subprocess
import sys
import time
import tomllib
import types

from problem_source_common import digest, expected_materials, load_lock, read_regular, snapshot


def checked(command, **kwargs):
    """All compiler, generator, solution and checker children have wall limits."""
    kwargs.setdefault("timeout", 120)
    kwargs.setdefault("stdout", subprocess.DEVNULL)
    kwargs.setdefault("stderr", subprocess.DEVNULL)
    kwargs.setdefault("check", True)
    return subprocess.run(command, **kwargs)


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
        report.append({"source_id": item["source_id"], "cases": len(item["cases"]),
                       "input_validator": "passed-all", "reference_checker": "passed-all",
                       "invalid_output_checker_exit": result.returncode})
        print(item["source_id"], len(item["cases"]), "cases validated", flush=True)
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
    validation = {"schema": "oj.source-validation.v1", "commit": lock["commit"],
                  "elapsed_seconds": round(time.monotonic() - start, 3),
                  "compiler": subprocess.check_output(["g++", "--version"], text=True).splitlines()[0],
                  "python": sys.version.split()[0], "problems": report}
    (export / "validation.json").write_text(json.dumps(validation, indent=2) + "\n")


if __name__ == "__main__":
    main()
