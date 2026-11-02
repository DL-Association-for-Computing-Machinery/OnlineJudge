package sandbox_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"strings"
	"testing"
)

const apiLogLimit = 16 * 1024

// Parse only complete records emitted by this API process. No log text or
// arbitrary URL may choose the destination of the private experiment token.
func experimentAPIAddress(data []byte) (string, bool, error) {
	if len(data) > apiLogLimit {
		return "", false, errors.New("API startup log exceeded bound")
	}
	lines := bytes.Split(data, []byte{'\n'})
	address := ""
	for i, line := range lines {
		if len(line) > 2048 {
			return "", false, errors.New("API startup record exceeded bound")
		}
		if i == len(lines)-1 {
			break // A partial final record is not readiness evidence.
		}
		var record struct {
			Message string `json:"msg"`
			Scope   string `json:"scope"`
			Address string `json:"address"`
		}
		if err := json.Unmarshal(line, &record); err != nil {
			return "", false, errors.New("invalid complete API startup record")
		}
		if record.Message != "API starting" {
			continue
		}
		host, port, err := net.SplitHostPort(record.Address)
		number, numberErr := strconv.Atoi(port)
		if address != "" || record.Scope != "m0-acceptance-only" || err != nil || host != "127.0.0.1" || numberErr != nil || number < 1 || number > 65535 || strconv.Itoa(number) != port {
			return "", false, errors.New("unexpected API startup identity or listener")
		}
		address = "http://" + record.Address
	}
	return address, address != "", nil
}

func TestExperimentAPIAddress(t *testing.T) {
	valid := `{"msg":"API starting","scope":"m0-acceptance-only","address":"127.0.0.1:12345"}` + "\n"
	for _, tc := range []struct {
		name, data string
		found, bad bool
	}{
		{"valid", valid, true, false},
		{"other JSON before startup", "{\"msg\":\"other\"}\n" + valid, true, false},
		{"partial following record", valid + "{", true, false},
		{"empty", "", false, false},
		{"partial startup", strings.TrimSuffix(valid, "\n"), false, false},
		{"wrong message", strings.Replace(valid, "API starting", "API stopped", 1), false, false},
		{"wrong scope", strings.Replace(valid, "m0-acceptance-only", "bootstrap-only", 1), false, true},
		{"all addresses", strings.Replace(valid, "127.0.0.1", "0.0.0.0", 1), false, true},
		{"hostname", strings.Replace(valid, "127.0.0.1", "localhost", 1), false, true},
		{"external IP", strings.Replace(valid, "127.0.0.1", "192.0.2.1", 1), false, true},
		{"userinfo", strings.Replace(valid, "127.0.0.1", "user@127.0.0.1", 1), false, true},
		{"URL", strings.Replace(valid, "127.0.0.1:12345", "http://127.0.0.1:12345", 1), false, true},
		{"zero port", strings.Replace(valid, "12345", "0", 1), false, true},
		{"port overflow", strings.Replace(valid, "12345", "65536", 1), false, true},
		{"noncanonical port", strings.Replace(valid, "12345", "012345", 1), false, true},
		{"missing address", "{\"msg\":\"API starting\",\"scope\":\"m0-acceptance-only\"}\n", false, true},
		{"invalid complete JSON", "invalid\n", false, true},
		{"duplicate startup", valid + valid, false, true},
		{"long line", strings.Repeat(" ", 2049), false, true},
		{"long log", strings.Repeat("{}\n", apiLogLimit/3+1), false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, found, err := experimentAPIAddress([]byte(tc.data))
			if found != tc.found || (err != nil) != tc.bad || (found && got != "http://127.0.0.1:12345") {
				t.Fatalf("startup address: found=%v error=%v", found, err)
			}
		})
	}
}
