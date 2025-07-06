package lock

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ocuroot/gittools"
)

func TestLockExpiration(t *testing.T) {
	// Use the SafeTest helper to ensure we're working in a temporary directory
	gittools.SafeTest(t, func(t *testing.T, tempDir string) {
		// Set up test repository
		localDir, _, cleanup := setupRemoteTestRepo(t)
		defer cleanup()

		// Properly open the repo with a client rather than creating it directly
		var err error
		repo, err := gittools.Open(localDir)
		if err != nil {
			t.Fatalf("Failed to open repository: %v", err)
		}

		// Set lock path - ensure it's scoped to the temp directory
		lockPath := "locks/expiring-resource.lock"
		
		// Create the locks directory within the repository
		locksDir := filepath.Join(localDir, "locks")
		if err := os.MkdirAll(locksDir, 0755); err != nil {
			t.Fatalf("Failed to create locks directory: %v", err)
		}
		
		// Add a .gitkeep file to ensure the directory is tracked
		gitkeepPath := filepath.Join(locksDir, ".gitkeep")
		if err := os.WriteFile(gitkeepPath, []byte{}, 0644); err != nil {
			t.Fatalf("Failed to create .gitkeep file: %v", err)
		}

		// Commit the .gitkeep file
		err = repo.Commit("Add .gitkeep file", []string{"locks/.gitkeep"})
		if err != nil {
			t.Fatalf("Failed to commit .gitkeep file: %v", err)
		}
		
		expiration := 10 * time.Minute

		locking := NewRepoLocking(repo)

		// Define a fixed base time for testing
		baseTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

		// Set the initial time
		locking.now = func() time.Time {
			return baseTime
		}

		// Acquire a lock with 10 minute expiration
		err = locking.AcquireLock(lockPath, expiration, "Test expiring lock")
		if err != nil {
			t.Fatalf("Failed to acquire lock: %v", err)
		}

		// Verify that the lock exists and we own it
		lock, err := locking.ReadLock(lockPath)
		if err != nil {
			t.Fatalf("Failed to read lock: %v", err)
		}
		if lock == nil {
			t.Fatal("Expected lock to exist, got nil")
		}

		ownsLock, err := locking.OwnsLock(lock)
		if err != nil {
			t.Fatalf("Failed to check lock ownership: %v", err)
		}
		if !ownsLock {
			t.Fatalf("Expected to own the lock, but doesn't")
		}

		// Set time to just before expiration (9 minutes 59 seconds after lock creation)
		locking.now = func() time.Time {
			return baseTime.Add(expiration - time.Second)
		}

		// Verify lock is still valid just before expiration
		lock, err = locking.ReadLock(lockPath)
		if err != nil {
			t.Fatalf("Failed to read lock just before expiration: %v", err)
		}
		if lock == nil {
			t.Fatal("Expected lock to still be valid just before expiration, but it was invalid")
		}

		// Set time to after expiration (10 minutes and 1 second after lock creation)
		locking.now = func() time.Time {
			return baseTime.Add(expiration + time.Second)
		}

		// Verify lock is now invalid
		lock, err = locking.ReadLock(lockPath)
		if err != nil {
			t.Fatalf("Failed to read lock after expiration: %v", err)
		}
		if lock != nil {
			t.Fatal("Expected lock to be invalid after expiration, but it was still valid")
		}

		// Test that we can acquire a new lock after the previous one expired
		// Reset time to "now" (after expiration)
		err = locking.AcquireLock(lockPath, expiration, "New lock after expiration")
		if err != nil {
			t.Fatalf("Failed to acquire new lock after previous one expired: %v", err)
		}

		// Verify that the new lock exists and we own it
		lock, err = locking.ReadLock(lockPath)
		if err != nil {
			t.Fatalf("Failed to read new lock: %v", err)
		}
		if lock == nil {
			t.Fatal("Expected new lock to exist, got nil")
		}
	})
}
