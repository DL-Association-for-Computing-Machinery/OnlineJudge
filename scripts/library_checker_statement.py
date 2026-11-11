"""Convert the fixed source's small template vocabulary without executing it."""

import re


KEYWORDS = {"statement": "Problem Statement", "constraints": "Constraints",
            "input": "Input", "output": "Output", "sample": "Examples"}
DIRECTIVE = re.compile(r"@\{([^{}\n]+)\}")
FORMAT_SECTIONS = {
    "library-checker:aplusb": ("input", "output"),
    "library-checker:associative_array": ("input",),
    "library-checker:convolution_mod": ("input", "output"),
    "library-checker:primality_test": ("input",),
    "library-checker:static_range_sum": ("input",),
    "library-checker:suffixarray": ("input", "output"),
}
_FENCE = re.compile(r" {0,3}(`{3,}|~{3,}).*")
_FENCE_END = re.compile(r" {0,3}(`{3,}|~{3,})[ \t]*")
_FORMAT_TOKEN = re.compile(r"\$((?:[A-Za-z0-9_{}()+\- ]|\\(?:ldots|vdots|textrm)(?![A-Za-z]))+)\$|\.\.\.")


def _format_math(rows):
    if not 1 <= len(rows) <= 16 or sum(len(row.encode()) for row in rows) > 4096:
        raise ValueError("input/output format exceeds its budget")
    converted = []
    for raw in rows:
        row = raw.strip(" \t")
        if row == ":":
            converted.append(row)
            continue
        tokens = []
        end = 0
        for match in _FORMAT_TOKEN.finditer(row):
            gap = row[end:match.start()]
            if gap.strip(" \t") or tokens and not gap:
                raise ValueError("unsupported input/output format token")
            token = match[1] if match[1] is not None else r"\ldots"
            depth = 0
            for char in token:
                depth += (char == "{") - (char == "}")
                if depth < 0:
                    raise ValueError("unbalanced input/output formula")
            if depth or not token.strip(" "):
                raise ValueError("invalid input/output formula")
            tokens.append(token)
            end = match.end()
        if not tokens or row[end:].strip(" \t"):
            raise ValueError("unsupported input/output format row")
        converted.append(r"\quad ".join(tokens))
    formula = r"\begin{gathered}" + r" \\ ".join(converted) + r"\end{gathered}"
    if len(formula.encode()) > 4096:
        raise ValueError("input/output formula exceeds its budget")
    return "```math\n" + formula + "\n```"


def _convert_formats(lines, sections):
    # Only the first bare fence immediately under each designated source
    # heading is a format block. Samples have not been expanded at this point.
    headings = {"## @{keyword." + name + "}" for name in sections}
    pending = set(headings)
    converted, active = [], None
    i = 0
    while i < len(lines):
        line = lines[i].removesuffix("\n").removesuffix("\r")
        if active:
            closer = _FENCE_END.fullmatch(line)
            if closer and closer[1].startswith(active):
                active = None
        elif line in headings:
            if line not in pending:
                raise ValueError("duplicate pinned input/output format heading")
            pending.remove(line)
            converted.append(lines[i])
            i += 1
            while i < len(lines) and not lines[i].strip():
                converted.append(lines[i])
                i += 1
            if i >= len(lines) or lines[i].rstrip("\r\n") != "```":
                raise ValueError("missing pinned input/output format fence")
            start = i + 1
            i = start
            while i < len(lines) and lines[i].rstrip("\r\n") != "```":
                i += 1
                if i - start > 16:
                    raise ValueError("input/output format exceeds its row budget")
            if i == len(lines):
                raise ValueError("unclosed pinned input/output format fence")
            converted.append(_format_math([row.rstrip("\r\n") for row in lines[start:i]]))
            if lines[i].endswith("\n"):
                converted.append("\n")
            i += 1
            continue
        else:
            opener = _FENCE.fullmatch(line)
            if opener:
                active = opener[1]
        converted.append(lines[i])
        i += 1
    if pending:
        raise ValueError("missing pinned input/output format heading")
    return converted


def convert(template, params, samples, *, source_id=None):
    """Return English Markdown and ordered sample IDs; unknown syntax fails closed.

    Samples contain only the source's explicit example group. Their original
    bytes remain separate; rendered code blocks add a delimiter newline if
    needed. Rendering/TeX compatibility is a later validation gate.
    """
    if len(template.encode()) > 1 << 20 or "\x00" in template:
        raise ValueError("invalid statement size or encoding")
    selected = []
    language = None
    for line in template.splitlines(keepends=True):
        marker = line.strip()
        if "@{lang." in line and marker not in {"@{lang.en}", "@{lang.ja}", "@{lang.end}"}:
            raise ValueError("language markers must occupy their own line")
        if marker in {"@{lang.en}", "@{lang.ja}", "@{lang.end}"}:
            if marker == "@{lang.en}":
                if language is not None:
                    raise ValueError("nested statement language block")
                language = "en"
            elif marker == "@{lang.ja}":
                if language != "en":
                    raise ValueError("unexpected statement language alternative")
                language = "ja"
            else:
                if language != "ja":
                    raise ValueError("unbalanced statement language block")
                language = None
        elif language != "ja":
            selected.append(line)
    if language is not None:
        raise ValueError("unterminated statement language block")
    examples = []
    def replace(match):
        kind, separator, name = match.group(1).partition(".")
        if not separator:
            raise ValueError("unknown statement directive")
        if kind == "keyword" and name in KEYWORDS:
            return KEYWORDS[name]
        if kind == "param" and name in params and type(params[name]) is int:
            return str(params[name])
        if kind == "example" and name in samples and name not in examples:
            examples.append(name)
            sections = ["### Example " + str(len(examples))]
            for label, text in zip(("Input", "Output"), samples[name]):
                if len(text.encode()) > 64 << 10 or "\x00" in text:
                    raise ValueError("sample text exceeds statement budget")
                # A source example cannot terminate its own Markdown fence.
                longest = max((len(m.group()) for m in re.finditer(r"`+", text)), default=0)
                fence = "`" * max(3, longest + 1)
                sections.append("#### " + label + "\n\n" + fence + "text\n" + text
                                + ("" if text.endswith("\n") else "\n") + fence)
            return "\n\n".join(sections)
        raise ValueError("unsupported or duplicate statement directive")
    # Validate even discarded translations: a new directive is a source-format
    # change, rather than permission to silently drop unsupported semantics.
    if "@{" in DIRECTIVE.sub("", template):
        raise ValueError("malformed statement directive")
    for directive in DIRECTIVE.findall(template):
        kind, _, name = directive.partition(".")
        if ((kind == "lang" and name in {"en", "ja", "end"})
                or (kind == "keyword" and name in KEYWORDS)
                or (kind == "param" and name in params and type(params[name]) is int)
                or (kind == "example" and name in samples)):
            continue
        raise ValueError("unknown statement directive")
    if sections := FORMAT_SECTIONS.get(source_id):
        selected = _convert_formats(selected, sections)
    markdown = DIRECTIVE.sub(replace, "".join(selected))
    if "@{" in markdown or set(examples) != set(samples):
        raise ValueError("unresolved statement syntax or missing sample")
    return markdown, examples
