package model

import "time"

// SeedHistory is a technical/audit table whose primary key is the seed
// artifact's natural key (version), per README's Canonical Types carve-out
// for tables like seed_history and idempotency records.
type SeedHistory struct {
	Version    string    `gorm:"column:version;primaryKey;size:32"`
	Checksum   string    `gorm:"column:checksum;size:64;not null"`
	ExecutedAt time.Time `gorm:"column:executed_at;not null"`
	ExecutorID string    `gorm:"column:executor_id;size:128;not null"`
}

func (SeedHistory) TableName() string { return "seed_history" }
