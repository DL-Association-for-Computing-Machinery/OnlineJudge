#!/usr/bin/env python3
"""Small, dependency-free repository gates; never prints file contents."""

import os
from pathlib import Path, PurePosixPath
import re
import subprocess
import sys
from urllib.parse import unquote, urlsplit


MAX_BYTES = 1024 * 1024
PROTECTED = {"refs/heads/main", "refs/heads/master"}
TYPES = "feat|fix|docs|refactor|test|chore|style|perf|build|ci|revert"
TITLE = re.compile(rf"(?:{TYPES})(?:\([^()\s]+\))?!?: \S.*")
PRIVATE_KEY = re.compile(rb"-----BEGIN (?:[A-Z0-9]+ )*PRIVATE KEY-----")
CONFLICT = re.compile(rb"^(?:<{7} |>{7} |\|{7} )", re.MULTILINE)
LINK = re.compile(r"!?\[[^\]\n]*\]\((<[^>\n]+>|[^\s)]+)(?:\s+\"[^\"]*\")?\)")


class PolicyError(Exception):
    """An actionable failure without disclosing source payloads."""


def git(*args, input_bytes=None):
    result = subprocess.run(
        ["git", *args], input=input_bytes, stdout=subprocess.PIPE,
        stderr=subprocess.PIPE, check=False,
    )
    if result.returncode:
        raise PolicyError("Git operation failed: " + " ".join(args[:2]))
    return result.stdout


def names(raw):
    return {os.fsdecode(name) for name in raw.split(b"\0") if name}


def prohibited(path):
    parts = PurePosixPath(path).parts
    name = parts[-1].lower()
    directories = {"node_modules", "dist", "__pycache__", ".local", "coverage"}
    return (
        bool(set(parts) & directories)
        or (name == ".env" or name.startswith(".env.")) and name != ".env.example"
        or name.endswith(".local") or ".local." in name
        or name.endswith((".pem", ".key", ".p12", ".pfx", ".crt", ".cer"))
        or name in {"id_rsa", "id_ed25519", ".ds_store"}
    )


def link_errors(path, content, available):
    # This intentionally checks inline Markdown file links, not URLs or anchors.
    errors = []
    for match in LINK.finditer(content):
        target = match.group(1).strip("<>")
        parsed = urlsplit(target)
        if parsed.scheme or parsed.netloc or not parsed.path or parsed.path.startswith("/"):
            continue
        destination = os.path.normpath(str(PurePosixPath(path).parent / unquote(parsed.path)))
        if destination == ".." or destination.startswith("../"):
            errors.append(f"{path}: relative link escapes repository")
        elif destination not in available and not any(
            p.startswith(destination.rstrip("/") + "/") for p in available
        ):
            errors.append(f"{path}: missing local link target {destination}")
    return errors


def inspect_files(paths, available, reader):
    errors = []
    for path in sorted(paths):
        if prohibited(path):
            errors.append(f"{path}: prohibited secret/configuration/artifact filename")
            continue
        try:
            data = reader(path)
        except PolicyError as error:
            errors.append(str(error))
            continue
        if len(data) > MAX_BYTES:
            errors.append(f"{path}: exceeds 1 MiB; store large assets outside source Git")
            continue
        if PRIVATE_KEY.search(data):
            errors.append(f"{path}: private key marker detected")
        if CONFLICT.search(data):
            errors.append(f"{path}: unresolved merge marker")
        if path.endswith(".md"):
            try:
                errors.extend(link_errors(path, data.decode("utf-8"), available))
            except UnicodeDecodeError:
                errors.append(f"{path}: Markdown is not UTF-8")
    if errors:
        raise PolicyError("\n".join(errors))


def worktree_files():
    candidates = names(git("ls-files", "-z", "--cached", "--others", "--exclude-standard"))
    return {name for name in candidates if os.path.lexists(name)}


def read_worktree(path):
    local = Path(path)
    if any(parent.is_symlink() for parent in [local, *local.parents]):
        raise PolicyError(f"{path}: symbolic links require an explicitly reviewed policy")
    if not local.is_file():
        raise PolicyError(f"{path}: expected a regular file")
    with local.open("rb") as stream:
        return stream.read(MAX_BYTES + 1)


def repository():
    paths = worktree_files()
    inspect_files(paths, paths, read_worktree)
    print(f"Repository policy passed ({len(paths)} files; no application validation).")


def staged():
    branch = git("symbolic-ref", "--quiet", "HEAD").decode().strip()
    if branch in PROTECTED:
        raise PolicyError("Commits to main/master are blocked; use an authorized task branch.")
    entries = {}
    for record in git("ls-files", "--stage", "-z").split(b"\0"):
        if record:
            metadata, filename = record.split(b"\t", 1)
            mode, oid, stage = metadata.decode().split()
            if stage != "0":
                raise PolicyError("Resolve index conflicts before committing.")
            entries[os.fsdecode(filename)] = (mode, oid)

    def reader(path):
        mode, oid = entries[path]
        if mode not in {"100644", "100755"}:
            raise PolicyError(f"{path}: only regular files are supported by this gate")
        size = int(git("cat-file", "-s", oid))
        if size > MAX_BYTES:
            return b" " * (MAX_BYTES + 1)
        return git("cat-file", "blob", oid)

    # Check the complete proposed snapshot, including link targets and deletions.
    inspect_files(set(entries), set(entries), reader)
    result = subprocess.run(["git", "diff", "--cached", "--check"], capture_output=True)
    if result.returncode:
        raise PolicyError("Staged whitespace errors; inspect git diff --cached --check locally.")
    print("Staged snapshot policy passed.")


def commit_message(filename):
    lines = Path(filename).read_text(encoding="utf-8").splitlines()
    title = lines[0] if lines else ""
    if len(title) > 100 or not TITLE.fullmatch(title):
        raise PolicyError("Use a Conventional Commit title, at most 100 characters.")


def pre_push(stream):
    for line in stream:
        fields = line.split()
        if len(fields) != 4:
            raise PolicyError("Invalid pre-push input; refusing push.")
        if fields[2] in PROTECTED:
            raise PolicyError("Direct update/deletion of remote main/master is blocked; use PR review.")


def bootstrap_ci():
    manifests = {"go.mod", "package.json", "Cargo.toml", "pyproject.toml", "requirements.txt", "pom.xml"}
    code_suffixes = {".go", ".ts", ".tsx", ".js", ".jsx", ".vue", ".svelte", ".rs", ".java", ".cpp", ".c"}
    for path in sorted(worktree_files()):
        item = PurePosixPath(path)
        if item.name in manifests or item.suffix in code_suffixes or (
            item.suffix == ".py" and not path.startswith(("scripts/", "tests/governance/"))
        ):
            raise PolicyError(
                f"{path}: application detected. Add real application CI and replace this bootstrap gate together."
            )
    print("Documentation/governance bootstrap only; application CI is not configured.")


def main(argv):
    if len(argv) < 2:
        raise PolicyError("Expected repository, staged, commit-message, pre-push or bootstrap-ci.")
    command = argv[1]
    if command == "commit-message" and len(argv) == 3:
        commit_message(argv[2])
        return
    if command == "pre-push":
        pre_push(sys.stdin)
        return
    os.chdir(os.fsdecode(git("rev-parse", "--show-toplevel")).strip())
    actions = {"repository": repository, "staged": staged, "bootstrap-ci": bootstrap_ci}
    if command not in actions:
        raise PolicyError("Unknown repository policy command.")
    actions[command]()


if __name__ == "__main__":
    try:
        main(sys.argv)
    except (PolicyError, OSError, UnicodeError, ValueError) as error:
        print(f"Repository policy: {error}", file=sys.stderr)
        sys.exit(1)
