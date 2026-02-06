package cache

import (
	"hash/fnv"
	"sync/atomic"

	"github.com/jam-duna/jamduna/bmt/core"
	"github.com/jam-duna/jamduna/bmt/io"
)

// PageCache implements a sharded LRU cache for pages.
// Sharding reduces lock contention by distributing pages across multiple cache shards.
type PageCache struct {
	shards     []*CacheShard
	shardCount int
	pagePool   *io.PagePool

	// Global statistics (atomic counters)
	totalHits      int64
	totalMisses    int64
	totalEvictions int64
}

const (
	// DefaultShardCount is the default number of cache shards
	DefaultShardCount = 16
	// DefaultEntriesPerShard is the default number of entries per shard
	DefaultEntriesPerShard = 1000
)

// NewPageCache creates a new sharded page cache.
func NewPageCache(totalCapacity int, pagePool *io.PagePool) *PageCache {
	return NewPageCacheWithShards(totalCapacity, DefaultShardCount, pagePool)
}

// NewPageCacheWithShards creates a new sharded page cache with custom shard count.
func NewPageCacheWithShards(totalCapacity, shardCount int, pagePool *io.PagePool) *PageCache {
	if shardCount <= 0 {
		shardCount = DefaultShardCount
	}
	if totalCapacity <= 0 {
		totalCapacity = DefaultShardCount * DefaultEntriesPerShard
	}

	entriesPerShard := totalCapacity / shardCount
	if entriesPerShard == 0 {
		entriesPerShard = 1
	}

	shards := make([]*CacheShard, shardCount)
	for i := 0; i < shardCount; i++ {
		shards[i] = NewCacheShard(entriesPerShard, pagePool)
	}

	return &PageCache{
		shards:         shards,
		shardCount:     shardCount,
		pagePool:       pagePool,
		totalHits:      0,
		totalMisses:    0,
		totalEvictions: 0,
	}
}

// Get retrieves a page from the cache.
// Returns the cache entry and true if found, nil and false otherwise.
func (pc *PageCache) Get(pageId core.PageId) (*CacheEntry, bool) {
	shard := pc.getShard(pageId)
	entry, found := shard.Get(pageId)

	if found {
		atomic.AddInt64(&pc.totalHits, 1)
	} else {
		atomic.AddInt64(&pc.totalMisses, 1)
	}

	return entry, found
}

// Put adds a page to the cache.
// If the cache is full, evicts the least recently used entry from the appropriate shard.
func (pc *PageCache) Put(pageId core.PageId, page io.Page) *CacheEntry {
	shard := pc.getShard(pageId)
	entry, evicted := shard.Put(pageId, page)

	if evicted != nil {
		atomic.AddInt64(&pc.totalEvictions, 1)
	}

	return entry
}

// Remove removes a page from the cache.
func (pc *PageCache) Remove(pageId core.PageId) *CacheEntry {
	shard := pc.getShard(pageId)
	return shard.Remove(pageId)
}

// Clear removes all entries from all shards.
func (pc *PageCache) Clear() {
	for _, shard := range pc.shards {
		shard.Clear()
	}

	atomic.StoreInt64(&pc.totalHits, 0)
	atomic.StoreInt64(&pc.totalMisses, 0)
	atomic.StoreInt64(&pc.totalEvictions, 0)
}

// Stats returns aggregated cache statistics.
func (pc *PageCache) Stats() CacheStats {
	var totalSize int
	var shardStats []ShardStats

	for i, shard := range pc.shards {
		hits, misses, evictions, size := shard.Stats()
		totalSize += size

		shardStats = append(shardStats, ShardStats{
			ShardId:   i,
			Hits:      hits,
			Misses:    misses,
			Evictions: evictions,
			Size:      size,
		})
	}

	return CacheStats{
		TotalHits:      atomic.LoadInt64(&pc.totalHits),
		TotalMisses:    atomic.LoadInt64(&pc.totalMisses),
		TotalEvictions: atomic.LoadInt64(&pc.totalEvictions),
		TotalSize:      totalSize,
		ShardCount:     pc.shardCount,
		Shards:         shardStats,
	}
}

// HitRate returns the cache hit rate as a percentage.
func (pc *PageCache) HitRate() float64 {
	hits := atomic.LoadInt64(&pc.totalHits)
	misses := atomic.LoadInt64(&pc.totalMisses)

	total := hits + misses
	if total == 0 {
		return 0.0
	}

	return float64(hits) / float64(total) * 100.0
}

// getShard returns the cache shard for a given page ID.
func (pc *PageCache) getShard(pageId core.PageId) *CacheShard {
	hash := pc.hashPageId(pageId)
	return pc.shards[hash%uint64(pc.shardCount)]
}

// hashPageId computes a hash for a page ID to determine shard assignment.
func (pc *PageCache) hashPageId(pageId core.PageId) uint64 {
	h := fnv.New64a()
	pageIdBytes := pageId.Encode()
	h.Write(pageIdBytes[:])
	return h.Sum64()
}

// CacheStats represents aggregated cache statistics.
type CacheStats struct {
	TotalHits      int64
	TotalMisses    int64
	TotalEvictions int64
	TotalSize      int
	ShardCount     int
	Shards         []ShardStats
}

// ShardStats represents statistics for a single cache shard.
type ShardStats struct {
	ShardId   int
	Hits      int64
	Misses    int64
	Evictions int64
	Size      int
}