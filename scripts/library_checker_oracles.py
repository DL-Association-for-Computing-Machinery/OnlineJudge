"""Small, deterministic oracles derived from the pinned problem statements."""

from itertools import combinations, product
from math import isqrt

from problem_source_common import digest


ORACLE_SCHEMA = "oj.source-validation.v5"
ORACLE_SUITE = "lc-small-oracles.v1"
MODULUS = 998244353
WALL_SECONDS = 5
MAX_OUTPUT_BYTES = 64 << 10


def lines(rows):
    return ("\n".join(" ".join(map(str, row)) for row in rows) + "\n").encode("ascii")


def sum_probes():
    values = (0, 1, 1 << 29, 999999999, 1000000000)
    for a, b in product(values, repeat=2):
        yield lines([(a, b)]), lines([(a + b,)])


def connectivity(vertices, queries):
    edges = [set() for _ in range(vertices)]
    answers = []
    for command, start, goal in queries:
        if command == 0:
            edges[start].add(goal)
            edges[goal].add(start)
            continue
        reached = {start}
        queue = [start]
        while queue:
            for neighbor in edges[queue.pop()] - reached:
                reached.add(neighbor)
                queue.append(neighbor)
        answers.append((int(goal in reached),))
    return lines([(vertices, len(queries)), *queries]), lines(answers)


def connectivity_probes():
    edges = tuple(combinations(range(3), 2))
    reads = [(1, a, b) for a, b in product(range(3), repeat=2)]
    for mask in range(1 << len(edges)):
        updates = [(0, a, b) for i, (a, b) in enumerate(edges) if mask & (1 << i)]
        yield connectivity(3, [*reads, *updates, *reads])
    yield connectivity(1, [(1, 0, 0), (0, 0, 0), (1, 0, 0)])
    yield connectivity(5, [(0, 0, 1), (0, 0, 1), (0, 2, 3), (1, 0, 3), (0, 1, 2), (1, 0, 3), (1, 4, 0)])


def lookup_probes():
    maximum = 10**18
    queries = [[(1, 0)]]
    for a, b in ((0, 1), (0, maximum), (1, maximum)):
        queries.append([(1, a), (0, a, maximum), (1, a), (1, b), (0, b, 1), (1, b),
                        (0, a, 0), (1, a), (1, b)])
    for operations in queries:
        history = []
        answers = []
        for command, key, *value in operations:
            if command == 0:
                history.append((key, value[0]))
            else:
                answers.append((next((v for k, v in reversed(history) if k == key), 0),))
        yield lines([(len(operations),), *operations]), lines(answers)


_ARRAYS = (
    (0,), (10**9,), (10**9, 10**9, 10**9, 0), (0, 1, 2, 3, 4), (4, 3, 2, 1, 0),
    (9, 1, 9, 1, 9), (5, 5, 5, 5, 5, 5, 5), (8, 3, 3, 2, 7, 6, 1, 1),
)


def range_probes(mode):
    for array in _ARRAYS:
        ranges = list(combinations(range(len(array) + 1), 2))
        if mode == "updates":
            reads = [(1, a, b) for a, b in ranges]
            queries = [*reads, (0, 0, 10**9), (0, len(array) - 1, 10**9), *reads,
                       (0, 0, 0), (0, 0, 10**9), *reads]
            current, answer = list(array), []
            for command, a, b in queries:
                if command == 0:
                    current[a] += b
                else:
                    answer.append((sum(current[a:b]),))
        else:
            queries = ranges
            aggregate = sum if mode == "sum" else min
            answer = [(aggregate(array[a:b]),) for a, b in ranges]
        yield lines([(len(array), len(queries)), array, *queries]), lines(answer)


def string_probes(mode):
    for length in range(1, 6):
        for chars in product("ab", repeat=length):
            word = "".join(chars)
            if mode == "suffix":
                answer = sorted(range(length), key=lambda i: word[i:])
            else:
                answer = []
                for i in range(length):
                    count = 0
                    while i + count < length and word[count] == word[i + count]:
                        count += 1
                    answer.append(count)
            yield lines([(word,)]), lines([answer])


def prime_probes():
    for batch in ((1,), (2,), tuple(range(1, 257)), (341, 561, 1105, 1729, 2465, 2821, 6601),
                  (99991, 99999, 100000000, 100000007, 100140049, 99999989)):
        answers = [("Yes" if n > 1 and all(n % d for d in range(2, isqrt(n) + 1)) else "No",) for n in batch]
        yield lines([(len(batch),), *((n,) for n in batch)]), lines(answers)


def convolution_probes():
    m = MODULUS
    for a, b in (((0,), (m - 1,)), ((m - 1,), (m - 1,)), ((1, 0, 1), (2, 4)),
                 ((m - 1, 1, m - 1), (1, m - 1, 2, m - 2, 3)),
                 ((1, 2, 3, 4, 5, 6, 7, 8), (m - 1, 0, 1)), ((0, 0, 0), (7, 8, 9, 10))):
        result = [0] * (len(a) + len(b) - 1)
        for i, x in enumerate(a):
            for j, y in enumerate(b):
                result[i + j] = (result[i + j] + x * y) % m
        yield lines([(len(a), len(b)), a, b]), lines([result])


_GENERATORS = {
    "aplusb": sum_probes,
    "unionfind": connectivity_probes,
    "associative_array": lookup_probes,
    "static_range_sum": lambda: range_probes("sum"),
    "point_add_range_sum": lambda: range_probes("updates"),
    "staticrmq": lambda: range_probes("min"),
    "zalgorithm": lambda: string_probes("z"),
    "suffixarray": lambda: string_probes("suffix"),
    "primality_test": prime_probes,
    "convolution_mod": convolution_probes,
}


def small_cases(source_id):
    name = source_id.removeprefix("library-checker:")
    if source_id != f"library-checker:{name}" or name not in _GENERATORS:
        raise ValueError("no fixed small oracle for source")
    return tuple((f"small-{i:02d}", data, answer) for i, (data, answer) in enumerate(_GENERATORS[name](), 1))


def oracle_result(item, inventory, probes):
    base = f"source/{item['path']}"
    return {
        "suite": ORACLE_SUITE,
        "reference_sha256": inventory[f"{base}/sol/correct.cpp"]["sha256"],
        "validator_sha256": inventory[f"{base}/verifier.cpp"]["sha256"],
        "checker_sha256": inventory[f"{base}/checker.cpp"]["sha256"],
        "cases": [{"id": case, "input_sha256": digest(data), "oracle_sha256": digest(answer),
                   "validator_exit": 0, "reference_exit": 0, "checker_exit": 0}
                  for case, data, answer in probes],
    }
