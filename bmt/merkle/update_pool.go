package merkle

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/colorfulnotion/jam/bmt/core"
	"github.com/colorfulnotion/jam/bmt/io"
)

// UpdatePool manages a pool of workers for parallel merkle tree updates.
// Each worker processes page updates independently to maximize throughput.
type UpdatePool struct {
	workers     []*Worker
	workerCount int

	// Work distribution
	workQueue   chan *WorkItem
	resultQueue chan *WorkResult

	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Configuration
	pagePool *io.PagePool

	// Statistics
	mu           sync.RWMutex
	totalWork    int64
	completedWork int64
	failedWork   int64
}

// WorkItem represents a single page update task.
type WorkItem struct {
	PageId   core.PageId
	Updates  []PageUpdate
	Priority int // Higher priority items processed first

	// Context for cancellation
	ctx context.Context

	// Result callback
	resultChan chan *WorkResult
}

// PageUpdate represents an update to apply to a page.
type PageUpdate struct {
	Key    []byte
	Value  []byte
	Delete bool
}

// WorkResult contains the result of processing a work item.
type WorkResult struct {
	PageId   core.PageId
	Success  bool
	Error    error

	// Updated page data
	UpdatedPage io.Page
	NewHash     []byte

	// Witness information
	Witness     []byte
	WitnessHash []byte
}

// NewUpdatePool creates a new worker pool with the specified number of workers.
func NewUpdatePool(workerCount int, pagePool *io.PagePool) *UpdatePool {
	if workerCount <= 0 {
		workerCount = runtime.NumCPU()
	}

	ctx, cancel := context.WithCancel(context.Background())

	pool := &UpdatePool{
		workers:      make([]*Worker, workerCount),
		workerCount:  workerCount,
		workQueue:    make(chan *WorkItem, workerCount*10), // Buffer for work items
		resultQueue:  make(chan *WorkResult, workerCount*10),
		ctx:          ctx,
		cancel:       cancel,
		pagePool:     pagePool,
		totalWork:    0,
		completedWork: 0,
		failedWork:   0,
	}

	// Create and start workers
	for i := 0; i < workerCount; i++ {
		worker := NewWorker(i, pool.workQueue, pool.resultQueue, pagePool)
		pool.workers[i] = worker

		pool.wg.Add(1)
		go func(w *Worker) {
			defer pool.wg.Done()
			w.Run(ctx)
		}(worker)
	}

	return pool
}

// SubmitWork submits a work item for processing.
// Returns a channel that will receive the result.
func (up *UpdatePool) SubmitWork(pageId core.PageId, updates []PageUpdate, priority int) <-chan *WorkResult {
	resultChan := make(chan *WorkResult, 1)

	workItem := &WorkItem{
		PageId:     pageId,
		Updates:    updates,
		Priority:   priority,
		ctx:        up.ctx,
		resultChan: resultChan,
	}

	up.mu.Lock()
	up.totalWork++
	up.mu.Unlock()

	select {
	case up.workQueue <- workItem:
		// Work submitted successfully
	case <-up.ctx.Done():
		// Pool is shutting down
		close(resultChan)
	default:
		// Queue is full, this should be rare with proper sizing
		go func() {
			select {
			case up.workQueue <- workItem:
				// Eventually submitted
			case <-up.ctx.Done():
				close(resultChan)
			}
		}()
	}

	return resultChan
}

// SubmitWorkBlocking submits work and waits for the result.
func (up *UpdatePool) SubmitWorkBlocking(pageId core.PageId, updates []PageUpdate, priority int) (*WorkResult, error) {
	resultChan := up.SubmitWork(pageId, updates, priority)

	select {
	case result := <-resultChan:
		if result == nil {
			return nil, fmt.Errorf("work cancelled")
		}

		up.mu.Lock()
		if result.Success {
			up.completedWork++
		} else {
			up.failedWork++
		}
		up.mu.Unlock()

		return result, nil

	case <-up.ctx.Done():
		return nil, fmt.Errorf("pool shutdown")
	}
}

// ProcessBatch submits multiple work items and collects all results.
func (up *UpdatePool) ProcessBatch(items []*WorkItem) ([]*WorkResult, error) {
	if len(items) == 0 {
		return nil, nil
	}

	// Submit all work items
	resultChans := make([]<-chan *WorkResult, len(items))
	for i, item := range items {
		resultChans[i] = up.SubmitWork(item.PageId, item.Updates, item.Priority)
	}

	// Collect results
	results := make([]*WorkResult, len(items))
	var firstError error

	for i, resultChan := range resultChans {
		select {
		case result := <-resultChan:
			results[i] = result
			if result != nil && !result.Success && firstError == nil {
				firstError = result.Error
			}
		case <-up.ctx.Done():
			return nil, fmt.Errorf("pool shutdown")
		}
	}

	return results, firstError
}

// Stats returns current pool statistics.
func (up *UpdatePool) Stats() PoolStats {
	up.mu.RLock()
	defer up.mu.RUnlock()

	return PoolStats{
		WorkerCount:   up.workerCount,
		QueueDepth:    len(up.workQueue),
		TotalWork:     up.totalWork,
		CompletedWork: up.completedWork,
		FailedWork:    up.failedWork,
		ActiveWorkers: up.countActiveWorkers(),
	}
}

// countActiveWorkers counts the number of workers currently processing work.
func (up *UpdatePool) countActiveWorkers() int {
	active := 0
	for _, worker := range up.workers {
		if worker.IsBusy() {
			active++
		}
	}
	return active
}

// Shutdown gracefully shuts down the worker pool.
func (up *UpdatePool) Shutdown() {
	up.cancel()
	close(up.workQueue)
	up.wg.Wait()
}

// PoolStats provides statistics about the worker pool.
type PoolStats struct {
	WorkerCount   int
	QueueDepth    int
	TotalWork     int64
	CompletedWork int64
	FailedWork    int64
	ActiveWorkers int
}

// String returns a human-readable representation of pool stats.
func (ps PoolStats) String() string {
	successRate := float64(0)
	if ps.TotalWork > 0 {
		successRate = float64(ps.CompletedWork) / float64(ps.TotalWork) * 100
	}

	return fmt.Sprintf(`Worker Pool Stats:
  Workers: %d active / %d total
  Queue Depth: %d
  Work Items: %d total, %d completed, %d failed (%.1f%% success rate)`,
		ps.ActiveWorkers, ps.WorkerCount,
		ps.QueueDepth,
		ps.TotalWork, ps.CompletedWork, ps.FailedWork, successRate,
	)
}