// Package webui serves the embedded single-page console for exercising the API
// from a browser. It is presentation only and calls the API over HTTP.
package webui

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed index.html
var indexHTML []byte

// Handler serves the console page.
type Handler struct{}

// NewHandler returns the console handler.
func NewHandler() *Handler {
	return &Handler{}
}

// Index serves the console at the root path.
func (h *Handler) Index(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
}
