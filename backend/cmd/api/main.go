package main

import (
	"time"

	"go.uber.org/fx"
	"gorm.io/gorm"

	bookingHandler "backend/internal/handler/booking"
	catalogHandler "backend/internal/handler/catalog"
	"backend/internal/handler/health"
	schedulingHandler "backend/internal/handler/scheduling"
	"backend/internal/platform/config"
	"backend/internal/platform/database"
	"backend/internal/platform/httpserver"
	"backend/internal/platform/idgen"
	bookingRepo "backend/internal/repository/booking"
	catalogRepo "backend/internal/repository/catalog"
	schedulingRepo "backend/internal/repository/scheduling"
	bookingService "backend/internal/service/booking"
	catalogService "backend/internal/service/catalog"
	schedulingService "backend/internal/service/scheduling"
)

func newClinicLocation(cfg config.Config) (*time.Location, error) {
	return time.LoadLocation(cfg.ClinicTimezone)
}

func newSchedulingService(
	catalog *catalogService.Service,
	repo *schedulingRepo.Repository,
	loc *time.Location,
	cfg config.Config,
) *schedulingService.Service {
	return schedulingService.NewService(
		catalog, repo, repo, repo, loc,
		time.Duration(cfg.ClinicSlotIntervalMinutes)*time.Minute,
		time.Duration(cfg.ClinicMinLeadMinutes)*time.Minute,
	)
}

func newSchedulingHandler(service *schedulingService.Service, cfg config.Config) *schedulingHandler.Handler {
	return schedulingHandler.NewHandler(service, cfg.ClinicTimezone)
}

func newBookingService(
	repo *bookingRepo.Repository,
	catalog *catalogService.Service,
	scheduling *schedulingService.Service,
	ids idgen.Generator,
	loc *time.Location,
	cfg config.Config,
) *bookingService.Service {
	return bookingService.NewService(
		repo, catalog, scheduling, ids, loc,
		time.Duration(cfg.BookingSessionTTLMinutes)*time.Minute,
	)
}

func main() {
	fx.New(
		fx.Provide(
			config.Load,
			database.New,
			httpserver.NewEngine,
			newClinicLocation,
			idgen.New,

			catalogRepo.NewRepository,
			func(r *catalogRepo.Repository) catalogService.ServiceRepository { return r },
			func(r *catalogRepo.Repository) catalogService.ProfessionalRepository { return r },
			catalogService.NewService,
			catalogHandler.NewHandler,

			schedulingRepo.NewRepository,
			newSchedulingService,
			newSchedulingHandler,

			bookingRepo.NewRepository,
			newBookingService,
			bookingHandler.NewHandler,
		),
		fx.Invoke(
			func(*gorm.DB) {}, // force the pool to be constructed and pinged at startup
			health.RegisterRoutes,
			catalogHandler.RegisterRoutes,
			schedulingHandler.RegisterRoutes,
			bookingHandler.RegisterRoutes,
			httpserver.RegisterServer,
		),
	).Run()
}
