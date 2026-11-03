// Package m0browser owns the explicitly enabled, local synthetic judge page.
// It grants no account identity or general-purpose source submission capability.
package m0browser

import (
	"errors"
	"net"
	"strconv"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

// ErrConfiguration reports invalid local demonstration configuration safely.
var ErrConfiguration = errors.New("invalid M0 browser demonstration configuration")

// Enabled validates the complete opt-in before opening stores or listeners.
func Enabled(flag, addr, tokenFile, sessionFile, version string) (bool, error) {
	if flag == "" {
		return false, nil
	}
	if flag != "1" || tokenFile == "" || sessionFile != "" || version != experiment.PinnedVersion || !localAddress(addr, true) {
		return false, ErrConfiguration
	}
	return true, nil
}

func localAddress(addr string, allowZero bool) bool {
	host, port, err := net.SplitHostPort(addr)
	number, numberErr := strconv.Atoi(port)
	return err == nil && numberErr == nil && host == "127.0.0.1" && number <= 65535 &&
		(number > 0 || (allowZero && number == 0)) && strconv.Itoa(number) == port
}
