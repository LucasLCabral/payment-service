package monitoring

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"time"
)

type SystemMetrics struct {
	Timestamp time.Time `json:"timestamp"`

	MemAllocMB       float64 `json:"mem_alloc_mb"`
	MemSysMB         float64 `json:"mem_sys_mb"`
	MemHeapAllocMB   float64 `json:"mem_heap_alloc_mb"`
	MemHeapSysMB     float64 `json:"mem_heap_sys_mb"`
	MemStackSysMB    float64 `json:"mem_stack_sys_mb"`
	MemNumGC         uint32  `json:"mem_num_gc"`
	MemGCCPUFraction float64 `json:"mem_gc_cpu_fraction"`

	NumGoroutines int    `json:"num_goroutines"`
	NumCPU        int    `json:"num_cpu"`
	Version       string `json:"go_version"`

	DBStats *DatabaseStats `json:"db_stats,omitempty"`
}

type DatabaseStats struct {
	MaxOpenConnections int           `json:"max_open_connections"`
	OpenConnections    int           `json:"open_connections"`
	InUse              int           `json:"in_use"`
	Idle               int           `json:"idle"`
	WaitCount          int64         `json:"wait_count"`
	WaitDuration       time.Duration `json:"wait_duration"`
	MaxIdleClosed      int64         `json:"max_idle_closed"`
	MaxIdleTimeClosed  int64         `json:"max_idle_time_closed"`
	MaxLifetimeClosed  int64         `json:"max_lifetime_closed"`
}

type MetricsCollector struct {
	db *sql.DB
}

func NewMetricsCollector(db *sql.DB) *MetricsCollector {
	return &MetricsCollector{db: db}
}

func (mc *MetricsCollector) CollectMetrics() *SystemMetrics {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	metrics := &SystemMetrics{
		Timestamp:        time.Now(),
		MemAllocMB:       bToMb(m.Alloc),
		MemSysMB:         bToMb(m.Sys),
		MemHeapAllocMB:   bToMb(m.HeapAlloc),
		MemHeapSysMB:     bToMb(m.HeapSys),
		MemStackSysMB:    bToMb(m.StackSys),
		MemNumGC:         m.NumGC,
		MemGCCPUFraction: m.GCCPUFraction,
		NumGoroutines:    runtime.NumGoroutine(),
		NumCPU:           runtime.NumCPU(),
		Version:          runtime.Version(),
	}

	if mc.db != nil {
		stats := mc.db.Stats()
		metrics.DBStats = &DatabaseStats{
			MaxOpenConnections: stats.MaxOpenConnections,
			OpenConnections:    stats.OpenConnections,
			InUse:              stats.InUse,
			Idle:               stats.Idle,
			WaitCount:          stats.WaitCount,
			WaitDuration:       stats.WaitDuration,
			MaxIdleClosed:      stats.MaxIdleClosed,
			MaxIdleTimeClosed:  stats.MaxIdleTimeClosed,
			MaxLifetimeClosed:  stats.MaxLifetimeClosed,
		}
	}

	return metrics
}

func (mc *MetricsCollector) MetricsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		metrics := mc.CollectMetrics()

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache")

		if err := json.NewEncoder(w).Encode(metrics); err != nil {
			http.Error(w, "Failed to encode metrics", http.StatusInternalServerError)
			return
		}
	}
}

func (mc *MetricsCollector) PrometheusMetricsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		metrics := mc.CollectMetrics()

		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Cache-Control", "no-cache")

		w.Write([]byte("# HELP go_memstats_alloc_bytes Number of bytes allocated and still in use.\n"))
		w.Write([]byte("# TYPE go_memstats_alloc_bytes gauge\n"))
		w.Write(fmt.Appendf(nil, "go_memstats_alloc_bytes %d\n", int64(metrics.MemAllocMB*1024*1024)))

		w.Write([]byte("# HELP go_memstats_sys_bytes Number of bytes obtained from system.\n"))
		w.Write([]byte("# TYPE go_memstats_sys_bytes gauge\n"))
		w.Write(fmt.Appendf(nil, "go_memstats_sys_bytes %d\n", int64(metrics.MemSysMB*1024*1024)))

		w.Write([]byte("# HELP go_goroutines Number of goroutines that currently exist.\n"))
		w.Write([]byte("# TYPE go_goroutines gauge\n"))
		w.Write(fmt.Appendf(nil, "go_goroutines %d\n", metrics.NumGoroutines))

		if metrics.DBStats != nil {
			w.Write([]byte("# HELP db_connections_open Current number of open DB connections.\n"))
			w.Write([]byte("# TYPE db_connections_open gauge\n"))
			w.Write(fmt.Appendf(nil, "db_connections_open %d\n", metrics.DBStats.OpenConnections))

			w.Write([]byte("# HELP db_connections_in_use Current number of DB connections in use.\n"))
			w.Write([]byte("# TYPE db_connections_in_use gauge\n"))
			w.Write(fmt.Appendf(nil, "db_connections_in_use %d\n", metrics.DBStats.InUse))

			w.Write([]byte("# HELP db_connections_idle Current number of idle DB connections.\n"))
			w.Write([]byte("# TYPE db_connections_idle gauge\n"))
			w.Write(fmt.Appendf(nil, "db_connections_idle %d\n", metrics.DBStats.Idle))
		}
	}
}

func (mc *MetricsCollector) PerformanceAlert() *PerformanceStatus {
	metrics := mc.CollectMetrics()
	status := &PerformanceStatus{
		Healthy: true,
		Issues:  []string{},
	}

	if metrics.MemAllocMB > 500 {
		status.Healthy = false
		status.Issues = append(status.Issues, fmt.Sprintf("High memory usage: %.1fMB", metrics.MemAllocMB))
	}

	if metrics.NumGoroutines > 1000 {
		status.Healthy = false
		status.Issues = append(status.Issues, fmt.Sprintf("High goroutine count: %d", metrics.NumGoroutines))
	}

	if metrics.MemGCCPUFraction > 0.1 {
		status.Healthy = false
		status.Issues = append(status.Issues, fmt.Sprintf("High GC pressure: %.2f%%", metrics.MemGCCPUFraction*100))
	}

	if metrics.DBStats != nil {
		utilization := float64(metrics.DBStats.InUse) / float64(metrics.DBStats.MaxOpenConnections)
		if utilization > 0.8 {
			status.Healthy = false
			status.Issues = append(status.Issues, fmt.Sprintf("High DB connection utilization: %.1f%%", utilization*100))
		}

		if metrics.DBStats.WaitCount > 0 {
			status.Healthy = false
			status.Issues = append(status.Issues, fmt.Sprintf("DB connection waits detected: %d", metrics.DBStats.WaitCount))
		}
	}

	return status
}

type PerformanceStatus struct {
	Healthy bool     `json:"healthy"`
	Issues  []string `json:"issues"`
}

func bToMb(b uint64) float64 {
	return float64(b) / 1024 / 1024
}
