package httpserver

import "github.com/gin-gonic/gin"

// CORSMiddleware allows browser requests from a single, exact allowed
// origin. Non-matching origins receive no CORS headers at all, leaving the
// browser to enforce the block; non-browser clients (curl, Postman, tests)
// are unaffected either way.
func CORSMiddleware(allowedOrigin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && origin == allowedOrigin {
			c.Header("Access-Control-Allow-Origin", allowedOrigin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, If-Match, Idempotency-Key")
			c.Header("Access-Control-Expose-Headers", "ETag")
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
