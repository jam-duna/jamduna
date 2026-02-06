package rollback

import (
	"fmt"

	"github.com/jam-duna/jamduna/bmt/beatree"
	"github.com/jam-duna/jamduna/bmt/seglog"
)

// ApplyFunc is a function that applies a changeset.
type ApplyFunc func(changeset map[beatree.Key]*beatree.Change) error

// Rollback provides delta-based rollback functionality for a beatree.
// It coordinates:
//   - Capturing prior values before commits (via ReverseDeltaWorker)
//   - Storing deltas in segmented log + in-memory cache (via Shared)
//   - Rolling back to previous states by applying deltas in reverse
type Rollback struct {
	shared       *Shared
	worker       *ReverseDeltaWorker
	applyFunc    ApplyFunc
	currentHead  seglog.RecordId // Tracks the current applied state
	metadataFile *metadataFile   // Persists currentHead across restarts
}

// Config holds configuration for the rollback system.
type Config struct {
	SegLogDir        string
	SegLogMaxSize    uint64
	InMemoryCapacity int
}

// NewRollback creates a new rollback system.
func NewRollback(lookup LookupFunc, apply ApplyFunc, cfg Config) (*Rollback, error) {
	// Create segmented log
	segLogCfg := seglog.Config{
		Dir:            cfg.SegLogDir,
		MaxSegmentSize: cfg.SegLogMaxSize,
	}
	if segLogCfg.MaxSegmentSize == 0 {
		segLogCfg.MaxSegmentSize = seglog.DEFAULT_SEGMENT_SIZE
	}

	segLog, err := seglog.NewSegmentedLog(segLogCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create segmented log: %w", err)
	}

	// Create in-memory cache
	inMemory := NewInMemory(cfg.InMemoryCapacity)

	// Create shared state
	shared := NewShared(inMemory, segLog)

	// Create reverse delta worker
	worker := NewReverseDeltaWorker(lookup)

	// Create metadata file handler
	metaFile := newMetadataFile(cfg.SegLogDir)

	// Load persisted metadata
	meta, err := metaFile.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load metadata: %w", err)
	}

	// Initialize currentHead from metadata if available, otherwise from seglog
	currentHead := meta.CurrentHead
	if currentHead == seglog.InvalidRecordId {
		// First time or after clean start - use seglog's last record
		nextId := segLog.NextRecordId()
		if nextId != seglog.InvalidRecordId && nextId != seglog.RecordId(1) {
			currentHead = nextId.Prev()
		}
	}

	return &Rollback{
		shared:       shared,
		worker:       worker,
		applyFunc:    apply,
		currentHead:  currentHead,
		metadataFile: metaFile,
	}, nil
}

// Commit captures the current state as a delta and persists it.
// The changeset represents modifications about to be applied to the tree.
// Returns the RecordId of the committed delta.
func (r *Rollback) Commit(changeset map[beatree.Key]*beatree.Change) (seglog.RecordId, error) {
	// Build reverse delta by fetching prior values
	delta, err := r.worker.BuildDelta(changeset)
	if err != nil {
		return seglog.InvalidRecordId, fmt.Errorf("failed to build delta: %w", err)
	}

	// Persist delta
	recordId, err := r.shared.Commit(delta)
	if err != nil {
		return seglog.InvalidRecordId, fmt.Errorf("failed to commit delta: %w", err)
	}

	// Update currentHead to the newly committed delta
	r.currentHead = recordId

	// Persist metadata
	if err := r.persistMetadata(); err != nil {
		return recordId, fmt.Errorf("failed to persist metadata: %w", err)
	}

	return recordId, nil
}

// RollbackTo rolls back the tree to the state at the given RecordId.
// This applies all deltas from the current state back to the target RecordId.
func (r *Rollback) RollbackTo(targetRecordId seglog.RecordId) error {
	// Check if we have any state to rollback from
	if r.currentHead == seglog.InvalidRecordId {
		return fmt.Errorf("no deltas to rollback")
	}

	// Apply deltas in reverse order from currentHead down to targetRecordId
	for recordId := r.currentHead; recordId > targetRecordId; recordId = recordId.Prev() {
		delta, err := r.shared.GetDelta(recordId)
		if err != nil {
			return fmt.Errorf("failed to get delta %d: %w", recordId, err)
		}

		if err := r.applyDelta(delta); err != nil {
			return fmt.Errorf("failed to apply delta %d: %w", recordId, err)
		}

		// Move currentHead backward
		if recordId == seglog.RecordId(1) {
			r.currentHead = seglog.InvalidRecordId
			break
		}
		r.currentHead = recordId.Prev()
	}

	// Persist metadata after rollback
	if err := r.persistMetadata(); err != nil {
		return fmt.Errorf("failed to persist metadata: %w", err)
	}

	return nil
}

// RollbackSingle rolls back the most recent commit.
func (r *Rollback) RollbackSingle() error {
	// Check if we have any state to rollback from
	if r.currentHead == seglog.InvalidRecordId {
		return fmt.Errorf("no deltas to rollback")
	}

	// Get the delta at currentHead
	delta, err := r.shared.GetDelta(r.currentHead)
	if err != nil {
		return fmt.Errorf("failed to get delta %d: %w", r.currentHead, err)
	}

	// Apply the delta
	if err := r.applyDelta(delta); err != nil {
		return fmt.Errorf("failed to apply delta: %w", err)
	}

	// Move currentHead backward
	if r.currentHead == seglog.RecordId(1) {
		r.currentHead = seglog.InvalidRecordId
	} else {
		r.currentHead = r.currentHead.Prev()
	}

	// Persist metadata after rollback
	if err := r.persistMetadata(); err != nil {
		return fmt.Errorf("failed to persist metadata: %w", err)
	}

	return nil
}

// applyDelta applies a reverse delta using the apply function.
// For each entry in the delta:
//   - If prior value is nil, erase the key (was an insert)
//   - If prior value exists, restore it (was an update/delete)
func (r *Rollback) applyDelta(delta *Delta) error {
	changeset := make(map[beatree.Key]*beatree.Change)

	for key, priorValue := range delta.Priors {
		if priorValue == nil {
			// Key was inserted - delete it
			changeset[key] = beatree.NewDeleteChange()
		} else {
			// Key was updated/deleted - restore prior value
			changeset[key] = beatree.NewInsertChange(priorValue)
		}
	}

	// Apply the changeset
	return r.applyFunc(changeset)
}

// Sync flushes pending deltas to disk.
func (r *Rollback) Sync() error {
	return r.shared.Sync()
}

// Prune removes deltas older than the given RecordId.
func (r *Rollback) Prune(beforeRecordId seglog.RecordId) error {
	return r.shared.Prune(beforeRecordId)
}

// Close closes the rollback system.
func (r *Rollback) Close() error {
	return r.shared.Close()
}

// NextRecordId returns the next RecordId that will be assigned.
func (r *Rollback) NextRecordId() seglog.RecordId {
	return r.shared.NextRecordId()
}

// InMemorySize returns the number of deltas in the in-memory cache.
func (r *Rollback) InMemorySize() int {
	return r.shared.InMemorySize()
}

// persistMetadata writes the current metadata to disk.
func (r *Rollback) persistMetadata() error {
	meta := &Metadata{
		Version:     metadataVersion,
		CurrentHead: r.currentHead,
	}
	return r.metadataFile.Save(meta)
}
