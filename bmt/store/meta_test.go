package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMetaWrite(t *testing.T) {
	tmpDir := t.TempDir()

	manifest := &Manifest{
		RootHash:    [32]byte{1, 2, 3, 4, 5},
		SyncVersion: 42,
	}

	err := WriteManifest(tmpDir, manifest)
	if err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	// Verify file exists
	manifestPath := filepath.Join(tmpDir, "manifest")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		t.Errorf("Manifest file not created")
	}
}

func TestMetaRead(t *testing.T) {
	tmpDir := t.TempDir()

	// Write manifest
	original := &Manifest{
		RootHash:    [32]byte{0xAA, 0xBB, 0xCC},
		SyncVersion: 100,
	}

	err := WriteManifest(tmpDir, original)
	if err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	// Read it back
	read, err := ReadManifest(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read manifest: %v", err)
	}

	// Verify content
	if read.RootHash != original.RootHash {
		t.Errorf("Root hash mismatch: expected %v, got %v", original.RootHash, read.RootHash)
	}

	if read.SyncVersion != original.SyncVersion {
		t.Errorf("Sync version mismatch: expected %d, got %d", original.SyncVersion, read.SyncVersion)
	}
}

func TestMetaAtomicUpdate(t *testing.T) {
	tmpDir := t.TempDir()

	// Write initial manifest
	manifest1 := &Manifest{
		RootHash:    [32]byte{1, 1, 1},
		SyncVersion: 1,
	}
	WriteManifest(tmpDir, manifest1)

	// Update manifest
	manifest2 := &Manifest{
		RootHash:    [32]byte{2, 2, 2},
		SyncVersion: 2,
	}
	err := WriteManifest(tmpDir, manifest2)
	if err != nil {
		t.Fatalf("Failed to update manifest: %v", err)
	}

	// Verify temp file was removed
	tempPath := filepath.Join(tmpDir, "manifest.tmp")
	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Errorf("Temp file not cleaned up")
	}

	// Read and verify
	read, err := ReadManifest(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read updated manifest: %v", err)
	}

	if read.SyncVersion != 2 {
		t.Errorf("Manifest not updated atomically")
	}
}

func TestMetaReadNonExistent(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := ReadManifest(tmpDir)
	if err == nil {
		t.Errorf("Expected error when reading non-existent manifest")
	}
}

func TestMetaCorrupted(t *testing.T) {
	tmpDir := t.TempDir()

	// Write corrupted manifest (wrong size)
	manifestPath := filepath.Join(tmpDir, "manifest")
	os.WriteFile(manifestPath, []byte{1, 2, 3}, 0644)

	_, err := ReadManifest(tmpDir)
	if err == nil {
		t.Errorf("Expected error when reading corrupted manifest")
	}
}
