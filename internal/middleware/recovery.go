package middleware

import (
	"log/slog"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"github.com/S-Matsoso/goliath/internal/platform/httpx"
)

// Recovery converts handler panics into a logged 500 response with the standard
// error envelope, keeping the process alive.
func Recovery(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}

			log.Error("panic recovered",
				"panic", rec,
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
				"request_id", httpx.RequestID(c.Request.Context()),
				"stack", string(debug.Stack()),
			)

			if !c.Writer.Written() {
				httpx.InternalError(c.Writer, c.Request)
			}
			c.Abort()
		}()

		c.Next()
	}
}
