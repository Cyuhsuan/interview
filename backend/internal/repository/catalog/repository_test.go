package catalog

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"backend/internal/model"
)

// requireTestDB opens a connection to TEST_DATABASE_URL, applies the
// Catalog migration SQL files directly (the real, reviewed migration —
// never GORM AutoMigrate), and tears the schema down after the test.
// Skips the test entirely when TEST_DATABASE_URL is unset, per this
// project's contract-test convention for adapters that need a live
// Postgres.
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
	t.Cleanup(func() {
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

func seedCatalogFixtures(t *testing.T, db *gorm.DB) {
	t.Helper()

	services := []model.Service{
		{ID: "00000000-0000-0000-0000-000000000001", Code: "A", DisplayName: "Service A", DurationMinutes: 60, IsActive: true},
		{ID: "00000000-0000-0000-0000-000000000002", Code: "B", DisplayName: "Service B", DurationMinutes: 90, IsActive: false},
	}
	if err := db.Create(&services).Error; err != nil {
		t.Fatalf("seed services: %v", err)
	}

	professionals := []model.Professional{
		{ID: "00000000-0000-0000-0000-000000000011", Code: "SENIOR_1", DisplayName: "Senior 1", IsActive: true},
		{ID: "00000000-0000-0000-0000-000000000012", Code: "JUNIOR_INACTIVE", DisplayName: "Junior Inactive", IsActive: false},
	}
	if err := db.Create(&professionals).Error; err != nil {
		t.Fatalf("seed professionals: %v", err)
	}

	qualifications := []model.ProfessionalServiceQualification{
		{ProfessionalID: professionals[0].ID, ServiceID: services[0].ID}, // active pro, active service
		{ProfessionalID: professionals[1].ID, ServiceID: services[0].ID}, // inactive pro, active service
		{ProfessionalID: professionals[0].ID, ServiceID: services[1].ID}, // active pro, inactive service
	}
	if err := db.Create(&qualifications).Error; err != nil {
		t.Fatalf("seed qualifications: %v", err)
	}
}

func TestRepository_ListActiveServices_ExcludesInactive(t *testing.T) {
	db := requireTestDB(t)
	seedCatalogFixtures(t, db)
	repo := NewRepository(db)

	got, err := repo.ListActiveServices(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Code != "A" {
		t.Fatalf("expected only active service A, got %+v", got)
	}
}

func TestRepository_ListActiveProfessionalsByServiceCode(t *testing.T) {
	db := requireTestDB(t)
	seedCatalogFixtures(t, db)
	repo := NewRepository(db)

	t.Run("returns only active professional qualified for an active service", func(t *testing.T) {
		got, err := repo.ListActiveProfessionalsByServiceCode(context.Background(), "A")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0].Code != "SENIOR_1" {
			t.Fatalf("expected only SENIOR_1, got %+v", got)
		}
	})

	t.Run("excludes qualifications tied to an inactive service", func(t *testing.T) {
		got, err := repo.ListActiveProfessionalsByServiceCode(context.Background(), "B")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("expected empty result for inactive service, got %+v", got)
		}
	})

	t.Run("unknown service code yields empty result, not an error", func(t *testing.T) {
		got, err := repo.ListActiveProfessionalsByServiceCode(context.Background(), "UNKNOWN")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("expected empty result for unknown service code, got %+v", got)
		}
	})
}
