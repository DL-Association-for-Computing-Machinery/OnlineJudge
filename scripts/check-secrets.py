#!/usr/bin/env python3
"""Scan committed HEAD history offline without mounting a checkout or credentials."""

import argparse
from pathlib import Path
import platform
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parent.parent
DOCKER = ["docker"] + (["--context", "desktop-linux"] if platform.system() == "Darwin" else [])


def scan_command(image, self_test=False):
    command = DOCKER + ["run", "--rm", "-i", "--network", "none", "--read-only",
                        "--tmpfs", "/scan:rw,noexec,nosuid,size=256m",
                        "--user", "65532:65532", "--cap-drop", "ALL",
                        "--security-opt", "no-new-privileges", image]
    if self_test:
        return command + ["gitleaks", "stdin", "--verbose", "--redact=100", "--no-banner", "--exit-code", "1"]
    return command + ["sh", "-ec", "cat > /scan/history.bundle; git init -q /scan/repo; "
                      "git -C /scan/repo fetch -q /scan/history.bundle HEAD; "
                      "git -C /scan/repo checkout -q --detach FETCH_HEAD; "
                      "gitleaks git /scan/repo --log-opts=--all --redact=100 --no-banner --exit-code 1"]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--self-test", action="store_true", help="Require rejection of a synthetic token")
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="oj-secret-check-") as directory:
        temp = Path(directory)
        bundle = temp / "history.bundle"
        # Export only HEAD's reachable history; no ignored/untracked files or other branches.
        if not args.self_test:
            subprocess.run(["git", "bundle", "create", str(bundle), "HEAD"], cwd=ROOT, check=True)
        iid = temp / "image-id"
        subprocess.run(DOCKER + ["build", "--target", "security-tools", "--iidfile", str(iid), "."],
                       cwd=ROOT, check=True)
        command = scan_command(iid.read_text().strip(), args.self_test)
        if args.self_test:
            # Synthetic only; never a credential. Keep it out of the repository's scan surface.
            token = 'ghp_' + 'Ab3dE7gH9jK2mN4pQ6rS8tV1wX5yZ0cD3eF7'
            fixture = ('github_token="' + token + '"\n').encode()
            result = subprocess.run(command, input=fixture, capture_output=True)
            if result.returncode != 1 or b"REDACTED" not in result.stdout + result.stderr:
                raise RuntimeError("Synthetic secret was not rejected with redacted output")
            if token.encode() in result.stdout + result.stderr:
                raise RuntimeError("Scanner exposed the synthetic input")
            print("Synthetic secret rejected; output redacted.")
        else:
            with bundle.open("rb") as source:
                subprocess.run(command, stdin=source, check=True)


if __name__ == "__main__":
    main()
