package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/url"
	"os"
	"runtime"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

type config struct {
	slot        workerhost.M0Slot
	fixture     experiment.WorkerFixture
	endpoint    string
	roots       *x509.CertPool
	cert        tls.Certificate
	startAPlusB bool
}

// Configuration errors never echo file paths, endpoints or credential contents.
func loadConfig(getenv func(string) string) (config, error) {
	var c config
	if getenv("OJ_SANDBOX_EXPERIMENT") != "1" {
		return c, errors.New("explicit OJ_SANDBOX_EXPERIMENT=1 required")
	}
	var err error
	c.slot, err = workerhost.ParseM0Slot(getenv("OJ_WORKER_M0_SLOT"))
	if err != nil {
		return c, err
	}
	c.fixture, err = m0Fixture(getenv, c.slot)
	if err != nil {
		return c, err
	}
	_, aplusb, err := experiment.LoadAPlusBFixture(getenv)
	if err != nil {
		return c, err
	}
	if aplusb.IsAPlusB() {
		if getenv("OJ_WORKER_M0_SLOT") != "" || getenv(dualModeEnv) != "" || runtime.GOARCH != "arm64" {
			return c, errors.New("A+B requires a dedicated arm64 single reference slot")
		}
		if aplusb.APlusBTier() == experiment.ReferenceAPlusBUbuntu {
			if runtime.GOOS != "linux" || os.Geteuid() != 0 || getenv("OJ_APLUSB_START_STOPPED") != "1" {
				return c, errors.New("the Ubuntu A+B Worker requires the dedicated guest owner and explicit stopped-slot startup")
			}
		} else if runtime.GOOS != "darwin" {
			return c, errors.New("the Desktop A+B Worker requires the isolated Desktop reference slot")
		}
		c.fixture = aplusb
	}
	switch getenv("OJ_APLUSB_START_STOPPED") {
	case "":
	case "1":
		if !c.fixture.IsAPlusB() {
			return c, errors.New("managed reference startup requires an explicit A+B fixture")
		}
		c.startAPlusB = true
	default:
		return c, errors.New("invalid managed A+B startup setting")
	}
	c.endpoint = getenv("OJ_WORKER_CONTROLLER_URL")
	u, err := url.Parse(c.endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.ForceQuery ||
		u.Fragment != "" || (u.Path != "" && u.Path != "/") || u.RawPath != "" {
		return c, errors.New("explicit loopback HTTPS controller required")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return c, errors.New("M0 controller must use a literal loopback address")
	}
	key := getenv("OJ_WORKER_KEY_FILE")
	info, err := os.Lstat(key)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return c, errors.New("worker key must be an owner-private regular file")
	}
	c.cert, err = tls.LoadX509KeyPair(getenv("OJ_WORKER_CERT_FILE"), key)
	if err != nil {
		return c, errors.New("cannot load worker certificate and key")
	}
	data, err := os.ReadFile(getenv("OJ_WORKER_CA_FILE"))
	if err != nil {
		return c, errors.New("cannot load controller CA")
	}
	c.roots = x509.NewCertPool()
	if !c.roots.AppendCertsFromPEM(data) {
		return c, errors.New("invalid controller CA")
	}
	return c, nil
}

func m0Fixture(getenv func(string) string, slot workerhost.M0Slot) (experiment.WorkerFixture, error) {
	f, err := experiment.ParseWorkerFixture(getenv("OJ_EXPERIMENT_MATERIAL_VERSION"))
	if err == nil && f.Version() == experiment.UbuntuVersion {
		if runtime.GOOS != "linux" || runtime.GOARCH != "arm64" || os.Geteuid() != 0 ||
			slot != (workerhost.M0Slot{}) || getenv(dualModeEnv) != "" {
			return f, errors.New("the Ubuntu M0 fixture requires the dedicated Linux arm64 VM owner and single primary slot")
		}
	}
	return f, err
}
