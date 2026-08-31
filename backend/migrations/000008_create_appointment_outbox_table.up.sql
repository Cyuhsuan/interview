CREATE TABLE appointment_outbox (
    id               uuid PRIMARY KEY,
    appointment_id   uuid NOT NULL REFERENCES appointments(id) ON DELETE RESTRICT,
    provider         varchar(16) NOT NULL CHECK (provider IN ('google', 'microsoft')),
    status           varchar(16) NOT NULL CHECK (status IN ('pending', 'processing', 'retryable', 'delivered', 'dead_letter')),
    idempotency_key  varchar(128) NOT NULL,
    attempt_count    integer NOT NULL DEFAULT 0,
    next_attempt_at  timestamptz NOT NULL DEFAULT now(),
    event_reference  varchar(512),
    last_error       varchar(500),
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (appointment_id, provider),
    UNIQUE (idempotency_key)
);

CREATE INDEX idx_appointment_outbox_due
    ON appointment_outbox (next_attempt_at)
    WHERE status IN ('pending', 'retryable');
