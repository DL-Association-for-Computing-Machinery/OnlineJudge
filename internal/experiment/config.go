package experiment

import (
	"errors"
	"io"
	"net"
	"os"
	"strings"
)

// LoadToken permits only a literal loopback listener and an owner-private token file.
func LoadToken(addr, path string) (string, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return "", errors.New("M0 experiment requires a literal loopback listener")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", errors.New("cannot open experiment token file")
	}
	// Cleanup does not change the completed operation or its primary error.
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return "", errors.New("experiment token must be an owner-private regular file")
	}
	value, err := io.ReadAll(io.LimitReader(file, 258))
	token := strings.TrimSpace(string(value))
	if err != nil || len(token) < 32 || len(token) > 128 || strings.ContainsAny(token, " \t\r\n") {
		return "", errors.New("invalid experiment token")
	}
	return token, nil
}
