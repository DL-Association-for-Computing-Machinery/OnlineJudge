package sessionexperiment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const costConcurrency = 8
const costPadding = 1024
const costDSN = "host=postgres port=5432 user=oj_cost dbname=postgres sslmode=disable"

var costNonce = regexp.MustCompile(`^[0-9a-f]{32}$`)

type costConfig struct {
	backend, workload, nonce string
	subjects, operations     int
}

func parseCostConfig(get func(string) string) (costConfig, error) {
	c := costConfig{backend: get("OJ_SESSION_COST_BACKEND"), workload: get("OJ_SESSION_COST_WORKLOAD"), nonce: get("OJ_SESSION_COST_NONCE")}
	c.subjects, _ = strconv.Atoi(get("OJ_SESSION_COST_SUBJECTS"))
	c.operations, _ = strconv.Atoi(get("OJ_SESSION_COST_OPS"))
	if !costNonce.MatchString(c.nonce) || c.operations < 8 || c.operations > 8192 ||
		(c.backend != "redis" && c.backend != "postgres") {
		return c, errors.New("invalid private cost configuration")
	}
	switch c.workload {
	case "find", "save", "reject":
		if c.subjects == 1 || c.subjects == 32 {
			return c, nil
		}
	case "admit":
		if c.subjects == c.operations {
			return c, nil
		}
	}
	return c, errors.New("invalid cost workload or subject count")
}

type costPayload struct {
	Subject string `json:"subject"`
	Data    []byte `json:"data"`
}

type costFixture struct {
	Schema   int           `json:"schema"`
	Recovery string        `json:"recovery"`
	Deadline int64         `json:"deadline_ms"`
	Expiry   int64         `json:"expiry_ms"`
	Payloads []costPayload `json:"payloads"`
	SHA256   string        `json:"-"`
}

func writeCostFixture(path, nonce string, operations int) error {
	f := costFixture{Schema: 1, Recovery: nonce, Deadline: time.Now().Add(11 * time.Hour).UnixMilli(), Expiry: time.Now().Add(2 * time.Hour).UnixMilli()}
	for i := range max(operations, 64) {
		subject := fmt.Sprintf("cost-user-%06d", i)
		b, err := (scs.GobCodec{}).Encode(time.UnixMilli(f.Deadline), map[string]interface{}{
			"subject": subject, "remember_login": false, "__rememberMe": false,
			"auth_generation": int64(1), "recovery_generation": nonce, "padding": strings.Repeat("x", costPadding),
		})
		if err != nil {
			return err
		}
		f.Payloads = append(f.Payloads, costPayload{subject, b})
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	encodeErr := json.NewEncoder(file).Encode(f)
	closeErr := file.Close()
	return errors.Join(encodeErr, closeErr)
}

func TestSessionCostPrepare(t *testing.T) {
	if os.Getenv("OJ_SESSION_COST_PREPARE") != "1" {
		t.Skip("offline cost fixture entry; run scripts/test-session-cost.py")
	}
	n, err := strconv.Atoi(os.Getenv("OJ_SESSION_COST_OPS"))
	nonce := os.Getenv("OJ_SESSION_COST_NONCE")
	if err != nil || n < 8 || n > 8192 || !costNonce.MatchString(nonce) || os.Getenv("OJ_SESSION_COST_FIXTURE_OUT") != "/out/fixture.json" {
		t.Fatal("invalid offline cost fixture configuration")
	}
	if err := writeCostFixture("/out/fixture.json", nonce, n); err != nil {
		t.Fatal(err)
	}
}

func loadCostFixture(path string, operations int) (*costFixture, error) {
	info, err := os.Stat(path)
	if err != nil || info.Size() > 64<<20 || !info.Mode().IsRegular() {
		return nil, errors.New("missing or oversized private cost fixture")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f costFixture
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	if f.Schema != 1 || !costNonce.MatchString(f.Recovery) || len(f.Payloads) != max(operations, 64) || f.Expiry <= 0 || f.Expiry >= f.Deadline {
		return nil, errors.New("invalid cost fixture metadata")
	}
	for i, p := range f.Payloads {
		if p.Subject != fmt.Sprintf("cost-user-%06d", i) || len(p.Data) > 8192 {
			return nil, errors.New("invalid cost fixture identity or payload size")
		}
		deadline, values, err := (scs.GobCodec{}).Decode(p.Data)
		if err != nil || deadline.UnixMilli() != f.Deadline || len(values) != 6 || values["subject"] != p.Subject ||
			values["auth_generation"] != int64(1) || values["recovery_generation"] != f.Recovery ||
			values["remember_login"] != false || values["__rememberMe"] != false || values["padding"] != strings.Repeat("x", costPadding) {
			return nil, errors.New("cost fixture does not preserve the common ordinary-session contract")
		}
	}
	hash := sha256.Sum256(b)
	f.SHA256 = hex.EncodeToString(hash[:])
	return &f, nil
}

type costDataset struct {
	config    costConfig
	fixture   *costFixture
	namespace string
	pools     [2]*pgxpool.Pool
	clients   [2]*redis.Client
	stores    [2]scs.CtxStore
}

func openCostDataset(t *testing.T, c costConfig, f *costFixture) *costDataset {
	t.Helper()
	d := &costDataset{config: c, fixture: f}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for i := range 2 {
		config, err := pgxpool.ParseConfig(costDSN)
		if err != nil {
			t.Fatal(err)
		}
		config.MaxConns, config.ConnConfig.ConnectTimeout = 4, time.Second
		config.ConnConfig.RuntimeParams["application_name"] = fmt.Sprintf("oj-cost-client-%d", i)
		d.pools[i], err = pgxpool.NewWithConfig(ctx, config)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(d.pools[i].Close)
		var marker, user string
		var now int64
		err = d.pools[i].QueryRow(ctx, `SELECT nonce,current_user,floor(extract(epoch FROM clock_timestamp())*1000)::bigint FROM session_cost_owner`).Scan(&marker, &user, &now)
		if err != nil || marker != c.nonce || user != "oj_cost" || now >= f.Expiry || f.Deadline > now+(12*time.Hour).Milliseconds() {
			t.Fatal("private TCP fixture marker/identity/clock check failed", err,
				"marker_matches", marker == c.nonce, "user_matches", user == "oj_cost",
				"expiry_remaining_ms", f.Expiry-now, "deadline_remaining_ms", f.Deadline-now)
		}
		if c.backend == "redis" {
			d.clients[i] = sessionRedisClient("redis:6379", "oj-cost-client:", 8)
			t.Cleanup(func() { _ = d.clients[i].Close() })
			if err := d.clients[i].Ping(ctx).Err(); err != nil {
				t.Fatal("private Redis fixture not ready", err)
			}
		}
	}
	schema := postgresSessionSchema + postgresAccountSchema + ";" + postgresRecoverySchema
	if c.backend == "redis" {
		schema = sessionAuthSchema + ";" + sessionRecoverySchema
	}
	if _, err := d.pools[0].Exec(ctx, schema); err != nil {
		t.Fatal(err)
	}
	return d
}

func (d *costDataset) initialize(ctx context.Context, namespace string, subjects int, empty bool) error {
	d.namespace = namespace
	names := make([]string, subjects)
	for i := range subjects {
		names[i] = d.fixture.Payloads[i].Subject
		if d.config.backend == "postgres" {
			names[i] = digest(names[i])
		}
	}
	if d.config.backend == "postgres" {
		if _, err := d.pools[0].Exec(ctx, `INSERT INTO session_pg_account_fixture SELECT $1,subject,1,true FROM unnest($2::text[]) subject`, namespace, names); err != nil {
			return err
		}
		if _, err := d.pools[0].Exec(ctx, `INSERT INTO session_pg_recovery_fixture VALUES($1,$2,true)`, namespace, d.fixture.Recovery); err != nil {
			return err
		}
		for i := range 2 {
			d.stores[i] = &postgresSessionStore{pool: d.pools[i], namespace: namespace, account: true, recovery: true}
		}
	} else {
		if _, err := d.pools[0].Exec(ctx, `INSERT INTO session_auth_fixture SELECT $1,subject,1,true FROM unnest($2::text[]) subject`, namespace, names); err != nil {
			return err
		}
		if _, err := d.pools[0].Exec(ctx, `INSERT INTO session_recovery_fixture VALUES($1,$2,true)`, namespace, d.fixture.Recovery); err != nil {
			return err
		}
		for i := range 2 {
			d.stores[i] = &guardedStore{client: d.clients[i], prefix: namespace + ":", auth: &authAuthority{pool: d.pools[i], namespace: namespace}}
		}
	}
	if !empty {
		for i := range subjects {
			for slot := range 3 {
				if err := d.stores[slot%2].CommitCtx(ctx, costExistingToken(i, slot), d.fixture.Payloads[i].Data, time.UnixMilli(d.fixture.Expiry)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func costExistingToken(subject, slot int) string {
	return fmt.Sprintf("existing-%06d-%d", subject, slot)
}
