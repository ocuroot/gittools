package gitlock

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLockExpiration(t *testing.T) {
	// Set up test repository
	localDir, _, cleanup := setupRemoteTestRepo(t)
	defer cleanup()

	repo, err := New(localDir)
	if err != nil {
		t.Fatalf("Failed to create GitRepo: %v", err)
	}

	// Set lock path
	lockPath := "locks/expiring-resource.lock"
	expiration := 10 * time.Minute

	// Define a fixed base time for testing
	baseTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	// Set the initial time
	repo.now = func() time.Time {
		return baseTime
	}

	// Acquire a lock with 10 minute expiration
	err = repo.AcquireLock(lockPath, expiration, "Test expiring lock")
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// Verify that the lock exists and we own it
	lock, err := repo.ReadLock(lockPath)
	if err != nil {
		t.Fatalf("Failed to read lock: %v", err)
	}
	if lock == nil {
		t.Fatal("Expected lock to exist, got nil")
	}

	ownsLock, err := repo.OwnsLock(lock)
	if err != nil {
		t.Fatalf("Failed to check lock ownership: %v", err)
	}
	if !ownsLock {
		t.Fatalf("Expected to own the lock, but doesn't")
	}

	// Set time to just before expiration (9 minutes 59 seconds after lock creation)
	repo.now = func() time.Time {
		return baseTime.Add(expiration - time.Second)
	}

	// Verify lock is still valid just before expiration
	lock, err = repo.ReadLock(lockPath)
	if err != nil {
		t.Fatalf("Failed to read lock just before expiration: %v", err)
	}
	if lock == nil {
		t.Fatal("Expected lock to still be valid just before expiration, but it was invalid")
	}

	// Set time to after expiration (10 minutes and 1 second after lock creation)
	repo.now = func() time.Time {
		return baseTime.Add(expiration + time.Second)
	}

	// Verify lock is now invalid
	lock, err = repo.ReadLock(lockPath)
	if err != nil {
		t.Fatalf("Failed to read lock after expiration: %v", err)
	}
	if lock != nil {
		t.Fatal("Expected lock to be invalid after expiration, but it was still valid")
	}

	// Verify file still exists on disk even though lock is logically expired
	lockFileFull := filepath.Join(localDir, lockPath)
	if _, _, err := repo.execGitCommand("ls-files", "--", lockFileFull); err != nil {
		t.Fatal("Lock file should still exist on disk even after expiration")
	}

	// Test that we can acquire a new lock after the previous one expired
	// Reset time to "now" (after expiration)
	err = repo.AcquireLock(lockPath, expiration, "New lock after expiration")
	if err != nil {
		t.Fatalf("Failed to acquire new lock after previous one expired: %v", err)
	}

	// Verify that the new lock exists and we own it
	lock, err = repo.ReadLock(lockPath)
	if err != nil {
		t.Fatalf("Failed to read new lock: %v", err)
	}
	if lock == nil {
		t.Fatal("Expected new lock to exist, got nil")
	}
}
