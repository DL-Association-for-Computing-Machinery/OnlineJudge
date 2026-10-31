-- name: Initialize :one
INSERT INTO identity_credentials
    (id, enabled, format_version, algorithm, argon_version, memory_kib, iterations,
     parallelism, salt_bytes, digest_bytes, salt, digest)
VALUES (@id, @enabled, @format_version, @algorithm, @argon_version, @memory_kib, @iterations,
        @parallelism, @salt_bytes, @digest_bytes, @salt, @digest)
ON CONFLICT (id) DO NOTHING
RETURNING *;

-- name: Get :one
SELECT * FROM identity_credentials WHERE id = @id;

-- name: GetState :one
SELECT id, enabled, credential_version, auth_generation FROM identity_credentials WHERE id = @id;

-- name: RehashCAS :one
UPDATE identity_credentials
SET credential_version = credential_version + 1,
    format_version = @format_version, algorithm = @algorithm, argon_version = @argon_version,
    memory_kib = @memory_kib, iterations = @iterations, parallelism = @parallelism,
    salt_bytes = @salt_bytes, digest_bytes = @digest_bytes, salt = @salt, digest = @digest
WHERE id = @id AND enabled
  AND credential_version = @expected_credential_version AND auth_generation = @expected_auth_generation
  AND credential_version < 9223372036854775807
RETURNING *;

-- name: ReplacePasswordCAS :one
UPDATE identity_credentials
SET credential_version = credential_version + 1, auth_generation = auth_generation + 1,
    format_version = @format_version, algorithm = @algorithm, argon_version = @argon_version,
    memory_kib = @memory_kib, iterations = @iterations, parallelism = @parallelism,
    salt_bytes = @salt_bytes, digest_bytes = @digest_bytes, salt = @salt, digest = @digest
WHERE id = @id
  AND credential_version = @expected_credential_version AND auth_generation = @expected_auth_generation
  AND credential_version < 9223372036854775807 AND auth_generation < 9223372036854775807
RETURNING *;

-- name: DisableCAS :one
UPDATE identity_credentials SET enabled = false, auth_generation = auth_generation + 1
WHERE id = @id AND enabled
  AND credential_version = @expected_credential_version AND auth_generation = @expected_auth_generation
  AND auth_generation < 9223372036854775807
RETURNING credential_version, auth_generation;
