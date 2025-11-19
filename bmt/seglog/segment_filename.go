package seglog

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// SegmentFilename generates a segment filename from a segment number.
// Format: "rollback-NNNNN.seg" where NNNNN is zero-padded to 5 digits.
func SegmentFilename(segmentNum uint64) string {
	return fmt.Sprintf("rollback-%05d.seg", segmentNum)
}

// ParseSegmentFilename parses a segment filename to extract the segment number.
// Returns (segmentNum, true) if valid, (0, false) if invalid.
func ParseSegmentFilename(filename string) (uint64, bool) {
	// Extract just the filename (no directory)
	base := filepath.Base(filename)

	// Check prefix
	if !strings.HasPrefix(base, "rollback-") {
		return 0, false
	}

	// Check suffix
	if !strings.HasSuffix(base, ".seg") {
		return 0, false
	}

	// Extract number part
	numStr := strings.TrimPrefix(base, "rollback-")
	numStr = strings.TrimSuffix(numStr, ".seg")

	// Parse as uint64
	num, err := strconv.ParseUint(numStr, 10, 64)
	if err != nil {
		return 0, false
	}

	return num, true
}
