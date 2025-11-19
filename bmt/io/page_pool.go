// Package io provides I/O abstractions for the BMT storage layer.
package io

import (
	"sync"
	"sync/atomic"
)

// PageSize is the size of a page in bytes.
// For GP mode (blake2b-gp), this should be 16KB (16384).
// For legacy mode, this is 4KB (4096).
const PageSize = 16384 // 16KB for GP mode

// Page is a reference to a pooled page buffer.
type Page []byte

// pooledPage wraps a page with reuse tracking.
type pooledPage struct {
	data   Page
	reused bool
}

// FatPage is a managed page that automatically returns to the pool when dropped.
// It wraps a Page and its PagePool for automatic cleanup.
type FatPage struct {
	data []byte
	pool *PagePool
}

// NewFatPage creates a new FatPage from a Page and PagePool.
func NewFatPage(data []byte, pool *PagePool) *FatPage {
	return &FatPage{
		data: data,
		pool: pool,
	}
}

// Data returns the underlying byte slice.
func (fp *FatPage) Data() []byte {
	return fp.data
}

// Clone creates a deep copy of the FatPage.
func (fp *FatPage) Clone() *FatPage {
	newPage := fp.pool.AllocFatPage()
	copy(newPage.data, fp.data)
	return newPage
}

// Release returns the page to the pool.
// After calling Release, the FatPage should not be used.
func (fp *FatPage) Release() {
	if fp.pool != nil && fp.data != nil {
		fp.pool.Dealloc(fp.data)
		fp.data = nil
		fp.pool = nil
	}
}

// PagePool is an efficient allocator for pages used in I/O operations.
// It uses sync.Pool for efficient page recycling.
type PagePool struct {
	pool         *sync.Pool
	allocCount   atomic.Int64
	deallocCount atomic.Int64
	reuseCount   atomic.Int64
	wrappers     sync.Map // maps &page[0] to *pooledPage for tracking
}

// NewPagePool creates a new page pool.
func NewPagePool() *PagePool {
	pp := &PagePool{
		pool: &sync.Pool{
			New: func() interface{} {
				// Allocate a new page aligned to page size
				return &pooledPage{
					data:   Page(make([]byte, PageSize)),
					reused: false,
				}
			},
		},
	}
	return pp
}

// Alloc allocates a page from the pool.
// The returned page may contain arbitrary data and should be cleared if needed.
func (pp *PagePool) Alloc() Page {
	pp.allocCount.Add(1)
	pooled := pp.pool.Get().(*pooledPage)

	// Track if this is a reused page
	if pooled.reused {
		pp.reuseCount.Add(1)
	}
	// Mark as reused for next time
	pooled.reused = true

	// Store wrapper for later lookup during Dealloc
	pp.wrappers.Store(&pooled.data[0], pooled)

	return pooled.data
}

// AllocFatPage allocates a FatPage from the pool.
// The FatPage will automatically return to the pool when Release() is called.
func (pp *PagePool) AllocFatPage() *FatPage {
	return NewFatPage(pp.Alloc(), pp)
}

// Dealloc returns a page to the pool for reuse.
// After calling Dealloc, the page should not be used.
func (pp *PagePool) Dealloc(page Page) {
	if page == nil || len(page) != PageSize {
		return
	}

	pp.deallocCount.Add(1)

	// Retrieve the wrapper and put it back in the pool
	if val, ok := pp.wrappers.LoadAndDelete(&page[0]); ok {
		pooled := val.(*pooledPage)
		pp.pool.Put(pooled)
	}
	// If wrapper not found, this page wasn't allocated from this pool
	// In that case, we just ignore it (defensive programming)
}

// Stats returns allocation statistics.
type PoolStats struct {
	AllocCount   int64
	DeallocCount int64
	ReuseCount   int64
}

// Stats returns current pool statistics.
func (pp *PagePool) Stats() PoolStats {
	return PoolStats{
		AllocCount:   pp.allocCount.Load(),
		DeallocCount: pp.deallocCount.Load(),
		ReuseCount:   pp.reuseCount.Load(),
	}
}

// Reset clears pool statistics (for testing).
func (pp *PagePool) Reset() {
	pp.allocCount.Store(0)
	pp.deallocCount.Store(0)
	pp.reuseCount.Store(0)
}
