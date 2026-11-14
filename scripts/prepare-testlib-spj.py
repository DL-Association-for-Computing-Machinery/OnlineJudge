#!/usr/bin/env python3
"""Prepare the pinned Testlib header for an offline SPJ protocol probe."""

import argparse
import hashlib
from pathlib import Path
import re

UPSTREAM_COMMIT = "2d20123984e9479b8a56ebe0d6a51e23ad7c35b3"
UPSTREAM_BLOB = "349b839ff14e3b933668e9938dbbe04ecb265843"
UPSTREAM_BYTES = 206468

# Keep nonzero protocol exits separate from ordinary process return 0 and
# ICPC 42/43. All unsupported scores are rejected before trailing-output
# handling can turn them into contestant errors.
FLAGS = (
    "-DENABLE_UNEXPECTED_EOF",
    "-DOK_EXIT_CODE=50",
    "-DWA_EXIT_CODE=51",
    "-DPE_EXIT_CODE=52",
    "-DDIRT_EXIT_CODE=53",
    "-DUNEXPECTED_EOF_EXIT_CODE=54",
    "-DFAIL_EXIT_CODE=55",
    "-DPOINTS_EXIT_CODE=55",
)


def replace_once(data, old, new):
    if data.count(old) != 1:
        raise ValueError("pinned Testlib source no longer matches the adaptation")
    return data.replace(old, new, 1)


def prepare(raw, expected_sha256):
    if not re.fullmatch(r"[a-f0-9]{64}", expected_sha256) or hashlib.sha256(raw).hexdigest() != expected_sha256:
        raise ValueError("Testlib SHA-256 mismatch")
    blob = b"blob " + str(len(raw)).encode() + b"\0" + raw
    if len(raw) != UPSTREAM_BYTES or hashlib.sha1(blob, usedforsecurity=False).hexdigest() != UPSTREAM_BLOB:
        raise ValueError("Testlib does not match the pinned upstream Git blob")
    raw = replace_once(
        raw, b"return PC_BASE_EXIT_CODE + (r - _partially);", b"return FAIL_EXIT_CODE;",
    )
    start = b"void InStream::quit(TResult result, const char *msg) {\n"
    raw = replace_once(
        raw, start, start + b"    if (result == _points || result >= _partially)\n        result = _fail;\n",
    )
    copyright_end = b' */\n#define VERSION "0.9.45"'
    return replace_once(
        raw, copyright_end,
        b' * Modified by OnlineJudge: disable scoring for the binary checker protocol.\n' + copyright_end,
    )


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("source", type=Path)
    parser.add_argument("expected_sha256", help=f"trusted SHA-256 for Testlib at {UPSTREAM_COMMIT}")
    parser.add_argument("output", type=Path, help="new testlib.h in a private build directory")
    args = parser.parse_args()
    with args.source.open("rb") as source:
        raw = source.read(UPSTREAM_BYTES + 1)
    try:
        adapted = prepare(raw, args.expected_sha256)
        with args.output.open("xb") as output:
            output.write(adapted)
    except (OSError, ValueError) as exc:
        parser.error(str(exc))
    print("testlib.h sha256:", hashlib.sha256(adapted).hexdigest())
    print("compiler flags:", " ".join(FLAGS))


if __name__ == "__main__":
    main()
