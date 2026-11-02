package localdev

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/checkedfile"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

var errState = errors.New("local development state is missing, partial or incompatible; data was preserved")

type account struct {
	ID       string `json:"subject_id"`
	Email    string `json:"email"`
	Password string `json:"password"`
}
type manifest struct {
	Version    int               `json:"version"`
	Generation string            `json:"generation"`
	Files      map[string]string `json:"files"`
}

var materialNames = []string{"ca.crt", "server.crt", "server.key", "login-keys.json", "account.json"}

func randomText(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", errState
	}
	defer clear(b)
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func writeNew(dir, name string, body []byte) error {
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errState
	}
	_, writeErr := f.Write(body)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return errState
	}
	return nil
}

func writeJSON(dir, name string, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return errState
	}
	defer clear(body)
	return writeNew(dir, name, body)
}

func generateCertificates(dir string) error {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return errState
	}
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return errState
	}
	now := time.Now()
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "OnlineJudge local development CA"}, NotBefore: now.Add(-time.Minute), NotAfter: now.AddDate(1, 0, 0), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	root, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		return errState
	}
	// Use the encoded parent: CreateCertificate supplies its subject key ID.
	// Passing the unpopulated template would omit the leaf authority key ID,
	// which strict TLS clients correctly reject.
	ca, err = x509.ParseCertificate(root)
	if err != nil {
		return errState
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}, NotBefore: now.Add(-time.Minute), NotAfter: now.AddDate(0, 3, 0), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	cert, err := x509.CreateCertificate(rand.Reader, leaf, ca, &serverKey.PublicKey, caKey)
	if err != nil {
		return errState
	}
	key, err := x509.MarshalPKCS8PrivateKey(serverKey)
	if err != nil {
		return errState
	}
	defer clear(key)
	for name, block := range map[string]*pem.Block{"ca.crt": {Type: "CERTIFICATE", Bytes: root}, "server.crt": {Type: "CERTIFICATE", Bytes: cert}, "server.key": {Type: "PRIVATE KEY", Bytes: key}} {
		if err := writeNew(dir, name, pem.EncodeToMemory(block)); err != nil {
			return err
		}
	}
	return nil
}

func generate(dir string) error {
	if err := generateCertificates(dir); err != nil {
		return err
	}
	sealing, err := randomText(32)
	if err != nil {
		return err
	}
	limiting, err := randomText(32)
	if err != nil {
		return err
	}
	if err := writeJSON(dir, "login-keys.json", map[string]any{"version": 1, "current": "local", "sealing": map[string]string{"local": sealing}, "limiting": limiting}); err != nil {
		return err
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return errState
	}
	secret, err := randomText(32)
	if err != nil {
		return err
	}
	return writeJSON(dir, "account.json", account{ID: hex.EncodeToString(id), Email: "developer@example.invalid", Password: secret})
}

func readAccount(dir string) (account, error) {
	var a account
	b, err := checkedfile.ReadSecret(filepath.Join(dir, "account.json"), 4096)
	if err != nil {
		return a, errState
	}
	defer clear(b)
	if json.Unmarshal(b, &a) != nil || !identity.ValidID(a.ID) || a.Email != "developer@example.invalid" || len(a.Password) != 43 {
		return account{}, errState
	}
	return a, nil
}

func materialHashes(dir string) (map[string]string, error) {
	hashes := make(map[string]string)
	for _, name := range materialNames {
		b, err := checkedfile.ReadSecret(filepath.Join(dir, name), 16384)
		if err != nil {
			return nil, errState
		}
		digest := sha256.Sum256(b)
		clear(b)
		hashes[name] = hex.EncodeToString(digest[:])
	}
	return hashes, nil
}

func certificate(dir string) (tls.Certificate, error) {
	cert, err := checkedfile.ReadPublic(filepath.Join(dir, "server.crt"), 16384)
	if err != nil {
		return tls.Certificate{}, errState
	}
	key, err := checkedfile.ReadSecret(filepath.Join(dir, "server.key"), 16384)
	if err != nil {
		return tls.Certificate{}, errState
	}
	defer clear(key)
	pair, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return tls.Certificate{}, errState
	}
	ca, err := checkedfile.ReadPublic(filepath.Join(dir, "ca.crt"), 16384)
	if err != nil {
		return tls.Certificate{}, errState
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return tls.Certificate{}, errState
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return tls.Certificate{}, errState
	}
	if _, err = leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: "localhost"}); err != nil {
		return tls.Certificate{}, errState
	}
	return pair, nil
}
