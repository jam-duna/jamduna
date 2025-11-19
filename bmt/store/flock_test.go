package store

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestFlockAcquire(t *testing.T) {
	tmpDir := t.TempDir()

	lock, err := AcquireLock(tmpDir)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}
	defer lock.Release()

	// Verify lock file exists
	lockPath := filepath.Join(tmpDir, "LOCK")
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Errorf("Lock file not created")
	}
}

func TestFlockBlocks(t *testing.T) {
	tmpDir := t.TempDir()

	// Acquire first lock
	lock1, err := AcquireLock(tmpDir)
	if err != nil {
		t.Fatalf("Failed to acquire first lock: %v", err)
	}
	defer lock1.Release()

	// Try to acquire second lock (should fail)
	lock2, err := AcquireLock(tmpDir)
	if err == nil {
		lock2.Release()
		t.Fatalf("Second lock should have failed")
	}

	// Release first lock
	lock1.Release()

	// Now second lock should succeed
	lock3, err := AcquireLock(tmpDir)
	if err != nil {
		t.Fatalf("Failed to acquire lock after release: %v", err)
	}
	defer lock3.Release()
}

func TestFlockRelease(t *testing.T) {
	tmpDir := t.TempDir()

	lock, err := AcquireLock(tmpDir)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	err = lock.Release()
	if err != nil {
		t.Errorf("Failed to release lock: %v", err)
	}

	// Double release should be safe
	err = lock.Release()
	if err != nil {
		t.Errorf("Double release failed: %v", err)
	}
}

func TestFlockMultiProcess(t *testing.T) {
	// Skip if not in CI environment or if go command not available
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("Skipping multi-process test: go command not found")
	}

	tmpDir := t.TempDir()

	// Acquire lock in this process
	lock, err := AcquireLock(tmpDir)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}
	defer lock.Release()

	// Try to acquire lock from child process
	cmd := exec.Command("go", "run", "./testdata/flock_child.go", tmpDir)
	output, err := cmd.CombinedOutput()

	// Child process should fail to acquire lock
	if err == nil {
		t.Errorf("Child process should have failed to acquire lock")
	}

	// Output should indicate lock failure (if test program exists)
	if len(output) > 0 && !contains(string(output), "locked") {
		t.Logf("Child output: %s", output)
	}
}

func TestStoreMultipleOpen(t *testing.T) {
	tmpDir := t.TempDir()

	// Open first store
	store1, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Failed to open first store: %v", err)
	}
	defer store1.Close()

	// Try to open second store (should fail due to lock)
	store2, err := Open(tmpDir)
	if err == nil {
		store2.Close()
		t.Fatalf("Second store open should have failed (locked)")
	}

	// Close first store
	store1.Close()

	// Now second open should succeed
	store3, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Failed to open store after close: %v", err)
	}
	defer store3.Close()
}

func TestFlockReacquireAfterRelease(t *testing.T) {
	tmpDir := t.TempDir()

	for i := 0; i < 3; i++ {
		lock, err := AcquireLock(tmpDir)
		if err != nil {
			t.Fatalf("Failed to acquire lock on iteration %d: %v", i, err)
		}

		// Hold lock briefly
		time.Sleep(10 * time.Millisecond)

		err = lock.Release()
		if err != nil {
			t.Fatalf("Failed to release lock on iteration %d: %v", i, err)
		}
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsInner(s, substr)))
}

func containsInner(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
