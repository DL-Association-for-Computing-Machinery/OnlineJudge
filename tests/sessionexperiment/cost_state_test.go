package sessionexperiment

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type costPGRow struct {
	token, subject   string
	data             []byte
	deadline, expiry int64
}

type costState struct {
	authority []byte
	registry  []string
	rows      []costPGRow
	memory    map[string]memoryRecord
}

func (d *costDataset) state(ctx context.Context) (costState, error) {
	var state costState
	query := `SELECT jsonb_build_object(
		'accounts',(SELECT jsonb_agg(to_jsonb(a) ORDER BY subject) FROM session_pg_account_fixture a WHERE namespace=$1),
		'gate',(SELECT to_jsonb(g) FROM session_pg_recovery_fixture g WHERE namespace=$1))`
	if d.config.backend == "redis" {
		query = `SELECT jsonb_build_object(
			'accounts',(SELECT jsonb_agg(to_jsonb(a) ORDER BY subject) FROM session_auth_fixture a WHERE namespace=$1),
			'gate',(SELECT to_jsonb(g) FROM session_recovery_fixture g WHERE namespace=$1))`
	}
	if err := d.pools[0].QueryRow(ctx, query, d.namespace).Scan(&state.authority); err != nil {
		return state, err
	}
	if d.config.backend == "redis" {
		var err error
		state.memory, err = readMemorySnapshot(ctx, d.clients[0], d.namespace+":")
		return state, err
	}
	rows, err := d.pools[0].Query(ctx, `SELECT token,subject,data,deadline_ms,expiry_ms FROM session_pg_store_fixture WHERE namespace=$1 ORDER BY token`, d.namespace)
	if err != nil {
		return state, err
	}
	for rows.Next() {
		var row costPGRow
		if err := rows.Scan(&row.token, &row.subject, &row.data, &row.deadline, &row.expiry); err != nil {
			rows.Close()
			return state, err
		}
		state.rows = append(state.rows, row)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return state, err
	}
	rows, err = d.pools[0].Query(ctx, `SELECT subject FROM session_pg_subject_fixture WHERE namespace=$1 ORDER BY subject`, d.namespace)
	if err != nil {
		return state, err
	}
	defer rows.Close()
	for rows.Next() {
		var subject string
		if err := rows.Scan(&subject); err != nil {
			return state, err
		}
		state.registry = append(state.registry, subject)
	}
	return state, rows.Err()
}

func (d *costDataset) validateAdmissions(before, after costState) error {
	if !bytes.Equal(before.authority, after.authority) || len(before.rows) != 0 || len(before.memory) != 0 || len(before.registry) != 0 {
		return errors.New("admission authority changed or initial session state was not empty")
	}
	if d.config.backend == "postgres" {
		if len(after.rows) != d.config.operations || len(after.registry) != d.config.operations {
			return errors.New("admission did not create exactly one registry and session per subject")
		}
		expected := make(map[string]costPayload)
		for i := range d.config.operations {
			expected[digest(fmt.Sprintf("new-%06d", i))] = d.fixture.Payloads[i]
		}
		for _, row := range after.rows {
			p, exists := expected[row.token]
			if !exists || row.subject != digest(p.Subject) || !bytes.Equal(row.data, p.Data) || row.deadline != d.fixture.Deadline || row.expiry != d.fixture.Expiry {
				return errors.New("admitted PostgreSQL row does not match the frozen credential")
			}
			delete(expected, row.token)
		}
		return nil
	}
	if len(after.memory) != 2*d.config.operations {
		return errors.New("admission did not create exactly one Redis token/index per subject")
	}
	for i := range d.config.operations {
		p := d.fixture.Payloads[i]
		key := d.namespace + ":token:" + digest(fmt.Sprintf("new-%06d", i))
		hash := digest(p.Subject + ":1:" + d.fixture.Recovery)
		token, found := after.memory[key]
		index, indexed := after.memory[d.namespace+":subject:"+hash]
		if !found || !indexed || len(token.fields) != 4 || token.fields["subject"] != hash ||
			token.fields["data"] != string(p.Data) || token.fields["deadline"] != strconv.FormatInt(d.fixture.Deadline, 10) ||
			token.fields["expiry"] != strconv.FormatInt(d.fixture.Expiry, 10) || token.expires != time.Duration(d.fixture.Deadline)*time.Millisecond ||
			len(index.slots) != 1 || index.slots[0].Member != key || index.slots[0].Score != float64(d.fixture.Expiry) ||
			index.expires != time.Duration(d.fixture.Expiry)*time.Millisecond {
			return errors.New("admitted Redis token/index does not match the frozen credential")
		}
	}
	return nil
}

func (d *costDataset) poolStats() map[string][]map[string]int64 {
	result := map[string][]map[string]int64{"postgres": {}, "redis": {}}
	for i := range 2 {
		s := d.pools[i].Stat()
		result["postgres"] = append(result["postgres"], map[string]int64{
			"total_connections": int64(s.TotalConns()), "max_connections": int64(s.MaxConns()),
			"acquires": s.AcquireCount(), "acquire_ns": s.AcquireDuration().Nanoseconds(),
			"empty_acquires": s.EmptyAcquireCount(), "empty_acquire_wait_ns": s.EmptyAcquireWaitTime().Nanoseconds(),
			"canceled_acquires": s.CanceledAcquireCount(),
		})
		if d.clients[i] != nil {
			s := d.clients[i].PoolStats()
			result["redis"] = append(result["redis"], map[string]int64{
				"total_connections": int64(s.TotalConns), "timeouts": int64(s.Timeouts),
				"waits": int64(s.WaitCount), "wait_ns": s.WaitDurationNs,
			})
		}
	}
	return result
}

func (d *costDataset) storageStats(ctx context.Context, beginLSN, endLSN string, warmSubjects int) (map[string]any, error) {
	var version, fsync, syncCommit, fullPage, autovacuum string
	var wal, databaseBytes, tableBytes int64
	err := d.pools[0].QueryRow(ctx, `SELECT version(),current_setting('fsync'),current_setting('synchronous_commit'),
		current_setting('full_page_writes'),current_setting('autovacuum'),
		pg_wal_lsn_diff($2::pg_lsn,$1::pg_lsn)::bigint,pg_database_size(current_database()),
		(SELECT coalesce(sum(pg_total_relation_size(oid)),0)::bigint FROM pg_class WHERE relnamespace='public'::regnamespace AND relkind='r')`, beginLSN, endLSN).
		Scan(&version, &fsync, &syncCommit, &fullPage, &autovacuum, &wal, &databaseBytes, &tableBytes)
	if err != nil {
		return nil, err
	}
	var connections int
	if err := d.pools[0].QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE application_name LIKE 'oj-cost-client-%'`).Scan(&connections); err != nil {
		return nil, err
	}
	result := map[string]any{
		"postgres_version": version, "postgres_fsync": fsync, "postgres_synchronous_commit": syncCommit,
		"postgres_full_page_writes": fullPage, "postgres_autovacuum": autovacuum,
		"postgres_wal_insert_bytes_delta": wal, "postgres_database_bytes": databaseBytes,
		"postgres_wal_window":                      "LSN queries immediately around the timed run, excluding later validation; cluster-wide including background writes; boundary SQL is outside Go elapsed",
		"postgres_public_tables_and_indexes_bytes": tableBytes, "postgres_client_connections_endpoint": connections,
		"warmup_subjects_retained": warmSubjects, "warmup_requests": 64,
		"durability_note": "disposable PostgreSQL tmpfs; fsync settings do not establish durable disk costs",
		"pool_note":       "two PostgreSQL pools capped at 4 each; Redis path additionally has two Redis pools capped at 8 each; endpoints are not connection peaks",
	}
	if d.config.backend == "redis" {
		client := d.clients[0]
		keys, err := client.Keys(ctx, d.namespace+":*").Result()
		if err != nil {
			return nil, err
		}
		pipe := client.Pipeline()
		commands := make([]*redis.IntCmd, 0, len(keys))
		for _, key := range keys {
			commands = append(commands, pipe.MemoryUsage(ctx, key))
		}
		_, err = pipe.Exec(ctx)
		if err != nil {
			return nil, err
		}
		var total int64
		for _, command := range commands {
			n, err := command.Result()
			if err != nil {
				return nil, err
			}
			total += n
		}
		info, err := client.Info(ctx, "memory", "server").Result()
		if err != nil {
			return nil, err
		}
		config := make(map[string]string)
		for _, key := range []string{"save", "appendonly", "maxmemory", "maxmemory-policy"} {
			values, err := client.ConfigGet(ctx, key).Result()
			if err != nil {
				return nil, err
			}
			value, exists := values[key]
			if !exists {
				return nil, errors.New("missing Redis cost configuration value")
			}
			config[key] = value
		}
		result["redis_measured_namespace_keys"] = len(keys)
		result["redis_measured_namespace_memory_usage_bytes"] = total
		selected := make(map[string]string)
		for line := range strings.SplitSeq(info, "\n") {
			key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
			if ok && (key == "redis_version" || key == "used_memory" || key == "used_memory_peak" || key == "used_memory_dataset" || key == "used_memory_overhead") {
				selected[key] = value
			}
		}
		result["redis_info_memory_and_version"] = selected
		result["redis_configuration"] = config
	}
	return result, nil
}
