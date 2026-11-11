"""Fixed quality controls for the pinned Library Checker sources."""

from problem_source_common import digest


INPUT_CONTROL_SCHEMA = "oj.source-validation.v2"
WRONG_SOLUTION_SCHEMA = "oj.source-validation.v3"

# Only the locked, stand-alone programs that upstream declares expect=WA.
WRONG_SOLUTIONS = {
    "sample/aplusb": "wa.cpp",
    "data_structure/static_range_sum": "wa.cpp",
    "number_theory/primality_test": "insufficient_random_bases.cpp",
}

# Each pair changes one field at a documented input boundary. Keep these bytes
# fixed for source-validation.v2 so reports can be checked and reproduced.
_BOUNDARIES = {
    "aplusb": (b"0 1000000000\n", b"0 1000000001\n"),
    "unionfind": (b"1 1\n1 0 0\n", b"1 1\n1 0 1\n"),
    "associative_array": (b"1\n1 0\n", b"1\n1 -1\n"),
    "static_range_sum": (b"1 1\n0\n0 1\n", b"1 1\n0\n0 0\n"),
    "point_add_range_sum": (b"1 1\n0\n1 0 1\n", b"1 1\n0\n1 0 0\n"),
    "staticrmq": (b"1 1\n0\n0 1\n", b"1 1\n0\n0 0\n"),
    "zalgorithm": (b"abc\n", b"Abc\n"),
    "suffixarray": (b"abc\n", b"Abc\n"),
    "primality_test": (b"1\n1\n", b"1\n0\n"),
    "convolution_mod": (b"1 1\n0\n998244352\n", b"1 1\n0\n998244353\n"),
}


def input_controls(source_id):
    prefix = "library-checker:"
    if not source_id.startswith(prefix) or source_id.removeprefix(prefix) not in _BOUNDARIES:
        raise ValueError("missing pinned input validator controls")
    valid, outside = _BOUNDARIES[source_id.removeprefix(prefix)]
    # Testlib maps validator input failures to FAIL (3). Other process
    # outcomes, including signals, must not count as expected rejections.
    return (
        ("valid-boundary", valid, 0),
        ("outside-domain", outside, 3),
        ("truncated", valid[:-1], 3),
        ("trailing-data", valid + b"0\n", 3),
    )


def control_result(name, payload, exit_code):
    return {"id": name, "input_sha256": digest(payload), "exit_code": exit_code}


def wrong_solution_result(problem, position, inventory):
    """Bind an observed WA to one formal point in the original case order."""
    path = problem["path"]
    solution = WRONG_SOLUTIONS[path]
    if type(position) is not int or not 1 <= position <= len(problem["cases"]):
        raise ValueError("invalid wrong-solution case position")
    case = problem["cases"][position - 1]
    if case["sample"]:
        raise ValueError("wrong-solution witness must be a formal point")
    name = case["id"]
    data = f"problems/{path.rsplit('/', 1)[-1]}"
    return {
        "solution": solution,
        "solution_sha256": inventory[f"source/{path}/sol/{solution}"]["sha256"],
        "checker_sha256": inventory[f"source/{path}/checker.cpp"]["sha256"],
        "case_id": name,
        "case_position": position,
        "input_sha256": inventory[f"{data}/in/{name}.in"]["sha256"],
        "answer_sha256": inventory[f"{data}/out/{name}.out"]["sha256"],
        "candidate_exit": 0,
        "checker_exit": 1,
    }
