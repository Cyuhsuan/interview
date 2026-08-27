package model

import "time"

type Service struct {
	ID              string    `gorm:"column:id;primaryKey;type:uuid"`
	Code            string    `gorm:"column:code;unique;size:32;not null"`
	DisplayName     string    `gorm:"column:display_name;size:100;not null"`
	DurationMinutes int16     `gorm:"column:duration_minutes;not null"`
	IsActive        bool      `gorm:"column:is_active;not null"`
	CreatedAt       time.Time `gorm:"column:created_at;not null"`
	UpdatedAt       time.Time `gorm:"column:updated_at;not null"`
}

func (Service) TableName() string { return "services" }

type Professional struct {
	ID          string    `gorm:"column:id;primaryKey;type:uuid"`
	Code        string    `gorm:"column:code;unique;size:32;not null"`
	DisplayName string    `gorm:"column:display_name;size:100;not null"`
	IsActive    bool      `gorm:"column:is_active;not null"`
	CreatedAt   time.Time `gorm:"column:created_at;not null"`
	UpdatedAt   time.Time `gorm:"column:updated_at;not null"`
}

func (Professional) TableName() string { return "professionals" }

type ProfessionalServiceQualification struct {
	ProfessionalID string    `gorm:"column:professional_id;primaryKey;type:uuid"`
	ServiceID      string    `gorm:"column:service_id;primaryKey;type:uuid"`
	CreatedAt      time.Time `gorm:"column:created_at;not null"`
}

func (ProfessionalServiceQualification) TableName() string {
	return "professional_service_qualifications"
}
