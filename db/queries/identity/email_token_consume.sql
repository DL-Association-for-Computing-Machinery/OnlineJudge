-- name: EmailTokenIdentity :one
SELECT id, subject_id, auth_generation, recovery_id
FROM identity_email_tokens
WHERE digest = @digest AND purpose = @purpose;

-- name: EmailTokenCurrent :one
SELECT EXISTS (
    SELECT 1 FROM identity_email_tokens
    WHERE id = @id AND subject_id = @subject_id AND purpose = @purpose
      AND digest = @digest AND auth_generation = @auth_generation AND recovery_id = @recovery_id
      AND issued_at <= @checked_at AND expires_at > @checked_at
) AS current;

-- name: MarkEmailVerified :execrows
UPDATE identity_personal_emails SET email_verified = true
WHERE subject_id = @subject_id AND NOT email_verified;

-- name: DeleteLiveEmailTokenPayloads :exec
DELETE FROM identity_email_payloads p
USING identity_email_tokens t
WHERE p.token_id = t.id AND t.subject_id = @subject_id AND t.purpose = @purpose
  AND t.auth_generation = @auth_generation AND t.recovery_id = @recovery_id
  AND t.expires_at > @checked_at;

-- name: EmailTokenConsumeAccount :one
SELECT enabled, auth_generation, credential_version FROM identity_credentials WHERE id = @id FOR UPDATE;
