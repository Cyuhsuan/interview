package httpserver

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/fx"

	"backend/internal/platform/config"
)

func NewEngine() *gin.Engine {
	engine := gin.New()
	engine.Use(gin.Recovery())
	return engine
}

func RegisterServer(lc fx.Lifecycle, cfg config.Config, engine *gin.Engine) {
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", cfg.Port),
		Handler: engine,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			ln, err := net.Listen("tcp", srv.Addr)
			if err != nil {
				return err
			}
			go func() {
				_ = srv.Serve(ln)
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return srv.Shutdown(ctx)
		},
	})
}
