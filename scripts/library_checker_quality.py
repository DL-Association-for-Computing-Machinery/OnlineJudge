"""Fixed input validator controls for the pinned Library Checker sources."""

from problem_source_common import digest


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
