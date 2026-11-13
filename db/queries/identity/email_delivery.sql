-- name: EmailPayloadForDelivery :one
WITH cutoff AS (SELECT clock_timestamp()::timestamptz AS checked_at)
SELECT t.id, t.subject_id, t.purpose, t.recovery_id, t.auth_generation,
       t.digest, t.issued_at, t.expires_at, p.key_id, p.ciphertext, p.send_until,
       e.original_email, e.email_key, cutoff.checked_at
FROM identity_email_payloads p
JOIN identity_email_tokens t ON t.id = p.token_id
JOIN identity_credentials c ON c.id = t.subject_id
JOIN identity_personal_emails e ON e.subject_id = t.subject_id
JOIN identity_session_recovery r ON r.singleton
CROSS JOIN cutoff
WHERE p.token_id = @payload_id
  AND c.enabled AND c.auth_generation = t.auth_generation
  AND r.enabled AND r.generation = t.recovery_id
  AND e.email_verified = (t.purpose = 'password-recovery')
  AND t.issued_at <= cutoff.checked_at AND t.expires_at > cutoff.checked_at
  AND p.send_until > cutoff.checked_at;
