CREATE TABLE appointment_idempotency_keys (
    key               varchar(128) PRIMARY KEY,
    request_hash      varchar(64) NOT NULL,
    appointment_id    uuid REFERENCES appointments(id) ON DELETE RESTRICT,
    response_status   smallint NOT NULL,
    response_body     jsonb NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE appointment_audit_log (
    id           uuid PRIMARY KEY,
    entity_id    uuid NOT NULL,
    action       varchar(64) NOT NULL,
    actor_id     varchar(128) NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_appointment_audit_log_entity_id ON appointment_audit_log (entity_id);
