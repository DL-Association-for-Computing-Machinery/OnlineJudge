"""Validate a container's export against host-derived source and data hashes."""

import hashlib
import json
from pathlib import PurePosixPath
import tarfile

from library_checker_quality import (
    INPUT_CONTROL_SCHEMA, SLOW_SOLUTION_SCHEMA, SLOW_SOURCE, WRONG_SOLUTIONS, WRONG_SOLUTION_SCHEMA,
    control_result, input_controls, slow_solution_result, wrong_solution_result,
)
from library_checker_oracles import ORACLE_SCHEMA, oracle_result, small_cases
from problem_source_common import MAX_DATA_FILE, MAX_EXPORT, MAX_REPORT, safe_path


class BoundedArchive:
    """Reject oversized metadata reads before tarfile allocates their payload."""

    def __init__(self, stream):
        self.stream = stream

    def read(self, size):
        if not 0 <= size <= 1 << 20:
            raise ValueError("archive metadata read exceeds budget")
        return self.stream.read(size)

    def seek(self, *args):
        return self.stream.seek(*args)

    def tell(self):
        return self.stream.tell()


def extract(archive, destination, expected):
    """No extractall: reject links, aliases, duplicates, sparse and unknown files."""
    seen = set()
    names_folded = set()
    directories = {str(parent) for name in expected for parent in PurePosixPath(name).parents}
    total = 0
    report = None
    members = set()
    if archive.stat().st_size > MAX_EXPORT + (64 << 20):
        raise ValueError("archive exceeds transfer budget")
    with archive.open("rb") as stream, tarfile.open(fileobj=BoundedArchive(stream), mode="r:") as tar:
        for member in tar:
            # GNU tar's fixed './' prefix is the only allowed non-canonical form.
            name = member.name.removeprefix("./")
            if member.isdir():
                name = name.removesuffix("/")
            if name in members or len(members) > len(expected) + len(directories) + 2:
                raise ValueError("duplicate or excessive archive members")
            members.add(name)
            if member.pax_headers or member.issparse():
                raise ValueError("unsupported archive metadata")
            if member.isdir():
                if name in {"", "."} or name in directories:
                    continue
                raise ValueError("unknown export directory")
            safe_path(name)
            if not member.isfile():
                raise ValueError("export must contain ordinary non-sparse files")
            if name in seen or name.casefold() in names_folded:
                raise ValueError("duplicate export path")
            seen.add(name)
            names_folded.add(name.casefold())
            if name != "validation.json" and name not in expected:
                raise ValueError("unknown export file")
            limit = MAX_REPORT if name == "validation.json" else MAX_DATA_FILE
            if not 0 <= member.size <= limit:
                raise ValueError("export file exceeds budget")
            if name in expected and "size" in expected[name] and member.size != expected[name]["size"]:
                raise ValueError("export source size mismatch")
            total += member.size
            if total > MAX_EXPORT:
                raise ValueError("export total exceeds budget")
            path = destination / name
            path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
            checksum = hashlib.sha256()
            reader = tar.extractfile(member)
            with path.open("xb") as output:
                remaining = member.size
                while remaining:
                    chunk = reader.read(min(1 << 20, remaining))
                    if not chunk:
                        raise ValueError("truncated export")
                    checksum.update(chunk)
                    output.write(chunk)
                    remaining -= len(chunk)
            path.chmod(0o600)
            if name == "validation.json":
                report = json.loads(path.read_text())
            elif checksum.hexdigest() != expected[name]["sha256"]:
                raise ValueError("export differs from pinned hash: " + name)
    if seen != set(expected) | {"validation.json"}:
        raise ValueError("incomplete export")
    return report, total


def validate_report(report, lock, catalog, *, inventory=None, required_schema=None):
    """A source-validation report is evidence, never a production verdict."""
    schema = report.get("schema")
    if (schema not in ("oj.source-validation.v1", INPUT_CONTROL_SCHEMA, WRONG_SOLUTION_SCHEMA, SLOW_SOLUTION_SCHEMA, ORACLE_SCHEMA)
            or required_schema is not None and schema != required_schema
            or report.get("commit") != lock["commit"]):
        raise ValueError("invalid validation report identity")
    actual = report.get("problems")
    if not isinstance(actual, list) or len(actual) != len(catalog):
        raise ValueError("incomplete validation results")
    expected = [{"source_id": p["source_id"], "cases": len(p["cases"]),
                 "input_validator": "passed-all", "reference_checker": "passed-all",
                 "invalid_output_checker_exit": 1} for p in catalog]
    if schema != "oj.source-validation.v1":
        for problem in expected:
            problem["input_controls"] = [control_result(*probe) for probe in input_controls(problem["source_id"])]
    if schema in (WRONG_SOLUTION_SCHEMA, SLOW_SOLUTION_SCHEMA, ORACLE_SCHEMA):
        if inventory is None:
            raise ValueError("missing pinned inventory for wrong-solution results")
        for item, row, observed in zip(catalog, expected, actual, strict=True):
            row["wrong_solution"] = None
            if item["path"] in WRONG_SOLUTIONS:
                result = observed.get("wrong_solution") if isinstance(observed, dict) else None
                if not isinstance(result, dict):
                    raise ValueError("missing wrong-solution witness")
                row["wrong_solution"] = wrong_solution_result(item, result.get("case_position"), inventory)
            if schema in (SLOW_SOLUTION_SCHEMA, ORACLE_SCHEMA):
                row["slow_solution"] = None
                if item["path"] == SLOW_SOURCE:
                    result = observed.get("slow_solution") if isinstance(observed, dict) else None
                    if not isinstance(result, dict):
                        raise ValueError("missing slow-solution witness")
                    row["slow_solution"] = slow_solution_result(item, result.get("case_position"), inventory,
                                                               result.get("candidate_cpu_ms"), result.get("reference_cpu_ms"))
            if schema == ORACLE_SCHEMA:
                row["independent"] = oracle_result(item, inventory, small_cases(item["source_id"]))
    # JSON comparison also distinguishes numbers from booleans and fractions.
    if json.dumps(actual, sort_keys=True) != json.dumps(expected, sort_keys=True):
        raise ValueError("incomplete validation results")
