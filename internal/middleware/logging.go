package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/S-Matsoso/goliath/internal/platform/httpx"
)

// Logging logs one structured line per request, including status and duration.
// Install it outside Recovery so panic responses are recorded too.
func Logging(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		log.Info("http request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration", time.Since(start),
			"request_id", httpx.RequestID(c.Request.Context()),
			"remote_addr", c.Request.RemoteAddr,
		)
	}
}
