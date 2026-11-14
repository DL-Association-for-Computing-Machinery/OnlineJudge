#!/usr/bin/env python3
"""Import runtime images selected by an independently pinned trial release."""

import argparse
import json
from pathlib import Path
import subprocess
import sys
import tarfile

from cold_trial_images import receiver
from trial_release import verify


def receive_release(manifest, digest, app, archive, socket, data_root):
    release = verify(manifest, digest, app, archive)
    if release["platform"] != "linux/arm64":
        raise ValueError("the cold trial rehearsal requires linux/arm64")
    # Recheck the archive at import; no role, platform or archive pin is
    # supplied independently of the verified release.
    report = receiver(socket, data_root, archive, release["runtime_sha256"],
                      release["platform"], release["image_configurations"])
    return {**report, "release_sha256": release["release_sha256"],
            "application_sha256": release["application_sha256"], "revision": release["revision"]}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("manifest", type=Path)
    parser.add_argument("--sha256", required=True)
    parser.add_argument("--app", type=Path, required=True)
    parser.add_argument("--runtime", type=Path, required=True)
    parser.add_argument("--socket", type=Path, required=True)
    parser.add_argument("--data-root", type=Path, required=True)
    args = parser.parse_args()
    try:
        print(json.dumps(receive_release(args.manifest, args.sha256, args.app, args.runtime,
                                         args.socket, args.data_root), sort_keys=True))
    except (OSError, ValueError, KeyError, TypeError, AttributeError, RecursionError,
            tarfile.TarError, subprocess.SubprocessError) as exc:
        print("trial release cold import refused or failed: " + type(exc).__name__, file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
