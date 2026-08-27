package seed

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"backend/internal/model"
	seedsvc "backend/internal/service/seed"
)

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
	execMigrationFile(t, db, "000002_create_seed_history_table.up.sql")
	t.Cleanup(func() {
		execMigrationFile(t, db, "000002_create_seed_history_table.down.sql")
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

func tableCounts(t *testing.T, db *gorm.DB) (services, professionals, qualifications, history int64) {
	t.Helper()
	db.Model(&model.Service{}).Count(&services)
	db.Model(&model.Professional{}).Count(&professionals)
	db.Model(&model.ProfessionalServiceQualification{}).Count(&qualifications)
	db.Model(&model.SeedHistory{}).Count(&history)
	return
}

func testArtifact() seedsvc.Artifact {
	return seedsvc.Artifact{
		Version: "1",
		Services: []seedsvc.ServiceSeed{
			{Code: "A", DisplayName: "Service A", DurationMinutes: 60},
			{Code: "B", DisplayName: "Service B", DurationMinutes: 60},
			{Code: "C", DisplayName: "Service C", DurationMinutes: 150},
			{Code: "D", DisplayName: "Service D", DurationMinutes: 120},
			{Code: "E", DisplayName: "Service E", DurationMinutes: 360},
		},
		Professionals: []seedsvc.ProfessionalSeed{
			{Code: "JUNIOR", DisplayName: "Junior", Qualifications: []string{"A", "B"}},
			{Code: "SENIOR_1", DisplayName: "Senior 1", Qualifications: []string{"A", "B", "C", "D", "E"}},
			{Code: "SENIOR_2", DisplayName: "Senior 2", Qualifications: []string{"A", "B", "C", "D", "E"}},
		},
	}
}

type fakeIDGenerator struct{ n int }

func (g *fakeIDGenerator) NewID() (string, error) {
	g.n++
	return "00000000-0000-0000-0000-" + paddedHex(g.n), nil
}

func paddedHex(n int) string {
	const hex = "0123456789abcdef"
	digits := make([]byte, 12)
	for i := 11; i >= 0; i-- {
		digits[i] = hex[n%16]
		n /= 16
	}
	return string(digits)
}

func TestRepository_Apply_FirstRun(t *testing.T) {
	db := requireTestDB(t)
	seeder := seedsvc.NewSeeder(NewRepository(db), &fakeIDGenerator{})

	result, err := seeder.Apply(context.Background(), testArtifact(), "checksum-1", "tester")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Applied || result.ServicesCreated != 5 || result.ProfessionalsCreated != 3 || result.QualificationsCreated != 12 {
		t.Fatalf("unexpected result: %+v", result)
	}

	services, professionals, qualifications, history := tableCounts(t, db)
	if services != 5 || professionals != 3 || qualifications != 12 || history != 1 {
		t.Fatalf("unexpected row counts: services=%d professionals=%d qualifications=%d history=%d",
			services, professionals, qualifications, history)
	}
}

func TestRepository_Apply_RerunIsNoOp(t *testing.T) {
	db := requireTestDB(t)
	seeder := seedsvc.NewSeeder(NewRepository(db), &fakeIDGenerator{})

	if _, err := seeder.Apply(context.Background(), testArtifact(), "checksum-1", "tester"); err != nil {
		t.Fatalf("first apply failed: %v", err)
	}
	before := countsSnapshot(t, db)

	result, err := seeder.Apply(context.Background(), testArtifact(), "checksum-1", "tester")
	if err != nil {
		t.Fatalf("unexpected error on rerun: %v", err)
	}
	if result.Applied {
		t.Fatalf("expected no-op, got %+v", result)
	}

	after := countsSnapshot(t, db)
	if before != after {
		t.Fatalf("expected no new rows on rerun: before=%+v after=%+v", before, after)
	}
}

func TestRepository_Apply_StaticFieldMismatch_RollsBackAllTables(t *testing.T) {
	db := requireTestDB(t)

	if err := db.Exec(
		`INSERT INTO services (id, code, display_name, duration_minutes, is_active) VALUES (gen_random_uuid(), 'A', 'Wrong Name', 60, true)`,
	).Error; err != nil {
		// pgcrypto's gen_random_uuid may not be available; fall back to a literal uuid.
		if err2 := db.Exec(
			`INSERT INTO services (id, code, display_name, duration_minutes, is_active) VALUES ('11111111-1111-1111-1111-111111111111', 'A', 'Wrong Name', 60, true)`,
		).Error; err2 != nil {
			t.Fatalf("seed conflicting service row: %v / %v", err, err2)
		}
	}

	before := countsSnapshot(t, db)

	seeder := seedsvc.NewSeeder(NewRepository(db), &fakeIDGenerator{})
	_, err := seeder.Apply(context.Background(), testArtifact(), "checksum-1", "tester")
	if err == nil {
		t.Fatal("expected an error due to static field mismatch")
	}

	after := countsSnapshot(t, db)
	if after.services != before.services {
		t.Fatalf("expected no new service rows (rollback), before=%d after=%d", before.services, after.services)
	}
	if after.professionals != 0 || after.qualifications != 0 || after.history != 0 {
		t.Fatalf("expected zero professionals/qualifications/history after rollback, got %+v", after)
	}
}

func TestRepository_Apply_DeactivatedProfessional_StaysDeactivated(t *testing.T) {
	db := requireTestDB(t)

	if err := db.Exec(
		`INSERT INTO professionals (id, code, display_name, is_active) VALUES ('22222222-2222-2222-2222-222222222222', 'SENIOR_1', 'Senior 1', false)`,
	).Error; err != nil {
		t.Fatalf("seed inactive professional row: %v", err)
	}

	seeder := seedsvc.NewSeeder(NewRepository(db), &fakeIDGenerator{})
	result, err := seeder.Apply(context.Background(), testArtifact(), "checksum-1", "tester")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Applied {
		t.Fatalf("expected apply to succeed, got %+v", result)
	}

	var isActive bool
	if err := db.Raw(`SELECT is_active FROM professionals WHERE code = 'SENIOR_1'`).Scan(&isActive).Error; err != nil {
		t.Fatalf("query professional: %v", err)
	}
	if isActive {
		t.Fatal("expected pre-existing deactivated professional to remain inactive after seeding")
	}
}

type counts struct {
	services, professionals, qualifications, history int64
}

func countsSnapshot(t *testing.T, db *gorm.DB) counts {
	t.Helper()
	s, p, q, h := tableCounts(t, db)
	return counts{services: s, professionals: p, qualifications: q, history: h}
}
