package model

import "time"

type ClinicHours struct {
	DayOfWeek int16   `gorm:"column:day_of_week;primaryKey"`
	IsOpen    bool    `gorm:"column:is_open;not null"`
	OpenTime  *string `gorm:"column:open_time;type:time"`
	CloseTime *string `gorm:"column:close_time;type:time"`
}

func (ClinicHours) TableName() string { return "clinic_hours" }

// ClinicClosure uses closure_date (not a UUID) as its natural key: a
// single-day clinic closure is inherently identified by its calendar date.
type ClinicClosure struct {
	ClosureDate string  `gorm:"column:closure_date;primaryKey;type:date"`
	Reason      *string `gorm:"column:reason;size:200"`
}

func (ClinicClosure) TableName() string { return "clinic_closures" }

type ProfessionalBlockedSlot struct {
	ID             string    `gorm:"column:id;primaryKey;type:uuid"`
	ProfessionalID string    `gorm:"column:professional_id;type:uuid;not null"`
	StartAt        time.Time `gorm:"column:start_at;not null"`
	EndAt          time.Time `gorm:"column:end_at;not null"`
	Reason         *string   `gorm:"column:reason;size:200"`
	CreatedAt      time.Time `gorm:"column:created_at;not null"`
}

func (ProfessionalBlockedSlot) TableName() string { return "professional_blocked_slots" }
