package bmt

import "sync/atomic"

// Metrics tracks performance statistics for a Nomt database.
type Metrics struct {
	reads       atomic.Uint64
	writes      atomic.Uint64
	commits     atomic.Uint64
	cacheHits   atomic.Uint64
	cacheMisses atomic.Uint64
}

// NewMetrics creates a new metrics tracker.
func NewMetrics() *Metrics {
	return &Metrics{}
}

// RecordRead records a read operation.
func (m *Metrics) RecordRead(cacheHit bool) {
	m.reads.Add(1)
	if cacheHit {
		m.cacheHits.Add(1)
	} else {
		m.cacheMisses.Add(1)
	}
}

// RecordWrite records a write operation.
func (m *Metrics) RecordWrite() {
	m.writes.Add(1)
}

// RecordCommit records a commit operation.
func (m *Metrics) RecordCommit() {
	m.commits.Add(1)
}

// Reads returns the total number of read operations.
func (m *Metrics) Reads() uint64 {
	return m.reads.Load()
}

// Writes returns the total number of write operations.
func (m *Metrics) Writes() uint64 {
	return m.writes.Load()
}

// Commits returns the total number of commit operations.
func (m *Metrics) Commits() uint64 {
	return m.commits.Load()
}

// CacheHits returns the number of cache hits.
func (m *Metrics) CacheHits() uint64 {
	return m.cacheHits.Load()
}

// CacheMisses returns the number of cache misses.
func (m *Metrics) CacheMisses() uint64 {
	return m.cacheMisses.Load()
}

// CacheHitRatio returns the cache hit ratio (0.0 - 1.0).
func (m *Metrics) CacheHitRatio() float64 {
	hits := m.cacheHits.Load()
	misses := m.cacheMisses.Load()
	total := hits + misses

	if total == 0 {
		return 0.0
	}

	return float64(hits) / float64(total)
}

// Reset resets all metrics to zero.
func (m *Metrics) Reset() {
	m.reads.Store(0)
	m.writes.Store(0)
	m.commits.Store(0)
	m.cacheHits.Store(0)
	m.cacheMisses.Store(0)
}
