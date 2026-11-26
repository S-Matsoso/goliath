package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCollectorRecordsRequestStatusAndRuntimeMetrics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	collector := New()
	engine := gin.New()
	engine.Use(collector.Middleware())
	engine.GET("/ok", func(c *gin.Context) { c.Status(http.StatusOK) })
	engine.GET("/failed", func(c *gin.Context) { c.Status(http.StatusInternalServerError) })

	for _, path := range []string{"/ok", "/failed"} {
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	}

	snapshot := collector.Snapshot()
	if snapshot.RequestsTotal != 2 {
		t.Fatalf("RequestsTotal = %d, want 2", snapshot.RequestsTotal)
	}
	if snapshot.ServerErrorsTotal != 1 {
		t.Fatalf("ServerErrorsTotal = %d, want 1", snapshot.ServerErrorsTotal)
	}
	if snapshot.StatusCodes[http.StatusOK] != 1 || snapshot.StatusCodes[http.StatusInternalServerError] != 1 {
		t.Fatalf("StatusCodes = %#v, want one 200 and one 500", snapshot.StatusCodes)
	}
	if snapshot.AverageLatencyMS < 0 || snapshot.Goroutines < 1 {
		t.Fatalf("unexpected runtime metrics: %+v", snapshot)
	}
}
