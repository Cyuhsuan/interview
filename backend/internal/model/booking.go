package model

import "time"

const (
	BookingSessionStatusCollecting     = "collecting"
	BookingSessionStatusReadyToConfirm = "readyToConfirm"
	BookingSessionStatusConfirmed      = "confirmed"
	BookingSessionStatusExpired        = "expired"
	AppointmentStatusConfirmed         = "confirmed"
)

type BookingSession struct {
	ID             string     `gorm:"column:id;primaryKey;type:uuid"`
	Status         string     `gorm:"column:status;size:16;not null"`
	ServiceID      *string    `gorm:"column:service_id;type:uuid"`
	ProfessionalID *string    `gorm:"column:professional_id;type:uuid"`
	SlotStartAt    *time.Time `gorm:"column:slot_start_at"`
	SlotEndAt      *time.Time `gorm:"column:slot_end_at"`
	SlotTimeZone   *string    `gorm:"column:slot_time_zone;size:64"`
	PatientName    *string    `gorm:"column:patient_name;size:100"`
	PatientEmail   *string    `gorm:"column:patient_email;size:254"`
	Version        int64      `gorm:"column:version;not null"`
	ExpiresAt      time.Time  `gorm:"column:expires_at;not null"`
	CreatedAt      time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt      time.Time  `gorm:"column:updated_at;not null"`
}

func (BookingSession) TableName() string { return "booking_sessions" }

type Appointment struct {
	ID               string    `gorm:"column:id;primaryKey;type:uuid"`
	BookingSessionID string    `gorm:"column:booking_session_id;type:uuid;not null"`
	ServiceID        string    `gorm:"column:service_id;type:uuid;not null"`
	ProfessionalID   string    `gorm:"column:professional_id;type:uuid;not null"`
	PatientName      string    `gorm:"column:patient_name;size:100;not null"`
	PatientEmail     string    `gorm:"column:patient_email;size:254;not null"`
	StartAt          time.Time `gorm:"column:start_at;not null"`
	EndAt            time.Time `gorm:"column:end_at;not null"`
	TimeZone         string    `gorm:"column:time_zone;size:64;not null"`
	Status           string    `gorm:"column:status;size:16;not null"`
	CreatedAt        time.Time `gorm:"column:created_at;not null"`
	UpdatedAt        time.Time `gorm:"column:updated_at;not null"`
}

func (Appointment) TableName() string { return "appointments" }

// AppointmentIdempotencyKey uses the caller-supplied Idempotency-Key header
// value as its natural key, per README's Canonical Types idempotency rules.
type AppointmentIdempotencyKey struct {
	Key            string    `gorm:"column:key;primaryKey;size:128"`
	RequestHash    string    `gorm:"column:request_hash;size:64;not null"`
	AppointmentID  *string   `gorm:"column:appointment_id;type:uuid"`
	ResponseStatus int16     `gorm:"column:response_status;not null"`
	ResponseBody   []byte    `gorm:"column:response_body;type:jsonb;not null"`
	CreatedAt      time.Time `gorm:"column:created_at;not null"`
}

func (AppointmentIdempotencyKey) TableName() string { return "appointment_idempotency_keys" }

// AppointmentAuditLog only stores entity ID, action, actor ID and
// timestamp, per README's security baseline on log/audit content.
type AppointmentAuditLog struct {
	ID        string    `gorm:"column:id;primaryKey;type:uuid"`
	EntityID  string    `gorm:"column:entity_id;type:uuid;not null"`
	Action    string    `gorm:"column:action;size:64;not null"`
	ActorID   string    `gorm:"column:actor_id;size:128;not null"`
	CreatedAt time.Time `gorm:"column:created_at;not null"`
}

func (AppointmentAuditLog) TableName() string { return "appointment_audit_log" }
