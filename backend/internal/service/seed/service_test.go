package seed

import (
	"context"
	"errors"
	"maps"
	"testing"

	"backend/internal/model"
)

// fakeRepo is an in-memory implementation of Repository/TxRepository.
// WithTx clones its state into a scratch copy, runs fn against that
// scratch copy, and only commits it back to the "real" state if fn
// returns nil — mirroring a real Postgres transaction's rollback
// semantics without needing an actual database.
type fakeRepo struct {
	services       map[string]model.Service // keyed by code
	professionals  map[string]model.Professional
	qualifications map[string]bool // "professionalID|serviceID"
	history        map[string]model.SeedHistory

	insertProfessionalCalls []string // codes passed to InsertProfessional, for assertions
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		services:       map[string]model.Service{},
		professionals:  map[string]model.Professional{},
		qualifications: map[string]bool{},
		history:        map[string]model.SeedHistory{},
	}
}

func (f *fakeRepo) clone() *fakeRepo {
	c := newFakeRepo()
	maps.Copy(c.services, f.services)
	maps.Copy(c.professionals, f.professionals)
	maps.Copy(c.qualifications, f.qualifications)
	maps.Copy(c.history, f.history)
	return c
}

func (f *fakeRepo) WithTx(ctx context.Context, fn func(TxRepository) error) error {
	scratch := f.clone()
	if err := fn(scratch); err != nil {
		return err // discard scratch; f is untouched
	}
	f.services = scratch.services
	f.professionals = scratch.professionals
	f.qualifications = scratch.qualifications
	f.history = scratch.history
	f.insertProfessionalCalls = append(f.insertProfessionalCalls, scratch.insertProfessionalCalls...)
	return nil
}

func (f *fakeRepo) FindServiceByCode(ctx context.Context, code string) (*model.Service, error) {
	if s, ok := f.services[code]; ok {
		return &s, nil
	}
	return nil, nil
}

func (f *fakeRepo) InsertService(ctx context.Context, s model.Service) error {
	f.services[s.Code] = s
	return nil
}

func (f *fakeRepo) FindProfessionalByCode(ctx context.Context, code string) (*model.Professional, error) {
	if p, ok := f.professionals[code]; ok {
		return &p, nil
	}
	return nil, nil
}

func (f *fakeRepo) InsertProfessional(ctx context.Context, p model.Professional) error {
	f.professionals[p.Code] = p
	f.insertProfessionalCalls = append(f.insertProfessionalCalls, p.Code)
	return nil
}

func (f *fakeRepo) QualificationExists(ctx context.Context, professionalID, serviceID string) (bool, error) {
	return f.qualifications[professionalID+"|"+serviceID], nil
}

func (f *fakeRepo) InsertQualification(ctx context.Context, q model.ProfessionalServiceQualification) error {
	f.qualifications[q.ProfessionalID+"|"+q.ServiceID] = true
	return nil
}

func (f *fakeRepo) FindSeedHistory(ctx context.Context, version string) (*model.SeedHistory, error) {
	if h, ok := f.history[version]; ok {
		return &h, nil
	}
	return nil, nil
}

func (f *fakeRepo) InsertSeedHistory(ctx context.Context, h model.SeedHistory) error {
	f.history[h.Version] = h
	return nil
}

type fakeIDGenerator struct{ n int }

func (g *fakeIDGenerator) NewID() (string, error) {
	g.n++
	return "id-" + string(rune('a'+g.n)), nil
}

func testArtifact() Artifact {
	return Artifact{
		Version: "1",
		Services: []ServiceSeed{
			{Code: "A", DisplayName: "Service A", DurationMinutes: 60},
			{Code: "B", DisplayName: "Service B", DurationMinutes: 60},
		},
		Professionals: []ProfessionalSeed{
			{Code: "JUNIOR", DisplayName: "Junior", Qualifications: []string{"A", "B"}},
			{Code: "SENIOR_1", DisplayName: "Senior 1", Qualifications: []string{"A", "B"}},
		},
	}
}

func TestApply_FreshRun(t *testing.T) {
	repo := newFakeRepo()
	seeder := NewSeeder(repo, &fakeIDGenerator{})

	result, err := seeder.Apply(context.Background(), testArtifact(), "checksum-1", "tester")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Applied || result.ServicesCreated != 2 || result.ProfessionalsCreated != 2 || result.QualificationsCreated != 4 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(repo.services) != 2 || len(repo.professionals) != 2 || len(repo.qualifications) != 4 {
		t.Fatalf("unexpected repo state: %+v", repo)
	}
	if _, ok := repo.history["1"]; !ok {
		t.Fatal("expected seed_history row for version 1")
	}
}

func TestApply_IdempotentRerun_NoOp(t *testing.T) {
	repo := newFakeRepo()
	seeder := NewSeeder(repo, &fakeIDGenerator{})

	if _, err := seeder.Apply(context.Background(), testArtifact(), "checksum-1", "tester"); err != nil {
		t.Fatalf("first apply failed: %v", err)
	}
	servicesBefore, professionalsBefore, qualificationsBefore := len(repo.services), len(repo.professionals), len(repo.qualifications)

	result, err := seeder.Apply(context.Background(), testArtifact(), "checksum-1", "tester")
	if err != nil {
		t.Fatalf("unexpected error on rerun: %v", err)
	}
	if result.Applied {
		t.Fatalf("expected no-op result, got %+v", result)
	}
	if len(repo.services) != servicesBefore || len(repo.professionals) != professionalsBefore || len(repo.qualifications) != qualificationsBefore {
		t.Fatal("expected no new writes on idempotent rerun")
	}
}

func TestApply_ChecksumConflict(t *testing.T) {
	repo := newFakeRepo()
	seeder := NewSeeder(repo, &fakeIDGenerator{})

	if _, err := seeder.Apply(context.Background(), testArtifact(), "checksum-1", "tester"); err != nil {
		t.Fatalf("first apply failed: %v", err)
	}

	_, err := seeder.Apply(context.Background(), testArtifact(), "checksum-2", "tester")
	if !errors.Is(err, ErrSeedChecksumConflict) {
		t.Fatalf("expected ErrSeedChecksumConflict, got %v", err)
	}
}

func TestApply_StaticFieldMismatch_RollsBackEverything(t *testing.T) {
	repo := newFakeRepo()
	repo.services["A"] = model.Service{ID: "existing-a", Code: "A", DisplayName: "Wrong Name", DurationMinutes: 60, IsActive: true}
	seeder := NewSeeder(repo, &fakeIDGenerator{})

	_, err := seeder.Apply(context.Background(), testArtifact(), "checksum-1", "tester")

	var mismatch *FieldMismatch
	if !errors.As(err, &mismatch) {
		t.Fatalf("expected *FieldMismatch, got %v", err)
	}
	if mismatch.EntityKind != "service" || mismatch.Code != "A" || mismatch.Field != "displayName" {
		t.Fatalf("unexpected mismatch detail: %+v", mismatch)
	}
	if !errors.Is(err, ErrSeedStaticFieldMismatch) {
		t.Fatal("expected error to unwrap to ErrSeedStaticFieldMismatch")
	}

	// Nothing else should have been written: service B, both
	// professionals, and all qualifications must be absent — proving
	// the whole transaction rolled back, not just the conflicting row.
	if len(repo.services) != 1 { // only the pre-existing "A" row
		t.Fatalf("expected no new service writes, got %+v", repo.services)
	}
	if len(repo.professionals) != 0 {
		t.Fatalf("expected no professional writes, got %+v", repo.professionals)
	}
	if len(repo.qualifications) != 0 {
		t.Fatalf("expected no qualification writes, got %+v", repo.qualifications)
	}
	if len(repo.history) != 0 {
		t.Fatalf("expected no seed_history writes, got %+v", repo.history)
	}
}

func TestApply_DeactivatedProfessional_IsNeverReactivated(t *testing.T) {
	repo := newFakeRepo()
	repo.professionals["SENIOR_1"] = model.Professional{ID: "existing-senior-1", Code: "SENIOR_1", DisplayName: "Senior 1", IsActive: false}
	seeder := NewSeeder(repo, &fakeIDGenerator{})

	result, err := seeder.Apply(context.Background(), testArtifact(), "checksum-1", "tester")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Applied {
		t.Fatalf("expected apply to succeed, got %+v", result)
	}

	got := repo.professionals["SENIOR_1"]
	if got.IsActive {
		t.Fatal("expected pre-existing deactivated professional to remain inactive")
	}
	for _, code := range repo.insertProfessionalCalls {
		if code == "SENIOR_1" {
			t.Fatal("expected InsertProfessional to never be called for an already-existing code")
		}
	}
}
