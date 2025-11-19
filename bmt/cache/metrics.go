package cache

import (
	"fmt"
	"sync"
	"time"
)

// Metrics provides detailed cache performance metrics and monitoring.
type Metrics struct {
	mu sync.RWMutex

	// Performance metrics
	getLatency    *LatencyTracker
	putLatency    *LatencyTracker
	evictionCount int64

	// Cache efficiency metrics
	memoryUsage      int64  // Bytes
	evictionRate     float64 // Evictions per second (recent)
	loadFactor       float64 // Percentage of cache capacity used
	hotPageCount     int     // Pages accessed frequently
	coldPageCount    int     // Pages rarely accessed

	// Time-based metrics
	lastResetTime time.Time

	// Configuration
	trackingEnabled bool
}

// LatencyTracker tracks latency statistics for operations.
type LatencyTracker struct {
	mu sync.RWMutex

	// Latency statistics
	totalOps     int64
	totalLatency time.Duration
	minLatency   time.Duration
	maxLatency   time.Duration

	// Recent latency (sliding window)
	recentOps      []time.Duration
	recentIndex    int
	recentCapacity int
}

// NewMetrics creates a new metrics tracker.
func NewMetrics() *Metrics {
	return &Metrics{
		getLatency:      NewLatencyTracker(1000), // Track last 1000 operations
		putLatency:      NewLatencyTracker(1000),
		evictionCount:   0,
		memoryUsage:     0,
		evictionRate:    0.0,
		loadFactor:      0.0,
		hotPageCount:    0,
		coldPageCount:   0,
		lastResetTime:   time.Now(),
		trackingEnabled: true,
	}
}

// NewLatencyTracker creates a new latency tracker with the specified capacity
// for recent operations tracking.
func NewLatencyTracker(capacity int) *LatencyTracker {
	return &LatencyTracker{
		totalOps:       0,
		totalLatency:   0,
		minLatency:     time.Duration(0),
		maxLatency:     time.Duration(0),
		recentOps:      make([]time.Duration, capacity),
		recentIndex:    0,
		recentCapacity: capacity,
	}
}

// RecordGet records the latency of a cache get operation.
func (m *Metrics) RecordGet(latency time.Duration) {
	if !m.trackingEnabled {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.getLatency.Record(latency)
}

// RecordPut records the latency of a cache put operation.
func (m *Metrics) RecordPut(latency time.Duration) {
	if !m.trackingEnabled {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.putLatency.Record(latency)
}

// RecordEviction records a cache eviction event.
func (m *Metrics) RecordEviction() {
	if !m.trackingEnabled {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.evictionCount++
}

// UpdateMemoryUsage updates the current memory usage estimate.
func (m *Metrics) UpdateMemoryUsage(bytes int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.memoryUsage = bytes
}

// UpdateLoadFactor updates the cache load factor.
func (m *Metrics) UpdateLoadFactor(factor float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.loadFactor = factor
}

// GetSummary returns a summary of all metrics.
func (m *Metrics) GetSummary() MetricsSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return MetricsSummary{
		GetLatency: LatencySummary{
			Count:   m.getLatency.Count(),
			Average: m.getLatency.Average(),
			Min:     m.getLatency.Min(),
			Max:     m.getLatency.Max(),
			Recent:  m.getLatency.RecentAverage(),
		},
		PutLatency: LatencySummary{
			Count:   m.putLatency.Count(),
			Average: m.putLatency.Average(),
			Min:     m.putLatency.Min(),
			Max:     m.putLatency.Max(),
			Recent:  m.putLatency.RecentAverage(),
		},
		EvictionCount: m.evictionCount,
		MemoryUsage:   m.memoryUsage,
		EvictionRate:  m.evictionRate,
		LoadFactor:    m.loadFactor,
		HotPageCount:  m.hotPageCount,
		ColdPageCount: m.coldPageCount,
		UpTime:        time.Since(m.lastResetTime),
	}
}

// Reset resets all metrics.
func (m *Metrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.getLatency.Reset()
	m.putLatency.Reset()
	m.evictionCount = 0
	m.memoryUsage = 0
	m.evictionRate = 0.0
	m.loadFactor = 0.0
	m.hotPageCount = 0
	m.coldPageCount = 0
	m.lastResetTime = time.Now()
}

// Enable/Disable tracking
func (m *Metrics) SetTrackingEnabled(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.trackingEnabled = enabled
}

// Record records a latency measurement.
func (lt *LatencyTracker) Record(latency time.Duration) {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	lt.totalOps++
	lt.totalLatency += latency

	// Update min/max
	if lt.minLatency == 0 || latency < lt.minLatency {
		lt.minLatency = latency
	}
	if latency > lt.maxLatency {
		lt.maxLatency = latency
	}

	// Update recent operations (circular buffer)
	lt.recentOps[lt.recentIndex] = latency
	lt.recentIndex = (lt.recentIndex + 1) % lt.recentCapacity
}

// Count returns the total number of operations.
func (lt *LatencyTracker) Count() int64 {
	lt.mu.RLock()
	defer lt.mu.RUnlock()

	return lt.totalOps
}

// Average returns the average latency across all operations.
func (lt *LatencyTracker) Average() time.Duration {
	lt.mu.RLock()
	defer lt.mu.RUnlock()

	if lt.totalOps == 0 {
		return 0
	}
	return lt.totalLatency / time.Duration(lt.totalOps)
}

// Min returns the minimum recorded latency.
func (lt *LatencyTracker) Min() time.Duration {
	lt.mu.RLock()
	defer lt.mu.RUnlock()

	return lt.minLatency
}

// Max returns the maximum recorded latency.
func (lt *LatencyTracker) Max() time.Duration {
	lt.mu.RLock()
	defer lt.mu.RUnlock()

	return lt.maxLatency
}

// RecentAverage returns the average latency of recent operations.
func (lt *LatencyTracker) RecentAverage() time.Duration {
	lt.mu.RLock()
	defer lt.mu.RUnlock()

	count := int64(0)
	total := time.Duration(0)

	// Count non-zero entries (handles partial fill)
	for _, latency := range lt.recentOps {
		if latency > 0 {
			count++
			total += latency
		}
	}

	if count == 0 {
		return 0
	}
	return total / time.Duration(count)
}

// Reset resets all latency statistics.
func (lt *LatencyTracker) Reset() {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	lt.totalOps = 0
	lt.totalLatency = 0
	lt.minLatency = 0
	lt.maxLatency = 0
	lt.recentIndex = 0

	// Clear recent operations
	for i := range lt.recentOps {
		lt.recentOps[i] = 0
	}
}

// MetricsSummary provides a snapshot of cache metrics.
type MetricsSummary struct {
	GetLatency    LatencySummary
	PutLatency    LatencySummary
	EvictionCount int64
	MemoryUsage   int64
	EvictionRate  float64
	LoadFactor    float64
	HotPageCount  int
	ColdPageCount int
	UpTime        time.Duration
}

// LatencySummary provides a summary of latency metrics.
type LatencySummary struct {
	Count   int64
	Average time.Duration
	Min     time.Duration
	Max     time.Duration
	Recent  time.Duration
}

// String returns a human-readable representation of the metrics.
func (ms MetricsSummary) String() string {
	return fmt.Sprintf(`Cache Metrics Summary:
  Get Operations: %d (avg: %v, min: %v, max: %v, recent: %v)
  Put Operations: %d (avg: %v, min: %v, max: %v, recent: %v)
  Evictions: %d
  Memory Usage: %d bytes (%.1f MB)
  Load Factor: %.1f%%
  Hot Pages: %d, Cold Pages: %d
  Up Time: %v`,
		ms.GetLatency.Count, ms.GetLatency.Average, ms.GetLatency.Min, ms.GetLatency.Max, ms.GetLatency.Recent,
		ms.PutLatency.Count, ms.PutLatency.Average, ms.PutLatency.Min, ms.PutLatency.Max, ms.PutLatency.Recent,
		ms.EvictionCount,
		ms.MemoryUsage, float64(ms.MemoryUsage)/(1024*1024),
		ms.LoadFactor*100,
		ms.HotPageCount, ms.ColdPageCount,
		ms.UpTime,
	)
}