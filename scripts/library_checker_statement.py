"""Convert the fixed source's small template vocabulary without executing it."""

import re


KEYWORDS = {"statement": "Problem Statement", "constraints": "Constraints",
            "input": "Input", "output": "Output", "sample": "Examples"}
DIRECTIVE = re.compile(r"@\{([^{}\n]+)\}")


def convert(template, params, samples):
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
    markdown = DIRECTIVE.sub(replace, "".join(selected))
    if "@{" in markdown or set(examples) != set(samples):
        raise ValueError("unresolved statement syntax or missing sample")
    return markdown, examples
