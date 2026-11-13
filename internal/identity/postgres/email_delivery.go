package postgres

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres/identitydb"
)

// EmailDeliveryReader prepares an eligible mail payload using one PostgreSQL
// snapshot and the sender's immutable protector.
type EmailDeliveryReader struct {
	q         *identitydb.Queries
	protector *emailtoken.Protector
}

// NewEmailDeliveryReader uses a caller-owned pool and does not open it or read
// account passwords. The caller provisions a bounded sender pool.
func NewEmailDeliveryReader(pool *pgxpool.Pool, protector *emailtoken.Protector) (*EmailDeliveryReader, error) {
	if pool == nil || protector.CurrentKeyID() == "" {
		return nil, identity.ErrInvalid
	}
	return &EmailDeliveryReader{q: identitydb.New(pool), protector: protector}, nil
}

// EmailDelivery is a sensitive, point-in-time result for a trusted mail
// renderer. It is neither a queue record nor a proof that a job is owned.
type EmailDelivery struct {
	scope emailtoken.Scope
	token emailtoken.Token
}

// Scope returns the destination, purpose and deadlines for this send.
func (d EmailDelivery) Scope() emailtoken.Scope { return d.scope }

// Token returns the credential for rendering the mail link.
func (d EmailDelivery) Token() emailtoken.Token { return d.token }

// Format redacts recipient and token from diagnostics.
func (EmailDelivery) Format(state fmt.State, _ rune) {
	_, _ = state.Write([]byte("[redacted email delivery]"))
}

// Prepare checks and decrypts one job immediately before each send attempt.
// ErrConflict means no eligible row in the snapshot; ErrStore means the read
// or payload check failed. No lock or connection is held during SMTP I/O.
// A later account change may make a prepared link unusable. The sender must
// still enforce the send deadline, queue ownership and attempt limits.
func (r *EmailDeliveryReader) Prepare(ctx context.Context, payloadID string) (EmailDelivery, error) {
	if r == nil || r.q == nil || ctx == nil || !identity.ValidID(payloadID) {
		return EmailDelivery{}, identity.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	row, err := r.q.EmailPayloadForDelivery(ctx, payloadID)
	if errors.Is(err, pgx.ErrNoRows) {
		return EmailDelivery{}, identity.ErrConflict
	}
	if err != nil {
		return EmailDelivery{}, storageError{err}
	}
	scope, err := deliveryScope(payloadID, row)
	if err != nil {
		return EmailDelivery{}, err
	}
	token, err := r.protector.Open(row.CheckedAt.Time, scope, emailtoken.Payload{KeyID: row.KeyID, Ciphertext: row.Ciphertext})
	if err != nil {
		return EmailDelivery{}, storageError{err}
	}
	digest, err := token.Digest(scope.Purpose)
	if err != nil || subtle.ConstantTimeCompare(digest[:], row.Digest) != 1 {
		return EmailDelivery{}, storageError{identity.ErrInvalid}
	}
	if err := ctx.Err(); err != nil {
		return EmailDelivery{}, err
	}
	return EmailDelivery{scope: scope, token: token}, nil
}

func deliveryScope(payloadID string, row identitydb.EmailPayloadForDeliveryRow) (emailtoken.Scope, error) {
	for _, instant := range []pgtype.Timestamptz{row.CheckedAt, row.IssuedAt, row.ExpiresAt, row.SendUntil} {
		if !instant.Valid || instant.InfinityModifier != pgtype.Finite || instant.Time.IsZero() {
			return emailtoken.Scope{}, storageError{identity.ErrInvalid}
		}
	}
	recipient, err := identity.ParseEmail(row.OriginalEmail)
	if err != nil || recipient.Key() != row.EmailKey || row.ID != payloadID {
		return emailtoken.Scope{}, storageError{identity.ErrInvalid}
	}
	return emailtoken.Scope{
		PayloadID: row.ID, SubjectID: row.SubjectID, RecoveryID: row.RecoveryID,
		AuthGeneration: row.AuthGeneration, Recipient: recipient, Purpose: emailtoken.Purpose(row.Purpose),
		IssuedAt: row.IssuedAt.Time, SendUntil: row.SendUntil.Time, TokenExpiry: row.ExpiresAt.Time,
	}, nil
}
