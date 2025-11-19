package rollback

import (
	"github.com/colorfulnotion/jam/bmt/beatree"
)

// LookupFunc is a function type for looking up values in the tree.
type LookupFunc func(key beatree.Key) ([]byte, error)

// ReverseDeltaWorker fetches prior values for keys to build a reverse delta.
// This is used during commit to capture the state before modifications.
type ReverseDeltaWorker struct {
	lookup LookupFunc
}

// NewReverseDeltaWorker creates a new worker for fetching prior values.
func NewReverseDeltaWorker(lookup LookupFunc) *ReverseDeltaWorker {
	return &ReverseDeltaWorker{
		lookup: lookup,
	}
}

// BuildDelta constructs a delta from a changeset by fetching prior values.
// For each key in the changeset:
//   - If the key existed before, store its prior value (reinstate on rollback)
//   - If the key did not exist, store nil (erase on rollback)
// Returns error if any lookup fails.
func (w *ReverseDeltaWorker) BuildDelta(changeset map[beatree.Key]*beatree.Change) (*Delta, error) {
	delta := NewDelta()

	for key := range changeset {
		// Fetch prior value - propagate errors
		priorValue, err := w.lookup(key)
		if err != nil {
			return nil, err
		}

		// Store prior value (nil if key didn't exist)
		delta.AddPrior(key, priorValue)
	}

	return delta, nil
}

// FetchPriorValue retrieves the current value for a single key.
func (w *ReverseDeltaWorker) FetchPriorValue(key beatree.Key) ([]byte, error) {
	return w.lookup(key)
}

// FetchPriorValues retrieves current values for multiple keys.
// Returns a map of key -> value (nil if key doesn't exist).
// Returns error if any lookup fails.
func (w *ReverseDeltaWorker) FetchPriorValues(keys []beatree.Key) (map[beatree.Key][]byte, error) {
	result := make(map[beatree.Key][]byte)

	for _, key := range keys {
		value, err := w.lookup(key)
		if err != nil {
			return nil, err
		}
		result[key] = value
	}

	return result, nil
}
