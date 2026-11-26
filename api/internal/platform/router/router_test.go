package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

func TestHandleMountsAndRecords(t *testing.T) {
	engine := newEngine()
	routes := New(engine)

	routes.Handle(http.MethodGet, "/z", "Zed", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	routes.Handle(http.MethodPost, "/a", "Create", func(c *gin.Context) {
		c.Status(http.StatusCreated)
	})
	routes.Handle(http.MethodGet, "/a", "List", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/z", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	want := []Route{
		{Method: http.MethodGet, Path: "/a", Summary: "List"},
		{Method: http.MethodPost, Path: "/a", Summary: "Create"},
		{Method: http.MethodGet, Path: "/z", Summary: "Zed"},
	}
	got := routes.Routes()
	if len(got) != len(want) {
		t.Fatalf("Routes() returned %d routes, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("route %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestGetRouteAnswersHeadOnce(t *testing.T) {
	engine := newEngine()
	routes := New(engine)
	routes.Handle(http.MethodGet, "/health", "Health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("HEAD status = %d, want %d", rec.Code, http.StatusOK)
	}

	if got := len(routes.Routes()); got != 1 {
		t.Fatalf("catalog has %d routes, want 1 (HEAD must not duplicate GET)", got)
	}
}

func TestRoutesReturnsCopy(t *testing.T) {
	routes := New(newEngine())
	routes.Handle(http.MethodGet, "/health", "Health", func(c *gin.Context) {})

	routes.Routes()[0].Path = "/tampered"

	if got := routes.Routes()[0].Path; got != "/health" {
		t.Fatalf("recorded path = %q, want it unchanged", got)
	}
}

func TestRootPathDoesNotCatchUnknownPaths(t *testing.T) {
	engine := newEngine()
	routes := New(engine)
	routes.Handle(http.MethodGet, "/", "Root", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown path status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("root status = %d, want %d", rec.Code, http.StatusOK)
	}

	if got := routes.Routes()[0].Path; got != "/" {
		t.Fatalf("catalog path = %q, want %q", got, "/")
	}
}

func TestPathParametersAreRecorded(t *testing.T) {
	engine := newEngine()
	routes := New(engine)
	routes.Handle(http.MethodGet, "/citizens/:id", "Get citizen", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/citizens/42", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	if got := routes.Routes()[0].Path; got != "/citizens/:id" {
		t.Fatalf("path = %q, want %q", got, "/citizens/:id")
	}
}

func TestAuthCoverageRequiresProtectedAPIHandlers(t *testing.T) {
	routes := New(newEngine())
	routes.Handle(http.MethodGet, "/healthz", "Health", func(c *gin.Context) {})
	routes.HandleProtected(http.MethodPost, "/v1/identity/persons", "Register",
		"identity", "supervisor",
		func(c *gin.Context) { c.Next() },
		func(c *gin.Context) {},
	)
	if err := routes.ValidateAuthCoverage(); err != nil {
		t.Fatalf("protected routes rejected: %v", err)
	}

	unprotected := New(newEngine())
	unprotected.Handle(http.MethodGet, "/v1/identity/persons", "List", func(c *gin.Context) {})
	if err := unprotected.ValidateAuthCoverage(); err == nil {
		t.Fatal("unprotected department route passed the self-test")
	}
}
