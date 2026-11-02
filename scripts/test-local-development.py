#!/usr/bin/env python3
"""Check local account initialization and recovery isolation with real Docker services."""

import argparse
from pathlib import Path
import platform
import re
import subprocess
import sys
import uuid


ROOT = Path(__file__).resolve().parent.parent
LABEL = "com.docker.compose.project"
STATE_FILES = ("ready.json", "account.json", "login-keys.json", "ca.crt", "server.crt", "server.key")


class CheckFailed(RuntimeError):
    """A fixed phase message that never includes captured container output."""


class LocalDevelopmentCheck:
    def __init__(self, context):
        self.project = "oj-local-check-" + uuid.uuid4().hex
        self.docker = ["docker"] + (["--context", context] if context else [])
        self.compose = self.docker + ["compose", "-p", self.project, "-f", str(ROOT / "compose.dev.yaml")]

    def run(self, command, phase, *, timeout=120, allow_failure=False):
        try:
            result = subprocess.run(command, cwd=ROOT, capture_output=True, text=True,
                                    timeout=timeout, check=False)
        except (OSError, subprocess.TimeoutExpired):
            raise CheckFailed(phase + " failed or timed out; Docker output was withheld.") from None
        if result.returncode and not allow_failure:
            raise CheckFailed(phase + " failed; Docker output was withheld.")
        return result

    def require(self, condition, phase):
        if not condition:
            raise CheckFailed(phase + " failed; runtime state was not printed.")

    def resources(self, kind, phase):
        if kind == "container":
            command = ["ps", "-a", "-q"]
        else:
            command = [kind, "ls", "-q"]
        return self.run(self.docker + command + ["--filter", "label=" + LABEL + "=" + self.project],
                        phase).stdout.split()

    def preflight(self):
        phase = "Fresh project ownership check"
        self.require(self.run(self.docker + ["info", "--format", "{{.OSType}}"], phase).stdout.strip() == "linux", phase)
        for kind in ("container", "volume", "network"):
            self.require(not self.resources(kind, phase), phase)
        # Refuse named collisions even if an existing resource has a wrong or
        # missing label. This UUID project must start without prior resources.
        for command in (["ps", "-a", "--format", "{{.Names}}"],
                        ["volume", "ls", "--format", "{{.Name}}"],
                        ["network", "ls", "--format", "{{.Name}}"]):
            names = self.run(self.docker + command, phase).stdout.splitlines()
            self.require(not any(name.startswith((self.project + "-", self.project + "_")) for name in names), phase)

    def initialize(self, phase, *, allow_failure=False):
        return self.run(self.compose + ["run", "--rm", "--no-deps", "-T", "localdev-init", "init"],
                        phase, allow_failure=allow_failure)

    def snapshot(self):
        phase = "Private state fingerprint check"
        command = "sha256sum " + " ".join("/state/" + name for name in STATE_FILES)
        output = self.run(self.compose + ["run", "--rm", "--no-deps", "-T", "--entrypoint", "sh",
                                           "localdev-init", "-ec", command], phase).stdout.strip()
        lines = output.splitlines()
        self.require(len(lines) == len(STATE_FILES) and all(
            re.fullmatch(r"[0-9a-f]{64}  /state/" + re.escape(name), line)
            for name, line in zip(STATE_FILES, lines)), phase)
        return output

    def redis_size(self):
        phase = "Redis marker count check"
        output = self.run(self.compose + ["exec", "-T", "redis", "redis-cli", "--raw", "DBSIZE"], phase).stdout.strip()
        self.require(bool(re.fullmatch(r"[0-9]+", output)), phase)
        return int(output)

    def check(self):
        print("Building isolated local development image.", flush=True)
        self.run(self.compose + ["build", "localdev-init"], "Local development image build", timeout=900)
        print("Starting private PostgreSQL and Redis services.", flush=True)
        self.run(self.compose + ["run", "--rm", "--no-deps", "-T", "volume-prepare"], "State volume preparation")
        self.run(self.compose + ["up", "-d", "--wait", "--wait-timeout", "90",
                                "postgres", "redis"], "Private service startup")
        print("Checking fresh and repeated account initialization.", flush=True)
        self.initialize("Fresh account initialization")
        original = self.snapshot()
        self.require(self.redis_size() > 0, "Fresh Redis marker initialization")
        self.initialize("Repeated account initialization")
        self.require(self.snapshot() == original, "Repeated initialization state preservation")
        print("Checking closed failure after isolated Redis marker loss.", flush=True)
        # FLUSHDB is limited to the newly created private UUID project's Redis.
        self.verify_ownership()
        result = self.run(self.compose + ["exec", "-T", "redis", "redis-cli", "--raw", "FLUSHDB"],
                          "Isolated Redis marker removal")
        self.require(result.stdout.strip() == "OK" and self.redis_size() == 0, "Isolated Redis marker removal")
        failed = self.initialize("Missing marker initialization refusal", allow_failure=True)
        self.require(failed.returncode > 0, "Missing marker initialization refusal")
        self.require(self.redis_size() == 0, "Missing marker preservation")
        self.require(self.snapshot() == original, "Refused initialization state preservation")

    def verify_ownership(self):
        phase = "Cleanup resource ownership check"
        for kind in ("container", "volume", "network"):
            identifiers = self.resources(kind, phase)
            if identifiers:
                labels = ".Config.Labels" if kind == "container" else ".Labels"
                template = '{{ index ' + labels + ' "' + LABEL + '" }}'
                output = self.run(self.docker + [kind, "inspect", "--format", template, *identifiers], phase).stdout.splitlines()
                self.require(len(output) == len(identifiers) and all(label == self.project for label in output), phase)
        # Check every volume Compose can remove, including incorrectly labeled
        # named volumes that a label-filtered inventory would miss.
        names = self.run(self.docker + ["volume", "ls", "--format", "{{.Name}}"], phase).stdout.splitlines()
        expected = {self.project + "_" + suffix for suffix in ("local-postgres", "local-redis", "local-state")}
        for name in expected.intersection(names):
            label = self.run(self.docker + ["volume", "inspect", "--format",
                                           '{{ index .Labels "' + LABEL + '" }}', name], phase).stdout.strip()
            self.require(label == self.project, phase)

    def cleanup(self):
        self.verify_ownership()
        self.run(self.compose + ["down", "--volumes", "--timeout", "15"], "Owned test resource cleanup")
        for kind in ("container", "volume", "network"):
            self.require(not self.resources(kind, "Cleanup verification"), "Cleanup verification")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--context", default="desktop-linux" if platform.system() == "Darwin" else None,
                        help="Docker context; macOS defaults to desktop-linux, Linux uses the current context")
    args = parser.parse_args()
    check = LocalDevelopmentCheck(args.context)
    try:
        check.preflight()
    except CheckFailed as error:
        print(str(error), file=sys.stderr)
        return 1
    status = 0
    try:
        check.check()
    except CheckFailed as error:
        print(str(error), file=sys.stderr)
        status = 1
    except KeyboardInterrupt:
        print("Local development check interrupted; checking owned resources before cleanup.", file=sys.stderr)
        status = 130
    finally:
        try:
            check.cleanup()
        except CheckFailed as error:
            print(str(error), file=sys.stderr)
            print("Cleanup could not be verified; existing resources were not forcibly removed.", file=sys.stderr)
            status = 1
    if not status:
        print("Local development checks passed: fresh init, repeated init, missing-marker refusal, owned cleanup.")
    return status


if __name__ == "__main__":
    sys.exit(main())
