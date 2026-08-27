package scheduling

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"backend/internal/model"
)

// requireTestDB opens TEST_DATABASE_URL, applies the Catalog and
// Scheduling migrations (the real, reviewed migrations — never GORM
// AutoMigrate), and tears the schema down after the test. Skips entirely
// when TEST_DATABASE_URL is unset.
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

	execMigrationFile(t, db, "000001_create_catalog_tables.up.sql")
	execMigrationFile(t, db, "000003_create_scheduling_tables.up.sql")
	t.Cleanup(func() {
		execMigrationFile(t, db, "000003_create_scheduling_tables.down.sql")
		execMigrationFile(t, db, "000001_create_catalog_tables.down.sql")
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

func TestRepository_GetHoursForDay(t *testing.T) {
	db := requireTestDB(t)
	openTime, closeTime := "09:00:00", "17:00:00"
	hours := model.ClinicHours{DayOfWeek: 2, IsOpen: true, OpenTime: &openTime, CloseTime: &closeTime}
	if err := db.Create(&hours).Error; err != nil {
		t.Fatalf("seed clinic hours: %v", err)
	}
	repo := NewRepository(db)

	t.Run("returns configured day", func(t *testing.T) {
		got, err := repo.GetHoursForDay(context.Background(), 2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil || !got.IsOpen {
			t.Fatalf("expected open hours for day 2, got %+v", got)
		}
	})

	t.Run("returns nil for an unconfigured day, not an error", func(t *testing.T) {
		got, err := repo.GetHoursForDay(context.Background(), 3)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Fatalf("expected nil for unconfigured day, got %+v", got)
		}
	})
}

func TestRepository_IsClosureDate(t *testing.T) {
	db := requireTestDB(t)
	if err := db.Create(&model.ClinicClosure{ClosureDate: "2026-12-25"}).Error; err != nil {
		t.Fatalf("seed closure: %v", err)
	}
	repo := NewRepository(db)

	closed, err := repo.IsClosureDate(context.Background(), "2026-12-25")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !closed {
		t.Fatal("expected 2026-12-25 to be a closure date")
	}

	open, err := repo.IsClosureDate(context.Background(), "2026-12-26")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if open {
		t.Fatal("expected 2026-12-26 to not be a closure date")
	}
}

func TestRepository_ListOverlapping_BlockedSlots(t *testing.T) {
	db := requireTestDB(t)
	professional := model.Professional{ID: "00000000-0000-0000-0000-000000000011", Code: "SENIOR_1", DisplayName: "Senior 1", IsActive: true}
	if err := db.Create(&professional).Error; err != nil {
		t.Fatalf("seed professional: %v", err)
	}
	blocked := model.ProfessionalBlockedSlot{
		ID: "00000000-0000-0000-0000-000000000021", ProfessionalID: professional.ID,
		StartAt: time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC),
		EndAt:   time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC),
	}
	if err := db.Create(&blocked).Error; err != nil {
		t.Fatalf("seed blocked slot: %v", err)
	}
	repo := NewRepository(db)

	got, err := repo.ListOverlapping(context.Background(), professional.ID,
		time.Date(2026, 9, 2, 9, 30, 0, 0, time.UTC), time.Date(2026, 9, 2, 11, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected the overlapping blocked slot, got %+v", got)
	}

	none, err := repo.ListOverlapping(context.Background(), professional.ID,
		time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC), time.Date(2026, 9, 2, 11, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("expected no overlap for a half-open adjacent window, got %+v", none)
	}
}
