package metrics

import (
	"runtime"
	runtimemetrics "runtime/metrics"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type Snapshot struct {
	UptimeSeconds     float64        `json:"uptime_seconds"`
	RequestsTotal     uint64         `json:"requests_total"`
	ServerErrorsTotal uint64         `json:"server_errors_total"`
	AverageLatencyMS  float64        `json:"average_latency_ms"`
	StatusCodes       map[int]uint64 `json:"status_codes"`
	Goroutines        int            `json:"goroutines"`
	CPUCores          int            `json:"cpu_cores"`
	GoMaxProcs        int            `json:"gomaxprocs"`
	CPUSecondsTotal   float64        `json:"cpu_seconds_total"`
	CPUSecondsUser    float64        `json:"cpu_seconds_user"`
	HeapInUseBytes    uint64         `json:"heap_in_use_bytes"`
	HeapAllocBytes    uint64         `json:"heap_alloc_bytes"`
	TotalAllocBytes   uint64         `json:"total_alloc_bytes"`
	GCCycles          uint32         `json:"gc_cycles"`
}

type Collector struct {
	startedAt time.Time
	mu        sync.RWMutex
	requests  uint64
	errors    uint64
	duration  time.Duration
	statuses  map[int]uint64
}

func New() *Collector {
	return &Collector{startedAt: time.Now(), statuses: make(map[int]uint64)}
}

func (c *Collector) Middleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		started := time.Now()
		ctx.Next()
		elapsed := time.Since(started)
		status := ctx.Writer.Status()

		c.mu.Lock()
		c.requests++
		c.duration += elapsed
		c.statuses[status]++
		if status >= 500 {
			c.errors++
		}
		c.mu.Unlock()
	}
}

func (c *Collector) Snapshot() Snapshot {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	cpuSamples := []runtimemetrics.Sample{
		{Name: "/cpu/classes/total:cpu-seconds"},
		{Name: "/cpu/classes/user:cpu-seconds"},
	}
	runtimemetrics.Read(cpuSamples)

	c.mu.RLock()
	requests, errors, duration := c.requests, c.errors, c.duration
	statuses := make(map[int]uint64, len(c.statuses))
	for status, count := range c.statuses {
		statuses[status] = count
	}
	c.mu.RUnlock()

	var averageLatency float64
	if requests > 0 {
		averageLatency = float64(duration) / float64(requests) / float64(time.Millisecond)
	}
	return Snapshot{
		UptimeSeconds:     time.Since(c.startedAt).Seconds(),
		RequestsTotal:     requests,
		ServerErrorsTotal: errors,
		AverageLatencyMS:  averageLatency,
		StatusCodes:       statuses,
		Goroutines:        runtime.NumGoroutine(),
		CPUCores:          runtime.NumCPU(),
		GoMaxProcs:        runtime.GOMAXPROCS(0),
		CPUSecondsTotal:   cpuSamples[0].Value.Float64(),
		CPUSecondsUser:    cpuSamples[1].Value.Float64(),
		HeapInUseBytes:    memory.HeapInuse,
		HeapAllocBytes:    memory.HeapAlloc,
		TotalAllocBytes:   memory.TotalAlloc,
		GCCycles:          memory.NumGC,
	}
}
