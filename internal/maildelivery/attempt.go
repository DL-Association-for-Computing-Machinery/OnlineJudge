// Package maildelivery runs one account email attempt for a claimed job.
package maildelivery

import (
	"context"
	"errors"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailmessage"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailtransport"
)

// Outcome is safe to record without exposing a recipient, credential or error.
type Outcome uint8

const (
	// Rejected covers malformed jobs, rendering failures or definite SMTP refusal.
	Rejected Outcome = iota
	// Obsolete means the payload is missing, ineligible or past its send window.
	Obsolete
	// Retry means no message was accepted; the caller may retry within policy.
	Retry
	// Unknown means SMTP may have accepted the message.
	Unknown
	// Accepted means the SMTP server accepted the message.
	Accepted
)

// Attempt connects the eligibility reader, trusted renderer and SMTP transport.
type Attempt struct {
	reader   *identitypg.EmailDeliveryReader
	renderer *emailmessage.Renderer
	send     func(context.Context, identity.Email, string, string) mailtransport.Outcome
}

// New constructs an attempt runner without opening a connection.
func New(reader *identitypg.EmailDeliveryReader, renderer *emailmessage.Renderer,
	send func(context.Context, identity.Email, string, string) mailtransport.Outcome,
) (*Attempt, error) {
	if reader == nil || renderer == nil || send == nil {
		return nil, identity.ErrInvalid
	}
	return &Attempt{reader: reader, renderer: renderer, send: send}, nil
}

// Run performs at most one SMTP attempt. The caller must own the job and
// enforce attempt counts and shared admission. No database transaction or
// connection is retained during SMTP. Run does not acknowledge the job.
func (a *Attempt) Run(ctx context.Context, payloadID string) Outcome {
	if a == nil || a.reader == nil || ctx == nil || !identity.ValidID(payloadID) {
		return Rejected
	}
	if ctx.Err() != nil {
		return Retry
	}
	delivery, err := a.reader.Prepare(ctx, payloadID)
	if errors.Is(err, identity.ErrConflict) {
		return Obsolete
	}
	if err != nil {
		return Retry
	}
	scope := delivery.Scope()
	message, err := a.renderer.Render(scope.Recipient, scope.Purpose, delivery.Token())
	if err != nil {
		return Rejected
	}
	// The database evaluated the window at read time. Keep the transport
	// bounded even if time elapsed between that read and starting SMTP.
	sendCtx, cancel := context.WithDeadline(ctx, delivery.SendDeadline())
	defer cancel()
	if ctx.Err() != nil {
		return Retry
	}
	if sendCtx.Err() != nil {
		return Obsolete
	}
	switch a.send(sendCtx, message.Recipient(), message.Subject(), message.Text()) {
	case mailtransport.Accepted:
		return Accepted
	case mailtransport.TemporaryFailure:
		return Retry
	case mailtransport.Unknown:
		return Unknown
	default:
		return Rejected
	}
}
