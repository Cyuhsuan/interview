// Package seed implements the Reference-data Seeder described in
// backend/README.md's "Reference-data Seeder" section. It has no HTTP
// handler — it is invoked only via cmd/migrate's "seed" subcommand, per
// AGENTS.md's rule that seeding must never run automatically at API
// startup.
package seed

import (
	"context"
	"errors"
	"fmt"
	"time"

	"backend/internal/model"
)

// TxRepository is the set of operations available within a single
// seeding transaction. Implementations must contain no business rules —
// all decisions (conflict detection, what to skip, what to insert) live
// in Seeder.Apply.
type TxRepository interface {
	FindServiceByCode(ctx context.Context, code string) (*model.Service, error)
	InsertService(ctx context.Context, s model.Service) error
	FindProfessionalByCode(ctx context.Context, code string) (*model.Professional, error)
	InsertProfessional(ctx context.Context, p model.Professional) error
	QualificationExists(ctx context.Context, professionalID, serviceID string) (bool, error)
	InsertQualification(ctx context.Context, q model.ProfessionalServiceQualification) error
	FindSeedHistory(ctx context.Context, version string) (*model.SeedHistory, error)
	InsertSeedHistory(ctx context.Context, h model.SeedHistory) error
}

// Repository provides the transaction boundary the seeder needs: all of
// README's "single PostgreSQL transaction, all-or-nothing" requirement is
// satisfied by running the entire Apply callback through WithTx.
type Repository interface {
	WithTx(ctx context.Context, fn func(TxRepository) error) error
}

// IDGenerator is the injectable, CSPRNG-based UUID generator required by
// README's Canonical Types.
type IDGenerator interface {
	NewID() (string, error)
}

var (
	// ErrSeedChecksumConflict: same version already recorded with a
	// different checksum.
	ErrSeedChecksumConflict = errors.New("seed: version exists with a different checksum")
	// ErrSeedStaticFieldMismatch: an existing row's static fields don't
	// match the artifact. Wrapped by *FieldMismatch for detail.
	ErrSeedStaticFieldMismatch = errors.New("seed: existing row does not match artifact's static fields")
)

// FieldMismatch carries the detail behind ErrSeedStaticFieldMismatch.
type FieldMismatch struct {
	EntityKind string // "service" | "professional"
	Code       string
	Field      string
	Expected   string
	Actual     string
}

func (m *FieldMismatch) Error() string {
	return fmt.Sprintf("%s %q: field %s expected %q, got %q", m.EntityKind, m.Code, m.Field, m.Expected, m.Actual)
}

func (m *FieldMismatch) Unwrap() error { return ErrSeedStaticFieldMismatch }

// Result summarizes what Apply did.
type Result struct {
	Applied               bool // false => no-op, version+checksum already recorded
	Version               string
	ServicesCreated       int
	ProfessionalsCreated  int
	QualificationsCreated int
}

type Seeder struct {
	repo Repository
	ids  IDGenerator
}

func NewSeeder(repo Repository, ids IDGenerator) *Seeder {
	return &Seeder{repo: repo, ids: ids}
}

// Apply runs the full Reference-data Seeder algorithm (README rules 1-4)
// inside one transaction.
func (s *Seeder) Apply(ctx context.Context, artifact Artifact, checksum, executorID string) (Result, error) {
	var result Result

	err := s.repo.WithTx(ctx, func(tx TxRepository) error {
		existingHistory, err := tx.FindSeedHistory(ctx, artifact.Version)
		if err != nil {
			return fmt.Errorf("find seed history: %w", err)
		}
		if existingHistory != nil {
			if existingHistory.Checksum == checksum {
				result = Result{Applied: false, Version: artifact.Version}
				return nil
			}
			return fmt.Errorf("version %s already recorded with a different checksum: %w", artifact.Version, ErrSeedChecksumConflict)
		}

		serviceIDByCode := make(map[string]string, len(artifact.Services))
		for _, svc := range artifact.Services {
			existing, err := tx.FindServiceByCode(ctx, svc.Code)
			if err != nil {
				return fmt.Errorf("find service %s: %w", svc.Code, err)
			}
			if existing != nil {
				if existing.DisplayName != svc.DisplayName {
					return &FieldMismatch{EntityKind: "service", Code: svc.Code, Field: "displayName", Expected: svc.DisplayName, Actual: existing.DisplayName}
				}
				if existing.DurationMinutes != svc.DurationMinutes {
					return &FieldMismatch{EntityKind: "service", Code: svc.Code, Field: "durationMinutes", Expected: fmt.Sprint(svc.DurationMinutes), Actual: fmt.Sprint(existing.DurationMinutes)}
				}
				serviceIDByCode[svc.Code] = existing.ID
				continue
			}

			id, err := s.ids.NewID()
			if err != nil {
				return fmt.Errorf("generate service id for %s: %w", svc.Code, err)
			}
			row := model.Service{
				ID:              id,
				Code:            svc.Code,
				DisplayName:     svc.DisplayName,
				DurationMinutes: svc.DurationMinutes,
				IsActive:        true,
			}
			if err := tx.InsertService(ctx, row); err != nil {
				return fmt.Errorf("insert service %s: %w", svc.Code, err)
			}
			serviceIDByCode[svc.Code] = id
			result.ServicesCreated++
		}

		professionalIDByCode := make(map[string]string, len(artifact.Professionals))
		for _, pro := range artifact.Professionals {
			existing, err := tx.FindProfessionalByCode(ctx, pro.Code)
			if err != nil {
				return fmt.Errorf("find professional %s: %w", pro.Code, err)
			}
			if existing != nil {
				if existing.DisplayName != pro.DisplayName {
					return &FieldMismatch{EntityKind: "professional", Code: pro.Code, Field: "displayName", Expected: pro.DisplayName, Actual: existing.DisplayName}
				}
				professionalIDByCode[pro.Code] = existing.ID
				continue
			}

			id, err := s.ids.NewID()
			if err != nil {
				return fmt.Errorf("generate professional id for %s: %w", pro.Code, err)
			}
			row := model.Professional{
				ID:          id,
				Code:        pro.Code,
				DisplayName: pro.DisplayName,
				IsActive:    true,
			}
			if err := tx.InsertProfessional(ctx, row); err != nil {
				return fmt.Errorf("insert professional %s: %w", pro.Code, err)
			}
			professionalIDByCode[pro.Code] = id
			result.ProfessionalsCreated++
		}

		for _, pro := range artifact.Professionals {
			professionalID := professionalIDByCode[pro.Code]
			for _, serviceCode := range pro.Qualifications {
				serviceID, ok := serviceIDByCode[serviceCode]
				if !ok {
					return fmt.Errorf("artifact references unknown service code %s for professional %s", serviceCode, pro.Code)
				}
				exists, err := tx.QualificationExists(ctx, professionalID, serviceID)
				if err != nil {
					return fmt.Errorf("check qualification %s/%s: %w", pro.Code, serviceCode, err)
				}
				if exists {
					continue
				}
				q := model.ProfessionalServiceQualification{ProfessionalID: professionalID, ServiceID: serviceID}
				if err := tx.InsertQualification(ctx, q); err != nil {
					return fmt.Errorf("insert qualification %s/%s: %w", pro.Code, serviceCode, err)
				}
				result.QualificationsCreated++
			}
		}

		history := model.SeedHistory{
			Version:    artifact.Version,
			Checksum:   checksum,
			ExecutedAt: time.Now().UTC(),
			ExecutorID: executorID,
		}
		if err := tx.InsertSeedHistory(ctx, history); err != nil {
			return fmt.Errorf("insert seed history: %w", err)
		}

		result.Applied = true
		result.Version = artifact.Version
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return result, nil
}
