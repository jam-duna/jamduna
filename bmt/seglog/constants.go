package seglog

// Segmented log constants

const (
	// RECORD_ALIGNMENT is the alignment boundary for records (4K for O_DIRECT).
	RECORD_ALIGNMENT = 4096

	// HEADER_SIZE is the size of the record header (payload_length + record_id).
	HEADER_SIZE = 4 + 8 // 4 bytes payload_length + 8 bytes record_id

	// MAX_RECORD_PAYLOAD is the maximum size of a record payload (1 GiB).
	MAX_RECORD_PAYLOAD = 1 << 30

	// DEFAULT_SEGMENT_SIZE is the default maximum segment size (64 MiB).
	DEFAULT_SEGMENT_SIZE = 64 << 20

	// MIN_SEGMENT_SIZE is the minimum segment size (1 MiB).
	MIN_SEGMENT_SIZE = 1 << 20
)
