CREATE TABLE booking_sessions (
    id                uuid PRIMARY KEY,
    status            varchar(16) NOT NULL CHECK (status IN ('collecting', 'readyToConfirm', 'confirmed', 'expired')),
    service_id        uuid REFERENCES services(id) ON DELETE RESTRICT,
    professional_id   uuid REFERENCES professionals(id) ON DELETE RESTRICT,
    slot_start_at     timestamptz,
    slot_end_at       timestamptz,
    slot_time_zone    varchar(64),
    patient_name      varchar(100),
    patient_email     varchar(254),
    version           bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    expires_at        timestamptz NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    CHECK (
        (slot_start_at IS NULL AND slot_end_at IS NULL)
        OR (slot_start_at IS NOT NULL AND slot_end_at IS NOT NULL AND slot_end_at > slot_start_at)
    )
);

CREATE INDEX idx_booking_sessions_expires_at ON booking_sessions (expires_at);
