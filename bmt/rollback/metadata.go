package rollback

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"

	"github.com/colorfulnotion/jam/bmt/seglog"
)

const (
	metadataFilename = "rollback.meta"
	metadataVersion  = 1
)

// Metadata stores persistent rollback state.
type Metadata struct {
	Version     uint32
	CurrentHead seglog.RecordId
}

// metadataFile handles reading/writing rollback metadata.
type metadataFile struct {
	path string
}

// newMetadataFile creates a metadata file handler.
func newMetadataFile(dir string) *metadataFile {
	return &metadataFile{
		path: filepath.Join(dir, metadataFilename),
	}
}

// Load reads the metadata from disk.
// Returns default metadata if file doesn't exist.
func (mf *metadataFile) Load() (*Metadata, error) {
	file, err := os.Open(mf.path)
	if os.IsNotExist(err) {
		// No metadata file yet - return defaults
		return &Metadata{
			Version:     metadataVersion,
			CurrentHead: seglog.InvalidRecordId,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to open metadata file: %w", err)
	}
	defer file.Close()

	var meta Metadata

	// Read version
	if err := binary.Read(file, binary.LittleEndian, &meta.Version); err != nil {
		return nil, fmt.Errorf("failed to read version: %w", err)
	}

	if meta.Version != metadataVersion {
		return nil, fmt.Errorf("unsupported metadata version: %d", meta.Version)
	}

	// Read currentHead
	if err := binary.Read(file, binary.LittleEndian, &meta.CurrentHead); err != nil {
		return nil, fmt.Errorf("failed to read currentHead: %w", err)
	}

	return &meta, nil
}

// Save writes the metadata to disk.
func (mf *metadataFile) Save(meta *Metadata) error {
	// Write to temporary file first
	tmpPath := mf.path + ".tmp"

	file, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temp metadata file: %w", err)
	}

	// Write version
	if err := binary.Write(file, binary.LittleEndian, meta.Version); err != nil {
		file.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write version: %w", err)
	}

	// Write currentHead
	if err := binary.Write(file, binary.LittleEndian, meta.CurrentHead); err != nil {
		file.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write currentHead: %w", err)
	}

	// Sync to disk
	if err := file.Sync(); err != nil {
		file.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to sync metadata file: %w", err)
	}

	file.Close()

	// Atomic rename
	if err := os.Rename(tmpPath, mf.path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename metadata file: %w", err)
	}

	return nil
}
