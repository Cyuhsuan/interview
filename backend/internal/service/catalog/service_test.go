package catalog

import (
	"context"
	"errors"
	"testing"

	"backend/internal/model"
)

type fakeServiceRepository struct {
	services []model.Service
	err      error
	called   bool
}

func (f *fakeServiceRepository) ListActiveServices(ctx context.Context) ([]model.Service, error) {
	f.called = true
	return f.services, f.err
}

type fakeProfessionalRepository struct {
	professionals []model.Professional
	err           error
	called        bool
	gotCode       string
}

func (f *fakeProfessionalRepository) ListActiveProfessionalsByServiceCode(ctx context.Context, serviceCode string) ([]model.Professional, error) {
	f.called = true
	f.gotCode = serviceCode
	return f.professionals, f.err
}

func TestListActiveServices_HappyPath(t *testing.T) {
	repo := &fakeServiceRepository{services: []model.Service{{Code: "A"}}}
	svc := NewService(repo, &fakeProfessionalRepository{})

	got, err := svc.ListActiveServices(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Code != "A" {
		t.Fatalf("unexpected result: %+v", got)
	}
	if !repo.called {
		t.Fatal("expected repository to be called")
	}
}

func TestListActiveServices_RepositoryError(t *testing.T) {
	wantErr := errors.New("boom")
	repo := &fakeServiceRepository{err: wantErr}
	svc := NewService(repo, &fakeProfessionalRepository{})

	_, err := svc.ListActiveServices(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error to propagate, got %v", err)
	}
}

func TestListQualifiedProfessionals_HappyPath(t *testing.T) {
	repo := &fakeProfessionalRepository{professionals: []model.Professional{{Code: "SENIOR_1"}}}
	svc := NewService(&fakeServiceRepository{}, repo)

	got, err := svc.ListQualifiedProfessionals(context.Background(), "A")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Code != "SENIOR_1" {
		t.Fatalf("unexpected result: %+v", got)
	}
	if !repo.called || repo.gotCode != "A" {
		t.Fatalf("expected repository called with code A, got called=%v code=%q", repo.called, repo.gotCode)
	}
}

func TestListQualifiedProfessionals_InvalidFormatShortCircuits(t *testing.T) {
	repo := &fakeProfessionalRepository{}
	svc := NewService(&fakeServiceRepository{}, repo)

	cases := []string{"", "a", "lowercase", "1START", "TOO-MANY-DASHES", "WAY_TOO_LONG_CODE_OVER_THIRTY_TWO_CHARS"}
	for _, code := range cases {
		_, err := svc.ListQualifiedProfessionals(context.Background(), code)
		if !errors.Is(err, ErrInvalidServiceCode) {
			t.Errorf("code %q: expected ErrInvalidServiceCode, got %v", code, err)
		}
	}
	if repo.called {
		t.Fatal("expected repository to never be called for invalid codes")
	}
}

func TestListQualifiedProfessionals_RepositoryError(t *testing.T) {
	wantErr := errors.New("boom")
	repo := &fakeProfessionalRepository{err: wantErr}
	svc := NewService(&fakeServiceRepository{}, repo)

	_, err := svc.ListQualifiedProfessionals(context.Background(), "A")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error to propagate, got %v", err)
	}
}
