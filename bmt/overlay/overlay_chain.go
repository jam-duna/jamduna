package overlay

import "fmt"

// ValidateChain validates that an overlay's ancestry is consistent.
// Returns an error if:
// - Any ancestor has been dropped
// - The chain is malformed
func ValidateChain(overlay *Overlay) error {
	// Check this overlay's status
	if overlay.Status() == StatusDropped {
		return fmt.Errorf("overlay has been dropped")
	}

	// Check all ancestors
	for i, ancestorData := range overlay.ancestors {
		status := ancestorData.Status().Get()
		if status == StatusDropped {
			return fmt.Errorf("ancestor %d has been dropped", i)
		}
	}

	return nil
}

// ValidateLiveChain validates that a live overlay's parent chain is consistent.
func ValidateLiveChain(liveOverlay *LiveOverlay) error {
	// Check if parent exists
	if liveOverlay.parent == nil {
		return nil // No parent is valid
	}

	// Validate parent overlay
	return ValidateChain(liveOverlay.parent)
}

// ChainDepth returns the depth of the overlay chain.
// Depth 0 means no ancestors, depth 1 means one parent, etc.
func ChainDepth(overlay *Overlay) int {
	return len(overlay.ancestors)
}

// LiveChainDepth returns the depth of a live overlay's chain.
func LiveChainDepth(liveOverlay *LiveOverlay) int {
	if liveOverlay.parent == nil {
		return 0
	}
	return 1 + ChainDepth(liveOverlay.parent)
}

// IsOrphaned returns true if the overlay has dropped ancestors.
// An orphaned overlay cannot be safely used.
func IsOrphaned(overlay *Overlay) bool {
	for _, ancestorData := range overlay.ancestors {
		if ancestorData.Status().IsDropped() {
			return true
		}
	}
	return false
}

// IsLiveOrphaned returns true if the live overlay's parent chain has dropped overlays.
func IsLiveOrphaned(liveOverlay *LiveOverlay) bool {
	if liveOverlay.parent == nil {
		return false
	}
	// Check if parent itself is dropped
	if liveOverlay.parent.Status() == StatusDropped {
		return true
	}
	// Check if any of parent's ancestors are dropped
	return IsOrphaned(liveOverlay.parent)
}

// GetAncestorStatuses returns the status of each ancestor.
func GetAncestorStatuses(overlay *Overlay) []OverlayStatus {
	statuses := make([]OverlayStatus, len(overlay.ancestors))
	for i, ancestorData := range overlay.ancestors {
		statuses[i] = ancestorData.Status().Get()
	}
	return statuses
}

// AllAncestorsCommitted returns true if all ancestors are committed.
func AllAncestorsCommitted(overlay *Overlay) bool {
	for _, ancestorData := range overlay.ancestors {
		if ancestorData.Status().Get() != StatusCommitted {
			return false
		}
	}
	return true
}

// AnyAncestorDropped returns true if any ancestor has been dropped.
func AnyAncestorDropped(overlay *Overlay) bool {
	return IsOrphaned(overlay)
}
