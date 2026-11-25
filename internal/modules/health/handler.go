package health

import (
	"net/http"

	"github.com/S-Matsoso/goliath/internal/platform/httpx"
)

// Handler translates HTTP requests into service calls. It contains no business
// logic and delegates all response writing to httpx.
type Handler struct {
	service *Service
}

// NewHandler returns a health handler backed by service.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Health serves GET /health.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, h.service.Status())
}
