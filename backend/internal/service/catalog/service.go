// Package catalog holds the Catalog module's business logic and the
// repository-owned interfaces it depends on, per backend/AGENTS.md's
// handler/service/repository layering.
package catalog

import (
	"context"
	"errors"
	"regexp"

	"backend/internal/model"
)

// serviceCodePattern is the Canonical Types business-key format
// (backend/README.md "ID" section): ^[A-Z][A-Z0-9_]{0,31}$.
var serviceCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,31}$`)

// ErrInvalidServiceCode signals a serviceCode that does not match the
// business-key format. Handlers map this to 400 INVALID_REQUEST.
var ErrInvalidServiceCode = errors.New("serviceCode does not match required pattern")

// ServiceRepository is implemented by internal/repository/catalog.
type ServiceRepository interface {
	ListActiveServices(ctx context.Context) ([]model.Service, error)
}

// ProfessionalRepository is implemented by internal/repository/catalog.
type ProfessionalRepository interface {
	ListActiveProfessionalsByServiceCode(ctx context.Context, serviceCode string) ([]model.Professional, error)
}

type Service struct {
	services      ServiceRepository
	professionals ProfessionalRepository
}

func NewService(services ServiceRepository, professionals ProfessionalRepository) *Service {
	return &Service{services: services, professionals: professionals}
}

func (s *Service) ListActiveServices(ctx context.Context) ([]model.Service, error) {
	return s.services.ListActiveServices(ctx)
}

func (s *Service) ListQualifiedProfessionals(ctx context.Context, serviceCode string) ([]model.Professional, error) {
	if !serviceCodePattern.MatchString(serviceCode) {
		return nil, ErrInvalidServiceCode
	}
	return s.professionals.ListActiveProfessionalsByServiceCode(ctx, serviceCode)
}
