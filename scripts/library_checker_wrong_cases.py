"""Fixed, locally authored wrong candidates for the remaining pinned sources."""

import sys


LOCAL_WRONG_SCHEMA = "oj.source-validation.v6"
WRONG_CASE_SUITE = "lc-formal-wrong.v1"
MAX_INPUT_BYTES = 12 << 20
MAX_OUTPUT_BYTES = 4 << 20
WALL_SECONDS = 15

# Positions use the original numbering, including samples. Keep the explicit
# probe order, including smaller probes that do not distinguish the candidate.
LOCAL_WRONG_CASES = {
    "library-checker:unionfind": ((9, 10), "direct-edges-only"),
    "library-checker:associative_array": ((17,), "first-write-wins"),
    "library-checker:point_add_range_sum": ((4,), "ignore-updates"),
    "library-checker:staticrmq": ((8, 9, 4), "right-endpoint-only"),
    "library-checker:zalgorithm": ((5,), "zero-at-origin"),
    "library-checker:suffixarray": ((32,), "leading-character-only"),
    "library-checker:convolution_mod": ((8,), "omit-last-coefficient"),
}


def rows(data):
    return (tuple(map(int, line.split())) for line in data.splitlines())


def direct_edges_only(data):
    stream = rows(data)
    next(stream)
    edges = set()
    for command, a, b in stream:
        edge = (min(a, b), max(a, b))
        if command == 0:
            edges.add(edge)
        else:
            yield int(a == b or edge in edges)


def first_write_wins(data):
    stream = rows(data)
    next(stream)
    values = {}
    for command, key, *value in stream:
        if command == 0:
            values.setdefault(key, value[0])
        else:
            yield values.get(key, 0)


def ignore_updates(data):
    stream = rows(data)
    next(stream)
    prefix = [0]
    for value in next(stream):
        prefix.append(prefix[-1] + value)
    for command, a, b in stream:
        if command == 1:
            yield prefix[b] - prefix[a]


def right_endpoint_only(data):
    stream = rows(data)
    next(stream)
    values = next(stream)
    for _, right in stream:
        yield values[right - 1]


def zero_at_origin(data):
    word = data.strip()
    if not 1 <= len(word) <= 128:
        raise ValueError("Z probe exceeds the fixed small-string budget")
    yield 0
    for i in range(1, len(word)):
        count = 0
        while i + count < len(word) and word[count] == word[i + count]:
            count += 1
        yield count


def leading_character_only(data):
    word = data.strip()
    if not 1 <= len(word) <= 128:
        raise ValueError("suffix probe exceeds the fixed small-string budget")
    yield from sorted(range(len(word)), key=word.__getitem__)


def omit_last_coefficient(data):
    stream = rows(data)
    n, m = next(stream)
    a, b = next(stream), next(stream)
    if not 1 <= n <= 32 or not 1 <= m <= 32 or len(a) != n or len(b) != m:
        raise ValueError("convolution probe exceeds the fixed small-array budget")
    result = [0] * (n + m - 1)
    for i, x in enumerate(a):
        for j, y in enumerate(b[:-1]):
            result[i + j] = (result[i + j] + x * y) % 998244353
    yield from result


_CANDIDATES = {
    "direct-edges-only": direct_edges_only,
    "first-write-wins": first_write_wins,
    "ignore-updates": ignore_updates,
    "right-endpoint-only": right_endpoint_only,
    "zero-at-origin": zero_at_origin,
    "leading-character-only": leading_character_only,
    "omit-last-coefficient": omit_last_coefficient,
}


def wrong_output(source_id, data):
    if not 0 < len(data) <= MAX_INPUT_BYTES:
        raise ValueError("wrong-candidate input exceeds the fixed budget")
    _, candidate = LOCAL_WRONG_CASES[source_id]
    separator = " " if candidate in ("zero-at-origin", "leading-character-only", "omit-last-coefficient") else "\n"
    return (separator.join(map(str, _CANDIDATES[candidate](data))) + "\n").encode("ascii")


def fixed_cases(item):
    positions, _ = LOCAL_WRONG_CASES[item["source_id"]]
    cases = []
    for position in positions:
        if not 1 <= position <= len(item["cases"]) or item["cases"][position - 1]["sample"]:
            raise ValueError("fixed wrong-candidate probe must be a formal point")
        cases.append((position, item["cases"][position - 1]["id"]))
    return cases


def local_wrong_result(item, inventory, witness):
    cases = fixed_cases(item)
    if type(witness) is not int or witness not in dict(cases):
        raise ValueError("invalid fixed wrong-candidate witness position")
    pid = item["source_id"].removeprefix("library-checker:")
    attempts = []
    for position, name in cases:
        row = {
            "case_id": name,
            "case_position": position,
            "input_sha256": inventory[f"problems/{pid}/in/{name}.in"]["sha256"],
            "answer_sha256": inventory[f"problems/{pid}/out/{name}.out"]["sha256"],
            "candidate_exit": 0,
            "checker_exit": int(position == witness),
        }
        if position == witness:
            return {"suite": WRONG_CASE_SUITE, "candidate": LOCAL_WRONG_CASES[item["source_id"]][1],
                    "checker_sha256": inventory[f"source/{item['path']}/checker.cpp"]["sha256"],
                    "preceding_ac": attempts, **row}
        attempts.append(row)


if __name__ == "__main__":
    sys.stdout.buffer.write(wrong_output(sys.argv[1], sys.stdin.buffer.read(MAX_INPUT_BYTES + 1)))
