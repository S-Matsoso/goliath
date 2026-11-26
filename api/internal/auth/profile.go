package auth

import (
	"net/http"

	"github.com/S-Matsoso/goliath/internal/platform/httpx"
	"github.com/S-Matsoso/goliath/internal/platform/router"
	"github.com/gin-gonic/gin"
)

type profileHandler struct {
	authenticator *OIDCAuthenticator
}

func RegisterProfile(r *router.Router, authenticator *OIDCAuthenticator) {
	handler := profileHandler{authenticator: authenticator}
	r.HandleProtected(
		http.MethodGet,
		"/auth/me",
		"Read the authenticated official profile",
		"any",
		"authenticated",
		authenticator.RequireAny(),
		handler.get,
	)
}

func (h profileHandler) get(c *gin.Context) {
	claims, ok := RequestClaims(c)
	if !ok {
		httpx.InternalError(c.Writer, c.Request)
		return
	}
	httpx.JSON(c.Writer, http.StatusOK, map[string]any{
		"official_id": claims.OfficialID,
		"department":  claims.Department,
		"role":        claims.Role,
	})
}
