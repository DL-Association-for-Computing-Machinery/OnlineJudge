"""Named input observations for the reviewed Library Checker source lock."""

from math import isqrt


BILLION = 10**9
QUINTILLION = 10**18
LARGE = 500_000
MOD_MINUS_ONE = 998244352
RANGE_PROBLEMS = {"static_range_sum", "point_add_range_sum", "staticrmq"}


BOUNDS = {
    "aplusb": {"a": (0, BILLION), "b": (0, BILLION), "sum": (0, 2 * BILLION)},
    "unionfind": {"n": (1, 200_000), "q": (1, 200_000)},
    "associative_array": {"q": (1, 1_000_000), "key": (0, QUINTILLION), "value": (0, QUINTILLION)},
    "static_range_sum": {"n": (1, LARGE), "q": (1, LARGE), "value": (0, BILLION)},
    "point_add_range_sum": {"n": (1, LARGE), "q": (1, LARGE), "value": (0, BILLION),
                            "increment": (0, BILLION)},
    "staticrmq": {"n": (1, LARGE), "q": (1, LARGE), "value": (0, BILLION)},
    "primality_test": {"q": (1, 100_000), "value": (1, QUINTILLION)},
    "suffixarray": {"n": (1, LARGE)},
    "zalgorithm": {"n": (1, LARGE)},
    "convolution_mod": {"n": (1, 2**19), "m": (1, 2**19),
                        "left_value": (0, MOD_MINUS_ONE), "right_value": (0, MOD_MINUS_ONE)},
}
BOUNDARIES = {source: {f"{name}={v}" for name, values in fields.items() for v in values}
              for source, fields in BOUNDS.items()}

INTERVALS = {"whole_range", "one_element_range", "starts_at_zero", "ends_at_n"}
STRING_SHAPES = {"all_equal_n_gt_1", "all_26_letters", "two_equal_halves_nonconstant", "alternating"}
STRUCTURES = {
    "aplusb": {"unequal_operands", "zero_with_nonzero"},
    "unionfind": {"query_and_union", "query_before_union", "self_union", "self_query", "vertex_zero",
                  "last_vertex"},
    "associative_array": {"lookup_existing", "lookup_missing", "overwrite"},
    "static_range_sum": INTERVALS,
    "point_add_range_sum": INTERVALS | {"query_after_update", "update_first", "update_last"},
    "staticrmq": INTERVALS,
    "primality_test": {"even_gt_2", "square_gt_1", "value_is_2"},
    "suffixarray": STRING_SHAPES,
    "zalgorithm": STRING_SHAPES,
    "convolution_mod": {"both_singleton", "both_max_length", "unequal_lengths", "one_singleton",
                        "left_all_zero", "right_all_zero", "left_one_nonzero", "right_one_nonzero"},
}


def numbers(line):
    return list(map(int, line.split()))


def observe(source, raw):
    """Return only criteria witnessed in this input; malformed shapes raise."""
    marks = set()

    def add(name, present):
        if present:
            marks.add(name)

    def limits(name, low, high):
        lower, upper = BOUNDS[source][name]
        add(f"{name}={lower}", low == lower)
        add(f"{name}={upper}", high == upper)

    def scalar(name, value):
        limits(name, value, value)

    if source in ("suffixarray", "zalgorithm"):
        word, = raw.splitlines()
        size, alphabet = len(word), len(set(word))
        scalar("n", size)
        add("all_equal_n_gt_1", size > 1 and alphabet == 1)
        add("all_26_letters", alphabet == 26)
        if size >= 4 and size % 2 == 0 and alphabet > 1:
            add("two_equal_halves_nonconstant", word[:size // 2] == word[size // 2:])
            add("alternating", alphabet == 2 and word == word[:2] * (size // 2))
    elif source == "aplusb":
        a, b = numbers(raw)
        scalar("a", a)
        scalar("b", b)
        scalar("sum", a + b)
        add("unequal_operands", a != b)
        add("zero_with_nonzero", min(a, b) == 0 < max(a, b))
    elif source == "primality_test":
        header, *queries = raw.splitlines()
        q, = numbers(header)
        values = [int(line) for line in queries]
        if len(values) != q or not values:
            raise ValueError("primality query count differs")
        scalar("q", q)
        limits("value", min(values), max(values))
        add("value_is_2", 2 in values)
        add("even_gt_2", any(x > 2 and x % 2 == 0 for x in values))
        add("square_gt_1", any(x > 1 and isqrt(x)**2 == x for x in values))
    elif source == "convolution_mod":
        header, left, right = raw.splitlines()
        n, m = numbers(header)
        scalar("n", n)
        scalar("m", m)
        add("both_singleton", n == m == 1)
        add("both_max_length", n == m == 2**19)
        add("unequal_lengths", n != m)
        add("one_singleton", min(n, m) == 1 < max(n, m))
        for label, line, count in (("left", left, n), ("right", right, m)):
            values = numbers(line)
            if len(values) != count or not values:
                raise ValueError("convolution vector length differs")
            largest = max(values)
            limits(f"{label}_value", min(values), largest)
            add(f"{label}_all_zero", largest == 0)
            add(f"{label}_one_nonzero", count > 1 and sum(v != 0 for v in values) == 1)
    else:
        observe_queries(source, raw, marks, add, limits, scalar)
    return marks


def observe_queries(source, raw, marks, add, limits, scalar):
    lines = iter(raw.splitlines())
    if source == "associative_array":
        q, = numbers(next(lines, b""))
        n = None
    else:
        n, q = numbers(next(lines, b""))
        scalar("n", n)
    scalar("q", q)
    if source in RANGE_PROBLEMS:
        values = numbers(next(lines, b""))
        if len(values) != n or not values:
            raise ValueError("initial array length differs")
        limits("value", min(values), max(values))
    if source == "associative_array":
        keys = set()
        for line in take_rows(lines, q):
            op, key, *tail = numbers(line)
            scalar("key", key)
            if op == 0:
                value, = tail
                scalar("value", value)
                add("overwrite", key in keys)
                keys.add(key)
            elif op == 1 and not tail:
                marks.add("lookup_existing" if key in keys else "lookup_missing")
            else:
                raise ValueError("unknown associative array operation")
    elif source == "unionfind":
        union = query = False
        for line in take_rows(lines, q):
            op, u, v = numbers(line)
            if op == 0:
                union = True
                add("self_union", u == v)
            elif op == 1:
                query = True
                add("query_before_union", not union)
                add("self_query", u == v)
            else:
                raise ValueError("unknown union-find operation")
            add("vertex_zero", 0 in (u, v))
            add("last_vertex", n - 1 in (u, v))
        add("query_and_union", query and union)
    else:
        updated = False
        for line in take_rows(lines, q):
            if source == "point_add_range_sum":
                op, left, right = numbers(line)
                if op == 0:
                    updated = True
                    add("update_first", left == 0)
                    add("update_last", left == n - 1)
                    scalar("increment", right)
                    continue
                if op != 1:
                    raise ValueError("unknown point-add operation")
                add("query_after_update", updated)
            else:
                left, right = numbers(line)
            add("whole_range", left == 0 and right == n)
            add("one_element_range", right == left + 1)
            add("starts_at_zero", left == 0)
            add("ends_at_n", right == n)


def take_rows(lines, count):
    for _ in range(count):
        line = next(lines, None)
        if line is None:
            raise ValueError("missing query rows")
        yield line
    if next(lines, None) is not None:
        raise ValueError("unexpected query rows")
