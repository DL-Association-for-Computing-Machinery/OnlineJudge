-- name: InitializePersonalEmail :one
INSERT INTO identity_personal_emails (subject_id, email_key, original_email)
VALUES (@subject_id, @email_key, @original_email)
ON CONFLICT DO NOTHING
RETURNING subject_id, email_key, original_email, email_verified;

-- name: PersonalByID :one
SELECT c.id, c.enabled, c.credential_version, c.auth_generation,
       e.email_key, e.original_email, e.email_verified
FROM identity_personal_emails e JOIN identity_credentials c ON c.id = e.subject_id
WHERE e.subject_id = @id;

-- name: PersonalByEmail :one
SELECT c.id, c.enabled, c.credential_version, c.auth_generation,
       e.email_key, e.original_email, e.email_verified
FROM identity_personal_emails e JOIN identity_credentials c ON c.id = e.subject_id
WHERE e.email_key = @email_key;
