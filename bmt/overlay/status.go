package overlay

import "sync"

// OverlayStatus represents the lifecycle state of an overlay.
type OverlayStatus int

const (
	// StatusLive indicates the overlay is still mutable (LiveOverlay).
	StatusLive OverlayStatus = iota

	// StatusDropped indicates the overlay was discarded without committing.
	StatusDropped

	// StatusCommitted indicates the overlay was frozen and committed.
	StatusCommitted
)

// String returns the string representation of the status.
func (s OverlayStatus) String() string {
	switch s {
	case StatusLive:
		return "LIVE"
	case StatusDropped:
		return "DROPPED"
	case StatusCommitted:
		return "COMMITTED"
	default:
		return "UNKNOWN"
	}
}

// StatusTracker tracks the lifecycle status of an overlay.
// It provides thread-safe access to the current status.
type StatusTracker struct {
	mu     sync.RWMutex
	status OverlayStatus
}

// NewStatusTracker creates a new status tracker with initial status LIVE.
func NewStatusTracker() *StatusTracker {
	return &StatusTracker{
		status: StatusLive,
	}
}

// Get returns the current status.
func (st *StatusTracker) Get() OverlayStatus {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.status
}

// MarkDropped transitions the status to DROPPED.
// Returns true if successful, false if already dropped or committed.
func (st *StatusTracker) MarkDropped() bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.status != StatusLive {
		return false
	}

	st.status = StatusDropped
	return true
}

// MarkCommitted transitions the status to COMMITTED.
// Returns true if successful, false if already dropped or committed.
func (st *StatusTracker) MarkCommitted() bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.status != StatusLive {
		return false
	}

	st.status = StatusCommitted
	return true
}

// IsLive returns true if the status is LIVE.
func (st *StatusTracker) IsLive() bool {
	return st.Get() == StatusLive
}

// IsDropped returns true if the status is DROPPED.
func (st *StatusTracker) IsDropped() bool {
	return st.Get() == StatusDropped
}

// IsCommitted returns true if the status is COMMITTED.
func (st *StatusTracker) IsCommitted() bool {
	return st.Get() == StatusCommitted
}
