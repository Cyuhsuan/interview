// Package catalog is the only place in the Catalog module allowed to
// import GORM or hold a *gorm.DB reference, per backend/README.md's
// tech-stack boundaries.
package catalog

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"backend/internal/model"
	catalogsvc "backend/internal/service/catalog"
)

type Repository struct {
	db *gorm.DB
}

// NewRepository is the Fx constructor. db is the process-wide singleton
// pool built by internal/platform/database — never construct a *gorm.DB
// here.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

var (
	_ catalogsvc.ServiceRepository      = (*Repository)(nil)
	_ catalogsvc.ProfessionalRepository = (*Repository)(nil)
)

func (r *Repository) ListActiveServices(ctx context.Context) ([]model.Service, error) {
	var services []model.Service
	if err := r.db.WithContext(ctx).
		Where("is_active = ?", true).
		Order("code").
		Find(&services).Error; err != nil {
		return nil, fmt.Errorf("list active services: %w", err)
	}
	return services, nil
}

// ListActiveProfessionalsByServiceCode returns active professionals
// qualified for the given active service, in one round trip. An unknown
// or inactive service code yields an empty slice, not an error.
func (r *Repository) ListActiveProfessionalsByServiceCode(ctx context.Context, serviceCode string) ([]model.Professional, error) {
	var professionals []model.Professional
	err := r.db.WithContext(ctx).
		Joins("JOIN professional_service_qualifications psq ON psq.professional_id = professionals.id").
		Joins("JOIN services s ON s.id = psq.service_id").
		Where("professionals.is_active = ?", true).
		Where("s.code = ?", serviceCode).
		Where("s.is_active = ?", true).
		Order("professionals.code").
		Find(&professionals).Error
	if err != nil {
		return nil, fmt.Errorf("list active professionals by service code: %w", err)
	}
	return professionals, nil
}
