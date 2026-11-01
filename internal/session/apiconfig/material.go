package apiconfig

import (
	"path/filepath"
	"strings"

	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
)

func validPath(path string, optional bool) bool {
	if path == "" {
		return optional
	}
	return len(path) <= 4096 && !strings.ContainsRune(path, 0) && filepath.IsAbs(path) && filepath.Clean(path) == path
}

func materialPaths(m Material) bool {
	return validPath(m.CertificateFile, true) && validPath(m.KeyFile, true) && validPath(m.CAFile, true) &&
		!strings.ContainsRune(m.ServerName, 0)
}

func listenerMaterial(m Material, mode sessionruntime.Mode) bool {
	if !materialPaths(m) || m.CertificateFile == "" || m.KeyFile == "" || m.ServerName != "" {
		return false
	}
	return (mode == sessionruntime.DirectTLS && m.CAFile == "") || (mode == sessionruntime.ProxyMTLS && m.CAFile != "")
}

func storageSecurity(value string, m Material) (sessionruntime.Security, bool) {
	if !materialPaths(m) {
		return 0, false
	}
	switch value {
	case "private":
		return sessionruntime.PrivateTransport, m == (Material{})
	case "tls":
		return sessionruntime.VerifiedTLS, m.CAFile != "" && m.ServerName != "" &&
			((m.CertificateFile == "") == (m.KeyFile == ""))
	default:
		return 0, false
	}
}
