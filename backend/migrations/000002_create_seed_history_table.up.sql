CREATE TABLE seed_history (
    version      varchar(32) PRIMARY KEY,
    checksum     varchar(64) NOT NULL,
    executed_at  timestamptz NOT NULL DEFAULT now(),
    executor_id  varchar(128) NOT NULL
);
