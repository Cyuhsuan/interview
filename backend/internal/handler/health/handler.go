package health

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(engine *gin.Engine) {
	engine.GET("/healthz", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
}
