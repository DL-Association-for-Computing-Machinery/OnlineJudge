// Package checkedfile reads bounded snapshots from explicitly configured local
// files. It never creates files, follows symlinks or exposes paths in errors.
package checkedfile

import "errors"

// ErrUnavailable hides filesystem details and configured paths.
var ErrUnavailable = errors.New("configured file unavailable or invalid")

// ReadSecret requires a single-link regular file owned by root/euid with mode
// 0400/0600. Linux/Darwin fd checks include every ancestor and before/after state.
// Use a trusted local filesystem/ACL; concurrent malicious privileged writers
// and hard cancellation of synchronous reads are outside this contract.
func ReadSecret(path string, limit int64) ([]byte, error) { return read(path, limit, true) }

// ReadPublic applies the same ownership/type/link/directory checks but permits
// group/other read access. Group/other write, execution and special mode bits are
// forbidden. Both readers require a nonempty snapshot and limit in 1..1MiB.
func ReadPublic(path string, limit int64) ([]byte, error) { return read(path, limit, false) }
