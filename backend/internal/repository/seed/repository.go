// Package seed is the only place in the Seeder module allowed to import
// GORM or hold a *gorm.DB reference, per backend/README.md's tech-stack
// boundaries.
package seed

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"backend/internal/model"
	seedsvc "backend/internal/service/seed"
)

type Repository struct {
	db *gorm.DB
}

// NewRepository is the constructor. db is the process-wide singleton pool
// built by internal/platform/database — never construct a *gorm.DB here.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

var _ seedsvc.Repository = (*Repository)(nil)

// WithTx runs fn inside a single Postgres transaction, satisfying
// README's "所有驗證、insert 與 seed_history 寫入在單一 PostgreSQL
// transaction 完成" — any error returned by fn rolls back every write
// made through the *txRepository handed to it.
func (r *Repository) WithTx(ctx context.Context, fn func(seedsvc.TxRepository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&txRepository{db: tx})
	})
}

// txRepository is pure CRUD scoped to one transaction — no business
// rules; those live in internal/service/seed.
type txRepository struct {
	db *gorm.DB
}

var _ seedsvc.TxRepository = (*txRepository)(nil)

func (t *txRepository) FindServiceByCode(ctx context.Context, code string) (*model.Service, error) {
	var row model.Service
	err := t.db.WithContext(ctx).Where("code = ?", code).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find service by code: %w", err)
	}
	return &row, nil
}

func (t *txRepository) InsertService(ctx context.Context, s model.Service) error {
	if err := t.db.WithContext(ctx).Create(&s).Error; err != nil {
		return fmt.Errorf("insert service: %w", err)
	}
	return nil
}

func (t *txRepository) FindProfessionalByCode(ctx context.Context, code string) (*model.Professional, error) {
	var row model.Professional
	err := t.db.WithContext(ctx).Where("code = ?", code).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find professional by code: %w", err)
	}
	return &row, nil
}

func (t *txRepository) InsertProfessional(ctx context.Context, p model.Professional) error {
	if err := t.db.WithContext(ctx).Create(&p).Error; err != nil {
		return fmt.Errorf("insert professional: %w", err)
	}
	return nil
}

func (t *txRepository) QualificationExists(ctx context.Context, professionalID, serviceID string) (bool, error) {
	var count int64
	err := t.db.WithContext(ctx).
		Model(&model.ProfessionalServiceQualification{}).
		Where("professional_id = ? AND service_id = ?", professionalID, serviceID).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check qualification existence: %w", err)
	}
	return count > 0, nil
}

func (t *txRepository) InsertQualification(ctx context.Context, q model.ProfessionalServiceQualification) error {
	if err := t.db.WithContext(ctx).Create(&q).Error; err != nil {
		return fmt.Errorf("insert qualification: %w", err)
	}
	return nil
}

func (t *txRepository) FindSeedHistory(ctx context.Context, version string) (*model.SeedHistory, error) {
	var row model.SeedHistory
	err := t.db.WithContext(ctx).Where("version = ?", version).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find seed history: %w", err)
	}
	return &row, nil
}

func (t *txRepository) InsertSeedHistory(ctx context.Context, h model.SeedHistory) error {
	if err := t.db.WithContext(ctx).Create(&h).Error; err != nil {
		return fmt.Errorf("insert seed history: %w", err)
	}
	return nil
}
