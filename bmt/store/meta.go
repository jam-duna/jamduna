package store

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
)

// Manifest represents the persistent state of the store.
// It's written atomically (write to temp file, then rename) to ensure consistency.
type Manifest struct {
	RootHash    [32]byte // Root hash of the beatree
	SyncVersion uint64   // Incremented on each successful sync
}

// WriteManifest writes the manifest atomically to disk.
// Uses write-to-temp-then-rename pattern for atomicity.
func WriteManifest(dirPath string, manifest *Manifest) error {
	manifestPath := filepath.Join(dirPath, "manifest")
	tempPath := filepath.Join(dirPath, "manifest.tmp")

	// Write to temporary file
	tempFile, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("failed to create temp manifest: %w", err)
	}
	defer tempFile.Close()

	// Serialize manifest
	buf := make([]byte, 40) // 32 bytes (root hash) + 8 bytes (version)
	copy(buf[0:32], manifest.RootHash[:])
	binary.LittleEndian.PutUint64(buf[32:40], manifest.SyncVersion)

	if _, err := tempFile.Write(buf); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	if err := tempFile.Sync(); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to sync manifest: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to close temp manifest: %w", err)
	}

	// Atomically replace old manifest with new one
	if err := os.Rename(tempPath, manifestPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename manifest: %w", err)
	}

	return nil
}

// ReadManifest reads the manifest from disk.
// Returns error if manifest doesn't exist or is corrupted.
func ReadManifest(dirPath string) (*Manifest, error) {
	manifestPath := filepath.Join(dirPath, "manifest")

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("manifest not found (store not initialized)")
		}
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	if len(data) != 40 {
		return nil, fmt.Errorf("manifest corrupted: expected 40 bytes, got %d", len(data))
	}

	manifest := &Manifest{}
	copy(manifest.RootHash[:], data[0:32])
	manifest.SyncVersion = binary.LittleEndian.Uint64(data[32:40])

	return manifest, nil
}
