CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE appointments (
    id                    uuid PRIMARY KEY,
    booking_session_id    uuid NOT NULL UNIQUE REFERENCES booking_sessions(id) ON DELETE RESTRICT,
    service_id            uuid NOT NULL REFERENCES services(id) ON DELETE RESTRICT,
    professional_id       uuid NOT NULL REFERENCES professionals(id) ON DELETE RESTRICT,
    patient_name          varchar(100) NOT NULL,
    patient_email         varchar(254) NOT NULL,
    start_at              timestamptz NOT NULL,
    end_at                timestamptz NOT NULL CHECK (end_at > start_at),
    time_zone             varchar(64) NOT NULL,
    status                varchar(16) NOT NULL CHECK (status IN ('confirmed')),
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    EXCLUDE USING gist (
        professional_id WITH =,
        tstzrange(start_at, end_at, '[)') WITH &&
    ) WHERE (status = 'confirmed')
);

CREATE INDEX idx_appointments_professional_id ON appointments (professional_id);
