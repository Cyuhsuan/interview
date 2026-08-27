CREATE TABLE services (
    id                uuid PRIMARY KEY,
    code              varchar(32) NOT NULL UNIQUE,
    display_name      varchar(100) NOT NULL,
    duration_minutes  smallint NOT NULL CHECK (duration_minutes > 0),
    is_active         boolean NOT NULL DEFAULT true,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE professionals (
    id                uuid PRIMARY KEY,
    code              varchar(32) NOT NULL UNIQUE,
    display_name      varchar(100) NOT NULL,
    is_active         boolean NOT NULL DEFAULT true,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE professional_service_qualifications (
    professional_id  uuid NOT NULL REFERENCES professionals(id) ON DELETE RESTRICT,
    service_id       uuid NOT NULL REFERENCES services(id) ON DELETE RESTRICT,
    created_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (professional_id, service_id)
);

CREATE INDEX idx_professional_service_qualifications_service_id
    ON professional_service_qualifications (service_id);
