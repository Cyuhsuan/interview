package main

import (
	"go.uber.org/fx"

	"backend/internal/handler/health"
	"backend/internal/platform/config"
	"backend/internal/platform/httpserver"
)

func main() {
	fx.New(
		fx.Provide(
			config.Load,
			httpserver.NewEngine,
		),
		fx.Invoke(
			health.RegisterRoutes,
			httpserver.RegisterServer,
		),
	).Run()
}
