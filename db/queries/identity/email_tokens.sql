-- name: EmailForTokenIssue :one
SELECT email_key, original_email, email_verified
FROM identity_personal_emails WHERE subject_id = @subject_id FOR UPDATE;

-- name: EmailIssueAccount :one
SELECT enabled, auth_generation FROM identity_credentials WHERE id = @id FOR UPDATE;

-- name: EmailIssueTime :one
SELECT clock_timestamp()::timestamptz AS issued_at;

-- name: EmailRecentIssue :one
SELECT EXISTS (
    SELECT 1 FROM identity_email_tokens
    WHERE subject_id = @subject_id AND purpose = @purpose AND issued_at > @cutoff
) AS recent;

-- name: EmailLiveTokenCount :one
SELECT count(*) FROM (
    SELECT 1 FROM identity_email_tokens
    WHERE subject_id = @subject_id AND purpose = @purpose AND recovery_id = @recovery_id
      AND auth_generation = @auth_generation AND expires_at > @issued_at
    LIMIT 3
) AS live;

-- name: InsertEmailToken :exec
INSERT INTO identity_email_tokens (id, subject_id, purpose, recovery_id, auth_generation, digest, issued_at, expires_at)
VALUES (@id, @subject_id, @purpose, @recovery_id, @auth_generation, @digest, @issued_at, @expires_at);

-- name: InsertEmailPayload :exec
INSERT INTO identity_email_payloads (token_id, key_id, ciphertext, send_until)
VALUES (@token_id, @key_id, @ciphertext, @send_until);
