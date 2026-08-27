package booking

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"backend/internal/model"
	bookingsvc "backend/internal/service/booking"
)

// requireTestDB opens TEST_DATABASE_URL, applies every migration up to and
// including the Booking module's tables (the real, reviewed migrations —
// never GORM AutoMigrate), and tears the schema down after the test. Skips
// entirely when TEST_DATABASE_URL is unset.
func requireTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping repository contract test")
	}

	db, err := gorm.Open(postgres.Open(url), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}

	upFiles := []string{
		"000001_create_catalog_tables.up.sql",
		"000004_create_booking_session_table.up.sql",
		"000005_create_appointment_tables.up.sql",
		"000006_create_appointment_support_tables.up.sql",
	}
	downFiles := []string{
		"000006_create_appointment_support_tables.down.sql",
		"000005_create_appointment_tables.down.sql",
		"000004_create_booking_session_table.down.sql",
		"000001_create_catalog_tables.down.sql",
	}

	for _, name := range upFiles {
		execMigrationFile(t, db, name)
	}
	t.Cleanup(func() {
		for _, name := range downFiles {
			execMigrationFile(t, db, name)
		}
	})

	return db
}

func execMigrationFile(t *testing.T, db *gorm.DB, name string) {
	t.Helper()

	path := filepath.Join("..", "..", "..", "migrations", name)
	sqlBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration file %s: %v", path, err)
	}
	if err := db.Exec(string(sqlBytes)).Error; err != nil {
		t.Fatalf("apply migration file %s: %v", path, err)
	}
}

func seedCatalogFixtures(t *testing.T, db *gorm.DB) (serviceID, professionalID string) {
	t.Helper()

	service := model.Service{ID: "00000000-0000-0000-0000-000000000001", Code: "A", DisplayName: "Service A", DurationMinutes: 60, IsActive: true}
	if err := db.Create(&service).Error; err != nil {
		t.Fatalf("seed service: %v", err)
	}
	professional := model.Professional{ID: "00000000-0000-0000-0000-000000000011", Code: "SENIOR_1", DisplayName: "Senior 1", IsActive: true}
	if err := db.Create(&professional).Error; err != nil {
		t.Fatalf("seed professional: %v", err)
	}
	return service.ID, professional.ID
}

func seedBookingSession(t *testing.T, db *gorm.DB, id string) {
	t.Helper()
	session := model.BookingSession{
		ID: id, Status: model.BookingSessionStatusReadyToConfirm, Version: 1,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("seed booking session: %v", err)
	}
}

func TestRepository_InsertAppointment_ExclusionConstraintRejectsOverlap(t *testing.T) {
	db := requireTestDB(t)
	serviceID, professionalID := seedCatalogFixtures(t, db)
	seedBookingSession(t, db, "00000000-0000-0000-0000-000000000101")
	seedBookingSession(t, db, "00000000-0000-0000-0000-000000000102")

	repo := NewRepository(db)
	err := repo.WithTx(context.Background(), func(tx bookingsvc.TxRepository) error {
		start := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
		first := model.Appointment{
			ID: "00000000-0000-0000-0000-000000000201", BookingSessionID: "00000000-0000-0000-0000-000000000101", ServiceID: serviceID, ProfessionalID: professionalID,
			PatientName: "Jane Doe", PatientEmail: "jane@example.com",
			StartAt: start, EndAt: start.Add(60 * time.Minute), TimeZone: "UTC",
			Status: model.AppointmentStatusConfirmed,
		}
		if err := tx.InsertAppointment(context.Background(), first); err != nil {
			t.Fatalf("insert first appointment: %v", err)
		}

		overlapping := first
		overlapping.ID = "00000000-0000-0000-0000-000000000202"
		overlapping.BookingSessionID = "00000000-0000-0000-0000-000000000102"
		overlapping.StartAt = start.Add(30 * time.Minute)
		overlapping.EndAt = overlapping.StartAt.Add(60 * time.Minute)

		err := tx.InsertAppointment(context.Background(), overlapping)
		if !errors.Is(err, bookingsvc.ErrSlotNoLongerAvailable) {
			t.Fatalf("expected ErrSlotNoLongerAvailable for overlapping insert, got %v", err)
		}
		return err
	})
	if !errors.Is(err, bookingsvc.ErrSlotNoLongerAvailable) {
		t.Fatalf("expected transaction to surface ErrSlotNoLongerAvailable, got %v", err)
	}

	var count int64
	db.Model(&model.Appointment{}).Count(&count)
	if count != 0 {
		t.Fatalf("expected rollback to leave no appointments, got %d", count)
	}
}

func TestRepository_InsertAppointment_NonOverlappingSucceeds(t *testing.T) {
	db := requireTestDB(t)
	serviceID, professionalID := seedCatalogFixtures(t, db)
	seedBookingSession(t, db, "00000000-0000-0000-0000-000000000101")
	seedBookingSession(t, db, "00000000-0000-0000-0000-000000000102")

	repo := NewRepository(db)
	start := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	err := repo.WithTx(context.Background(), func(tx bookingsvc.TxRepository) error {
		first := model.Appointment{
			ID: "00000000-0000-0000-0000-000000000201", BookingSessionID: "00000000-0000-0000-0000-000000000101", ServiceID: serviceID, ProfessionalID: professionalID,
			PatientName: "Jane Doe", PatientEmail: "jane@example.com",
			StartAt: start, EndAt: start.Add(60 * time.Minute), TimeZone: "UTC",
			Status: model.AppointmentStatusConfirmed,
		}
		if err := tx.InsertAppointment(context.Background(), first); err != nil {
			return err
		}
		second := first
		second.ID = "00000000-0000-0000-0000-000000000202"
		second.BookingSessionID = "00000000-0000-0000-0000-000000000102"
		second.StartAt = start.Add(60 * time.Minute)
		second.EndAt = second.StartAt.Add(60 * time.Minute)
		return tx.InsertAppointment(context.Background(), second)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var count int64
	db.Model(&model.Appointment{}).Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 appointments, got %d", count)
	}
}

func TestRepository_IdempotencyKey_UniquePrimaryKey(t *testing.T) {
	db := requireTestDB(t)
	seedCatalogFixtures(t, db)
	seedBookingSession(t, db, "00000000-0000-0000-0000-000000000101")

	repo := NewRepository(db)
	err := repo.WithTx(context.Background(), func(tx bookingsvc.TxRepository) error {
		rec := model.AppointmentIdempotencyKey{
			Key: "idem-key-0123456789", RequestHash: "hash-1",
			ResponseStatus: 201, ResponseBody: []byte(`{}`), CreatedAt: time.Now(),
		}
		if err := tx.InsertIdempotencyKey(context.Background(), rec); err != nil {
			t.Fatalf("insert first idempotency key: %v", err)
		}
		return tx.InsertIdempotencyKey(context.Background(), rec)
	})
	if err == nil {
		t.Fatal("expected duplicate idempotency key insert to fail")
	}
}
