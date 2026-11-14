"""Validate the independent database observation used for P6 reconciliation."""

from datetime import datetime, timezone
import re
from typing import NamedTuple

from p6_evidence import exact, label


UTC_TIME = re.compile(r"([0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2})(?:\.([0-9]{1,9}))?Z")
EPOCH = datetime(1970, 1, 1, tzinfo=timezone.utc)


class Receipt(NamedTuple):
    version: str
    offset_ms: int


def utc_ns(value):
    match = UTC_TIME.fullmatch(value) if isinstance(value, str) else None
    if not match:
        raise ValueError("invalid observation timestamp")
    seconds = datetime.fromisoformat(match[1]).replace(tzinfo=timezone.utc) - EPOCH
    return (seconds.days * 86_400 + seconds.seconds) * 1_000_000_000 + int((match[2] or "").ljust(9, "0"))


def acceptance_rows(observation, start_ms, end_ms):
    exact(observation, "schema from until observed_at accepted")
    start, end = start_ms * 1_000_000, end_ms * 1_000_000
    if (not 0 < end_ms - start_ms <= 3_600_000
            or observation["schema"] != "oj.p6-training-acceptance.v1"
            or utc_ns(observation["from"]) != start or utc_ns(observation["until"]) != end
            or utc_ns(observation["observed_at"]) < end):
        raise ValueError("observation does not cover the exact closed round window")
    rows = observation["accepted"]
    if not isinstance(rows, list) or len(rows) > 20_000:
        raise ValueError("unsupported acceptance row count")
    by_id = {}
    for row in rows:
        exact(row, "id accepted_at material_version")
        ident, version = label(row["id"]), label(row["material_version"])
        at = utc_ns(row["accepted_at"])
        if ident in by_id or not start <= at < end:
            raise ValueError("duplicate or out-of-window acceptance")
        by_id[ident] = Receipt(version, (at - start) // 1_000_000)
    return by_id


def independent_round(observations, index, start, end, seen):
    if not observations:
        return None
    raw, digest = observations[index]
    database = acceptance_rows(raw, start, end)
    if not seen.isdisjoint(database):
        raise ValueError("receipt appears in more than one acceptance window")
    seen.update(database)
    return database, digest
