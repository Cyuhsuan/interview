package main

import (
	"go.uber.org/fx"
	"gorm.io/gorm"

	catalogHandler "backend/internal/handler/catalog"
	"backend/internal/handler/health"
	"backend/internal/platform/config"
	"backend/internal/platform/database"
	"backend/internal/platform/httpserver"
	catalogRepo "backend/internal/repository/catalog"
	catalogService "backend/internal/service/catalog"
)

func main() {
	fx.New(
		fx.Provide(
			config.Load,
			database.New,
			httpserver.NewEngine,

			catalogRepo.NewRepository,
			func(r *catalogRepo.Repository) catalogService.ServiceRepository { return r },
			func(r *catalogRepo.Repository) catalogService.ProfessionalRepository { return r },
			catalogService.NewService,
			catalogHandler.NewHandler,
		),
		fx.Invoke(
			func(*gorm.DB) {}, // force the pool to be constructed and pinged at startup
			health.RegisterRoutes,
			catalogHandler.RegisterRoutes,
			httpserver.RegisterServer,
		),
	).Run()
}
