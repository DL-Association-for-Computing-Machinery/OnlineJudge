//go:build integration

package sessionprotocol_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

type entryLog struct {
	Message   string `json:"msg"`
	Address   string `json:"address"`
	Scope     string `json:"scope"`
	Operation string `json:"operation"`
	Stage     string `json:"stage"`
	Reason    string `json:"reason"`
	ElapsedMS int64  `json:"elapsed_ms"`
}

type entryProcess struct {
	cmd           *exec.Cmd
	done          chan struct{}
	notify        chan struct{}
	mu            sync.Mutex
	logs          []entryLog
	err           error
	invalidOutput bool
}

func entryBinary(t *testing.T) string {
	t.Helper()
	return entryProgram(t, "api")
}

func entryProgram(t *testing.T, name string) string {
	t.Helper()
	if dir := os.Getenv("OJ_PROTOCOL_ENTRY_BIN_DIR"); dir != "" {
		if os.Getenv("OJ_PROTOCOL_FIXTURE") != "1" || !filepath.IsAbs(dir) || (name != "api" && name != "migrate" && name != "mail-worker") {
			t.Fatal("invalid prebuilt entry fixture")
		}
		path := filepath.Join(dir, name)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0100 == 0 {
			t.Fatal("prebuilt race fixture binary unavailable")
		}
		return path
	}
	path := filepath.Join(t.TempDir(), name)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-race", "-mod=readonly", "-o", path, "../../cmd/"+name)
	if _, err := cmd.CombinedOutput(); err != nil {
		t.Fatal("race fixture binary build failed")
	}
	return path
}

func entryStart(t *testing.T, binary, config, address string) *entryProcess {
	t.Helper()
	cmd := exec.Command(binary)
	// The runtime receives only named test inputs, never ambient PG settings,
	// experiment switches, arbitrary credentials or the fixture database URL.
	cmd.Env = []string{"OJ_API_ADDR=" + address}
	if config != "" {
		cmd.Env = append(cmd.Env, "OJ_SESSION_CONFIG_FILE="+config)
	}
	reader, writer, err := os.Pipe()
	must(t, err)
	cmd.Stdout, cmd.Stderr = writer, writer
	if err := cmd.Start(); err != nil {
		_ = reader.Close()
		_ = writer.Close()
		t.Fatal("API child failed to start")
	}
	must(t, writer.Close())
	p := &entryProcess{cmd: cmd, done: make(chan struct{}), notify: make(chan struct{}, 1)}
	go func() {
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			var event entryLog
			if bytes.Contains(scanner.Bytes(), []byte("DATA RACE")) {
				p.invalidOutput = true
			}
			if json.Unmarshal(scanner.Bytes(), &event) == nil && event.Message != "" {
				p.mu.Lock()
				p.logs = append(p.logs, event)
				p.mu.Unlock()
				select {
				case p.notify <- struct{}{}:
				default:
				}
			} else {
				// Race reports and any unexpected diagnostics invalidate even an
				// expected failure; retain only a flag, never the original text.
				p.invalidOutput = true
			}
		}
		if scanner.Err() != nil {
			p.invalidOutput = true
		}
		if reader.Close() != nil {
			p.invalidOutput = true
		}
		p.err = cmd.Wait()
		close(p.done)
	}()
	t.Cleanup(func() {
		select {
		case <-p.done:
			return
		default:
		}
		_ = p.cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-p.done:
		case <-time.After(5 * time.Second):
			_ = p.cmd.Process.Kill()
			<-p.done
			t.Error("API child required forced cleanup")
		}
	})
	return p
}

func (p *entryProcess) event(message string) (entryLog, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, event := range p.logs {
		if event.Message == message {
			return event, true
		}
	}
	return entryLog{}, false
}

func (p *entryProcess) waitEvent(t *testing.T, message string) entryLog {
	t.Helper()
	deadline := time.NewTimer(6 * time.Second)
	defer deadline.Stop()
	for {
		if event, ok := p.event(message); ok {
			return event
		}
		select {
		case <-p.notify:
		case <-p.done:
			if event, ok := p.event(message); ok {
				return event
			}
			t.Fatal("API child exited before expected lifecycle event")
		case <-deadline.C:
			t.Fatal("API child lifecycle event timed out")
		}
	}
}

func (p *entryProcess) signal(t *testing.T) {
	t.Helper()
	must(t, p.cmd.Process.Signal(syscall.SIGTERM))
}

func (p *entryProcess) wait(t *testing.T, success bool) {
	t.Helper()
	select {
	case <-p.done:
		if p.invalidOutput {
			t.Fatal("API child emitted non-JSON or incomplete diagnostics")
		}
		if success {
			if p.err != nil {
				t.Fatal("unexpected API child failure")
			}
		} else {
			var exited *exec.ExitError
			if !errors.As(p.err, &exited) || exited.ExitCode() != 1 {
				t.Fatal("expected API configuration/shutdown failure did not exit exactly one")
			}
		}
	case <-time.After(6 * time.Second):
		t.Fatal("API child did not exit within bounded wait")
	}
}

func entryPath(t *testing.T, path string) string {
	t.Helper()
	canonical, err := filepath.EvalSymlinks(path)
	must(t, err)
	return canonical
}

func entryConfig(t *testing.T, c sessionruntime.Config, ca *testpki.Authority, shutdown time.Duration, blocklist ...string) string {
	t.Helper()
	return entryConfigWithWriteTimeout(t, c, ca, shutdown, 5*time.Second, blocklist...)
}

func entryConfigWithWriteTimeout(t *testing.T, c sessionruntime.Config, ca *testpki.Authority, shutdown, write time.Duration, blocklist ...string) string {
	t.Helper()
	_, cert, key := ca.Issue(t, "", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Hour))
	parameter := func(p password.Parameters) map[string]any {
		return map[string]any{"memory_kib": p.MemoryKiB, "iterations": p.Iterations, "parallelism": p.Parallelism,
			"salt_bytes": p.SaltBytes, "digest_bytes": p.DigestBytes}
	}
	legacy := make([]map[string]any, 0, len(c.PasswordPolicy.Legacy))
	for _, item := range c.PasswordPolicy.Legacy {
		legacy = append(legacy, parameter(item))
	}
	bucket := func(b protocol.LoginBucket) map[string]any {
		return map[string]any{"count": b.Count, "window": b.Window.String()}
	}
	l, a, db, redis, limits := c.PasswordLimits, c.PasswordAdmission, c.Database, c.Redis, c.LoginLimits
	doc := map[string]any{
		"version": 1, "mode": "direct", "namespace": c.Namespace, "secrets_file": entryPath(t, c.SecretsFile),
		"origin": c.HTTP.Origin, "login_window": c.LoginWindow.String(), "startup_timeout": c.StartupTimeout.String(), "http_timeout": c.HTTP.Timeout.String(),
		"database": entryDatabase(db),
		"redis": map[string]any{"address": redis.Addr, "username": redis.Username, "security": "private", "auth": "none",
			"connections": redis.Connections, "timeout": redis.Timeout.String(), "tls": map[string]any{}},
		"password": map[string]any{"current": parameter(c.PasswordPolicy.Current), "legacy": legacy,
			"limits": map[string]any{"max_memory_kib": l.MaxMemoryKiB, "max_iterations": l.MaxIterations, "max_parallelism": l.MaxParallelism,
				"max_password_bytes": l.MaxPasswordBytes, "max_salt_bytes": l.MaxSaltBytes, "max_digest_bytes": l.MaxDigestBytes},
			"admission": map[string]any{"max_in_flight": a.MaxInFlight, "max_waiting": a.MaxWaiting, "max_queue_wait": a.MaxQueueWait.String()}},
		"login_limits": map[string]any{"begin_global": bucket(limits.BeginGlobal), "begin_source": bucket(limits.BeginSource), "begin_email": bucket(limits.BeginEmail),
			"recovery_global": bucket(limits.RecoveryGlobal), "recovery_source": bucket(limits.RecoverySource)},
		"proxy_fingerprints": []string{}, "tls": map[string]any{"certificate_file": entryPath(t, cert), "key_file": entryPath(t, key)},
		"server": map[string]any{"read_header_timeout": "2s", "read_timeout": "5s", "write_timeout": write.String(), "idle_timeout": "1m",
			"shutdown_timeout": shutdown.String(), "max_header_bytes": 8192},
	}
	if c.Mode == sessionruntime.ProxyMTLS {
		doc["mode"] = "proxy"
		pins := make([]string, 0, len(c.Proxy.CertificateSHA256))
		for _, pin := range c.Proxy.CertificateSHA256 {
			pins = append(pins, hex.EncodeToString(pin[:]))
		}
		doc["proxy_fingerprints"] = pins
		doc["tls"].(map[string]any)["ca_file"] = entryPath(t, ca.CAFile)
	}
	if (c.PasswordChange != nil || c.EmailConfirmation != nil) && len(blocklist) != 1 {
		t.Fatal("enabled password capability needs explicit rule file")
	}
	if c.PasswordChange == nil && c.EmailConfirmation == nil && len(blocklist) != 0 {
		t.Fatal("rule file supplied without enabled capability")
	}
	if c.PasswordChange != nil {
		limits := c.PasswordChange.Limits
		doc["password_change"] = map[string]any{"minimum_characters": c.PasswordChange.Acceptance.MinimumCharacters(), "blocklist_file": blocklist[0], "global": bucket(limits.Global), "source": bucket(limits.Source), "subject": bucket(limits.Subject)}
	}
	if email := c.EmailLimits; email != nil {
		doc["email_limits"] = map[string]any{"request_global": bucket(email.RequestGlobal), "request_source": bucket(email.RequestSource),
			"request_mailbox": bucket(email.RequestMailbox), "confirm_global": bucket(email.ConfirmGlobal),
			"confirm_source": bucket(email.ConfirmSource), "confirm_token": bucket(email.ConfirmToken)}
	}
	if c.EmailConfirmation != nil {
		doc["email_confirmation"] = map[string]any{"minimum_characters": c.EmailConfirmation.MinimumCharacters(), "blocklist_file": blocklist[0]}
	}
	if intake := c.EmailIntake; intake != nil {
		doc["email_intake"] = map[string]any{"capacity": intake.Limits.Capacity, "workers": intake.Limits.Workers,
			"max_age": intake.Limits.MaxAge.String(), "payload_key_file": entryPath(t, intake.PayloadKeyFile),
			"budget_database": entryDatabase(intake.BudgetDatabase)}
	}
	if d := c.PrivateDrafts; d != nil {
		doc["private_drafts"] = map[string]any{"material_root": d.MaterialRoot, "bundle_directory": d.BundleDirectory,
			"source_lock_file": d.SourceLockFile, "source_ids": d.SourceIDs, "timeout": d.Timeout.String(), "max_in_flight": d.MaxInFlight,
			"reference_requests": d.ReferenceRequests, "review_intake": d.ReviewIntake, "review_access": d.ReviewAccess,
			"review_returns": d.ReviewReturns, "review_approvals": d.ReviewApprovals, "statement_confirmations": d.StatementConfirmations,
			"public_catalog": d.PublicCatalog}
		if d.LocalExecution != nil {
			doc["private_drafts"].(map[string]any)["aplusb_local_execution"] = d.LocalExecution
		}
		if d.LocalTextExecution != nil {
			doc["private_drafts"].(map[string]any)["local_text_execution"] = d.LocalTextExecution
		}
	}
	if c.TrainingHistory {
		doc["training_history"] = true
	}
	if c.TrainingSubmissionDetail {
		doc["training_submission_detail"] = true
	}
	if c.LocalTrainingIntake {
		doc["local_training_intake"] = true
	}
	if c.LocalJava21 {
		doc["local_java21"] = true
	}
	if c.LocalPython3 {
		doc["local_python3"] = true
	}
	if c.SessionDevices {
		doc["session_devices"] = true
	}
	if c.SessionRotation {
		doc["session_rotation"] = true
	}
	if c.ManagementSessions {
		doc["management_sessions"] = true
	}
	if c.ManagementRoleChanges {
		doc["management_role_changes"] = true
	}
	body, err := json.Marshal(doc)
	must(t, err)
	path := filepath.Join(t.TempDir(), "entry.json")
	must(t, os.WriteFile(path, body, 0600))
	return entryPath(t, path)
}

func entryDatabase(db sessionruntime.DatabaseConfig) map[string]any {
	return map[string]any{"host": db.Host, "port": db.Port, "database": db.Database, "username": db.Username,
		"application_name": db.ApplicationName, "security": "private", "auth": "none", "connections": db.Connections,
		"timeout": db.Timeout.String(), "max_lifetime": db.MaxLifetime.String(), "max_idle": db.MaxIdle.String(),
		"health_check": db.HealthCheck.String(), "tls": map[string]any{}}
}

func entryClient(t *testing.T, p *entryProcess, ca *testpki.Authority) *loginEndpoint {
	t.Helper()
	event := p.waitEvent(t, "API starting")
	if event.Scope != "ordinary-session-http" {
		t.Fatal("wrong entry scope")
	}
	tr := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: ca.Roots},
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", event.Address)
		}}
	t.Cleanup(tr.CloseIdleConnections)
	return &loginEndpoint{client: &http.Client{Transport: tr, Timeout: 6 * time.Second}}
}
