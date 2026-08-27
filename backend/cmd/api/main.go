package main

import (
	"go.uber.org/fx"
	"gorm.io/gorm"

	"backend/internal/handler/health"
	"backend/internal/platform/config"
	"backend/internal/platform/database"
	"backend/internal/platform/httpserver"
)

func main() {
	fx.New(
		fx.Provide(
			config.Load,
			database.New,
			httpserver.NewEngine,
		),
		fx.Invoke(
			func(*gorm.DB) {}, // force the pool to be constructed and pinged at startup
			health.RegisterRoutes,
			httpserver.RegisterServer,
		),
	).Run()
}
