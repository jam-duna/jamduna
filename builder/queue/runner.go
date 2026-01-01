package queue

import (
	"context"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

// SubmitFunc is the callback for submitting a work package bundle
// Returns the work package hash and error
type SubmitFunc func(bundle *types.WorkPackageBundle) (common.Hash, error)

// BuildBundleFunc is the callback for building/rebuilding a bundle
// This is called when an item needs to be resubmitted with a new RefineContext
type BuildBundleFunc func(item *QueueItem) (*types.WorkPackageBundle, error)

// Runner manages the queue submission loop
type Runner struct {
	mu sync.RWMutex

	queue     *QueueState
	serviceID uint32

	// Callbacks
	submitFunc     SubmitFunc
	buildBundleFunc BuildBundleFunc

	// Control
	tickInterval time.Duration
	stopCh       chan struct{}
	running      bool

	// Submission window timing (seconds before next timeslot to start submission)
	submissionWindowStart int // e.g., 3 seconds before timeslot
	submissionWindowEnd   int // e.g., 1 second before timeslot
}

// RunnerConfig holds configuration for the runner
type RunnerConfig struct {
	TickInterval          time.Duration
	SubmissionWindowStart int // Seconds before timeslot to start submission window
	SubmissionWindowEnd   int // Seconds before timeslot to end submission window
}

// DefaultRunnerConfig returns default runner configuration
func DefaultRunnerConfig() RunnerConfig {
	return RunnerConfig{
		TickInterval:          time.Second, // Check every second
		SubmissionWindowStart: 3,           // Start 3 seconds before timeslot
		SubmissionWindowEnd:   1,           // End 1 second before timeslot
	}
}

// NewRunner creates a new queue runner
func NewRunner(queue *QueueState, serviceID uint32, submitFunc SubmitFunc, buildBundleFunc BuildBundleFunc) *Runner {
	config := DefaultRunnerConfig()
	return &Runner{
		queue:                 queue,
		serviceID:             serviceID,
		submitFunc:            submitFunc,
		buildBundleFunc:       buildBundleFunc,
		tickInterval:          config.TickInterval,
		submissionWindowStart: config.SubmissionWindowStart,
		submissionWindowEnd:   config.SubmissionWindowEnd,
		stopCh:                make(chan struct{}),
	}
}

// Start begins the queue processing loop
func (r *Runner) Start(ctx context.Context) {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	r.running = true
	r.stopCh = make(chan struct{})
	r.mu.Unlock()

	log.Info(log.Node, "Queue Runner: Starting",
		"service", r.serviceID,
		"tickInterval", r.tickInterval)

	go r.runLoop(ctx)
}

// Stop stops the queue processing loop
func (r *Runner) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running {
		return
	}

	close(r.stopCh)
	r.running = false

	log.Info(log.Node, "Queue Runner: Stopped", "service", r.serviceID)
}

// IsRunning returns whether the runner is active
func (r *Runner) IsRunning() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.running
}

// runLoop is the main processing loop
func (r *Runner) runLoop(ctx context.Context) {
	ticker := time.NewTicker(r.tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.Stop()
			return
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.tick()
		}
	}
}

// tick processes one iteration of the queue
func (r *Runner) tick() {
	// Check for timeouts first
	r.queue.CheckTimeouts()

	// Check if we're in the submission window
	if !r.inSubmissionWindow() {
		return
	}

	// Try to submit items while we can
	for r.queue.CanSubmit() {
		item := r.queue.Dequeue()
		if item == nil {
			break
		}

		// If this is a resubmission (version > 1), rebuild the bundle
		if item.Version > 1 && r.buildBundleFunc != nil {
			newBundle, err := r.buildBundleFunc(item)
			if err != nil {
				log.Error(log.Node, "Queue Runner: Failed to rebuild bundle",
					"service", r.serviceID,
					"blockNumber", item.BlockNumber,
					"version", item.Version,
					"error", err)
				// Put back in queue for retry
				r.queue.mu.Lock()
				r.queue.Queued[item.BlockNumber] = item
				r.queue.mu.Unlock()
				continue
			}
			item.Bundle = newBundle
		}

		// Submit the bundle
		wpHash, err := r.submitFunc(item.Bundle)
		if err != nil {
			log.Error(log.Node, "Queue Runner: Submission failed",
				"service", r.serviceID,
				"blockNumber", item.BlockNumber,
				"version", item.Version,
				"error", err)
			// Put back in queue for retry
			r.queue.mu.Lock()
			r.queue.Queued[item.BlockNumber] = item
			r.queue.mu.Unlock()
			continue
		}

		// Mark as submitted
		r.queue.MarkSubmitted(item, wpHash)

		log.Info(log.Node, "Queue Runner: Submitted successfully",
			"service", r.serviceID,
			"blockNumber", item.BlockNumber,
			"version", item.Version,
			"wpHash", wpHash.Hex())
	}
}

// inSubmissionWindow checks if we're within the submission timing window
// Returns true if we're 2-3 seconds before the next JAM timeslot
func (r *Runner) inSubmissionWindow() bool {
	// Get current time within JAM timeslot (6 seconds)
	now := time.Now()
	timeslotDuration := 6 * time.Second

	// Calculate position within current timeslot
	epochStart := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	elapsed := now.Sub(epochStart)
	positionInSlot := elapsed % timeslotDuration
	secondsUntilNext := timeslotDuration - positionInSlot

	// Check if we're in the submission window
	// Window is [submissionWindowEnd, submissionWindowStart] seconds before next slot
	windowStart := time.Duration(r.submissionWindowStart) * time.Second
	windowEnd := time.Duration(r.submissionWindowEnd) * time.Second

	return secondsUntilNext <= windowStart && secondsUntilNext >= windowEnd
}

// GetQueue returns the underlying queue state
func (r *Runner) GetQueue() *QueueState {
	return r.queue
}

// EnqueueBundle is a convenience method to enqueue a bundle
func (r *Runner) EnqueueBundle(bundle *types.WorkPackageBundle) (uint64, error) {
	return r.queue.Enqueue(bundle)
}

// HandleGuaranteed processes a guarantee event
func (r *Runner) HandleGuaranteed(wpHash common.Hash) {
	r.queue.OnGuaranteed(wpHash)
}

// HandleAccumulated processes an accumulation event
func (r *Runner) HandleAccumulated(wpHash common.Hash) {
	r.queue.OnAccumulated(wpHash)
}

// HandleFinalized processes a finalization event
func (r *Runner) HandleFinalized(wpHash common.Hash) {
	r.queue.OnFinalized(wpHash)
}
