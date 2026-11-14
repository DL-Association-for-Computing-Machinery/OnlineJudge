package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres/submissiondb"
)

var materialVersionPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)
var hexDigestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// AcceptTraining atomically rechecks a fresh ordinary account and enqueues
// personal training work using this server-selected immutable material version.
// The caller must authorize the selected version before calling; fixture
// registration is not publication. Use only the ordinary identity database.
func (s *Store) AcceptTraining(ctx context.Context, actor identity.TrainingActor, key, language, source string) (submission.Receipt, bool, error) {
	if s == nil || s.pool == nil || ctx == nil || !actor.Valid() || language != submission.LanguageGNUCPP17 {
		return submission.Receipt{}, false, submission.ErrInvalid
	}
	req := submission.Request{MaterialVersion: s.version, Language: language, Source: source}
	return s.accept(ctx, actor.SubjectID, submission.PersonalTrainingScope, key, req, submission.Digest(req),
		func(ctx context.Context, tx pgx.Tx) error { return identitypg.LockTrainingAccount(ctx, tx, actor) })
}

// PublishedMaterial identifies the registered immutable execution material.
type PublishedMaterial struct {
	Version, Digest string
}

// ResolvePublication resolves the current publication inside the acceptance
// transaction. It must reject stale versions and unadmitted material.
type ResolvePublication func(context.Context, pgx.Tx, string, string) (PublishedMaterial, error)

// AcceptPublishedTraining binds a key to the displayed publication as well as
// the immutable material. Authorization, resolution and enqueue share one transaction.
func AcceptPublishedTraining(ctx context.Context, pool *pgxpool.Pool, actor identity.TrainingActor, key, problemID, version, language, source string, resolve ResolvePublication) (submission.Receipt, bool, error) {
	if pool == nil || ctx == nil || !actor.Valid() || resolve == nil || !identity.ValidID(problemID) ||
		!hexDigestPattern.MatchString(version) ||
		submission.Validate(actor.SubjectID, submission.PersonalTrainingScope, key, submission.Request{
			MaterialVersion: version, Language: language, Source: source,
		}) != nil {
		return submission.Receipt{}, false, submission.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return submission.Receipt{}, false, err
	}
	defer rollback(tx)
	if err := identitypg.LockTrainingAccount(ctx, tx, actor); err != nil {
		return submission.Receipt{}, false, err
	}
	m, err := resolve(ctx, tx, problemID, version)
	if err != nil {
		return submission.Receipt{}, false, err
	}
	if !materialVersionPattern.MatchString(m.Version) || !hexDigestPattern.MatchString(m.Digest) {
		return submission.Receipt{}, false, submission.ErrMaterial
	}
	queue := Store{version: m.Version, manifestHash: m.Digest}
	req := submission.Request{MaterialVersion: m.Version, Language: language, Source: source}
	sum := sha256.Sum256([]byte("published-training\x00" + problemID + "\x00" + version + "\x00" + submission.Digest(req)))
	return queue.acceptInTx(ctx, tx, actor.SubjectID, submission.PersonalTrainingScope, key, req, hex.EncodeToString(sum[:]))
}

// ReadTrainingReceipt recovers the original key in the currently authorized
// account's personal scope. An absent receipt cannot exclude an in-flight write.
func ReadTrainingReceipt(ctx context.Context, pool *pgxpool.Pool, actor identity.TrainingActor, key string) (submission.Receipt, error) {
	if ctx == nil || pool == nil || !actor.Valid() || !submission.ValidKey(key) {
		return submission.Receipt{}, submission.ErrInvalid
	}
	return readTraining(ctx, pool, actor, func(ctx context.Context, q *submissiondb.Queries) (submission.Receipt, error) {
		return lookupReceipt(ctx, q, actor.SubjectID, submission.PersonalTrainingScope, key)
	})
}
