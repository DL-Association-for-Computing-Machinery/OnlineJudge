-- +goose Up
SET LOCAL lock_timeout = '5s';
CREATE TABLE account_mail_send_rate (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    spacing_ms integer NOT NULL CHECK (spacing_ms BETWEEN 100 AND 60000),
    next_permit_at timestamptz NOT NULL DEFAULT '-infinity'
);

-- +goose Down
SET LOCAL lock_timeout = '5s';
DROP TABLE account_mail_send_rate;
