package admin

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/S-Matsoso/goliath/internal/platform/router"
)

func TestRegisterMountsAdminRoutesWithoutPublicRegistration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	routes := router.New(engine)
	NewHandler(nil, nil, nil, nil, "register-api").Register(routes)

	found := make(map[string]bool)
	for _, route := range routes.Routes() {
		found[route.Method+" "+route.Path] = true
	}
	for _, path := range []string{
		"POST /auth/login",
		"POST /auth/change-password",
		"GET /admin/metrics",
		"POST /admin/users",
		"POST /admin/departments",
	} {
		if !found[path] {
			t.Errorf("registered routes are missing %q", path)
		}
	}
	if found[http.MethodPost+" /auth/register"] {
		t.Fatal("public user registration must not be mounted")
	}
}
