CREATE TABLE clinic_hours (
    day_of_week  smallint PRIMARY KEY CHECK (day_of_week BETWEEN 0 AND 6),
    is_open      boolean NOT NULL,
    open_time    time,
    close_time   time,
    CHECK (
        (is_open = false AND open_time IS NULL AND close_time IS NULL)
        OR (is_open = true AND open_time IS NOT NULL AND close_time IS NOT NULL AND close_time > open_time)
    )
);

CREATE TABLE clinic_closures (
    closure_date  date PRIMARY KEY,
    reason        varchar(200)
);

CREATE TABLE professional_blocked_slots (
    id                uuid PRIMARY KEY,
    professional_id   uuid NOT NULL REFERENCES professionals(id) ON DELETE RESTRICT,
    start_at          timestamptz NOT NULL,
    end_at            timestamptz NOT NULL CHECK (end_at > start_at),
    reason            varchar(200),
    created_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_professional_blocked_slots_professional_id
    ON professional_blocked_slots (professional_id);
