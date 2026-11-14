"""Common local record validation for the P6 acceptance tools."""

import hashlib
import json
import os
import re
import stat


MAX_RECORD = 24 << 20
IDENT = re.compile(r"[a-zA-Z0-9_.:-]{1,80}")
SHA = re.compile(r"[0-9a-f]{64}")


def unique(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate field")
        result[key] = value
    return result


def read(path):
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(fd, "rb") as stream:
        if not stat.S_ISREG(os.fstat(fd).st_mode):
            raise ValueError("expected regular record")
        raw = stream.read(MAX_RECORD + 1)
    if len(raw) > MAX_RECORD:
        raise ValueError("oversized record")
    return json.loads(raw, object_pairs_hook=unique), hashlib.sha256(raw).hexdigest()


def exact(obj, fields):
    if not isinstance(obj, dict) or set(obj) != set(fields.split()):
        raise ValueError("unexpected record fields")


def number(value, low, high):
    if type(value) is not int or not low <= value <= high:
        raise ValueError("number outside supported range")
    return value


def label(value, pattern=IDENT):
    if not isinstance(value, str) or not pattern.fullmatch(value):
        raise ValueError("invalid identifier")
    return value


def timings(values, attempts):
    values = sorted(values)
    # Failed or missing observations rank after finite observations.
    result = {"attempts": attempts, "samples": len(values)}
    for p in (50, 95, 99):
        index = (attempts * p + 99) // 100 - 1
        result[f"p{p}_ms"] = values[index] if 0 <= index < len(values) else None
    return result
