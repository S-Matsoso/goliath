package health

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler translates HTTP requests into service calls. It contains no business
// logic.
type Handler struct {
	service *Service
}

// NewHandler returns a health handler backed by service.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Health serves GET /health.
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, h.service.Status())
}
