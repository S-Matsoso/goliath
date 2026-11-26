package system

import (
	"encoding/json"
	"testing"

	"github.com/S-Matsoso/goliath/internal/metrics"
)

func TestAdminMetricsResponseIncludesRuntimeAndDatabasePools(t *testing.T) {
	payload, err := json.Marshal(adminMetricsResponse{
		Snapshot: metrics.Snapshot{UptimeSeconds: 12},
		DatabasePools: []databasePoolMetrics{{
			Name: "identity", OpenConnections: 4, InUseConnections: 1,
			IdleConnections: 3, MaxConnections: 30, WaitCount: 2, WaitDurationMillis: 7,
		}},
	})
	if err != nil {
		t.Fatalf("marshal admin metrics: %v", err)
	}

	var response map[string]json.RawMessage
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatalf("unmarshal admin metrics: %v", err)
	}
	if _, ok := response["uptime_seconds"]; !ok {
		t.Fatal("runtime metrics are not flattened into the response")
	}
	var pools []databasePoolMetrics
	if err := json.Unmarshal(response["database_pools"], &pools); err != nil {
		t.Fatalf("unmarshal database pools: %v", err)
	}
	if len(pools) != 1 || pools[0].Name != "identity" || pools[0].MaxConnections != 30 {
		t.Fatalf("database pool metrics = %#v", pools)
	}
}
