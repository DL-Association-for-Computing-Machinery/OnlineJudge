-- name: DeleteEmailPayload :exec
DELETE FROM identity_email_payloads WHERE token_id = @payload_id;
