package lock

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ocuroot/gittools"
	"github.com/ocuroot/gittools/testutils"
)

func checkoutRemoteTestRepo(t *testing.T, remoteDir string) (*gittools.Repo, func()) {
	t.Helper()

	// Create a temporary directory for the remote repository
	localDir, err := os.MkdirTemp("", "gitlock-local-")
	if err != nil {
		t.Fatalf("Failed to create local temp directory: %v", err)
	}

	client := gittools.Client{}

	repo, err := client.Clone(fmt.Sprintf("file://%s", remoteDir), localDir)
	if err != nil {
		os.RemoveAll(localDir)
		t.Fatalf("Failed to clone remote repository: %v", err)
	}

	return repo, func() {
		os.RemoveAll(localDir)
	}
}

// setupRemoteTestRepo creates a local repository with a "remote" repository in a temp directory
// Returns: repoPath, remotePath, cleanup function
func setupRemoteTestRepo(t *testing.T) (string, string, func()) {
	t.Helper()

	// Create a bare repository using testutils
	remoteDir, remoteCleanup, err := testutils.CreateTestRemoteRepo("gitlock-test")
	if err != nil {
		t.Fatalf("Failed to create remote repository: %v", err)
	}

	// Clone the remote repository to create the local working copy
	localRepo, localCleanup := checkoutRemoteTestRepo(t, remoteDir)

	// Return the paths and a combined cleanup function
	return localRepo.RepoPath, remoteDir, func() {
		localCleanup()
		remoteCleanup()
	}
}

func TestLockAcquireRelease(t *testing.T) {
	localDir, _, cleanup := setupRemoteTestRepo(t)
	defer cleanup()

	repo, err := gittools.Open(localDir)
	if err != nil {
		t.Fatalf("Failed to create GitRepo: %v", err)
	}

	// Test acquiring a lock
	lockPath := "locks/test-resource.lock"
	absLockPath := filepath.Join(localDir, lockPath)

	// Ensure the directory exists
	lockDir := filepath.Dir(absLockPath)
	err = os.MkdirAll(lockDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create lock directory: %v", err)
	}

	// Debug lock file path
	t.Logf("Lock file path: %s", absLockPath)

	locking := NewRepoLocking(repo)
	err = locking.AcquireLock(lockPath, 10*time.Minute, "Test lock")
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// Check if file exists directly
	if _, err := os.Stat(absLockPath); os.IsNotExist(err) {
		t.Logf("Lock file does not exist after acquire: %s", absLockPath)
	} else {
		t.Logf("Lock file exists after acquire: %s", absLockPath)
	}

	// Check if the lock exists
	lock, err := locking.ReadLock(lockPath)
	if err != nil {
		t.Fatalf("Failed to check lock: %v", err)
	}

	// Check if we own the lock
	ownsLock, err := locking.OwnsLock(lock)
	if err != nil {
		t.Fatalf("Failed to check lock ownership: %v", err)
	}

	if lock == nil {
		t.Fatalf("Expected lock object to be returned, got nil")
	}

	if lock.Owner != locking.LockKey {
		t.Errorf("Expected lock owner to be %s, got %s", locking.LockKey, lock.Owner)
	}

	if !ownsLock {
		t.Errorf("Expected to be the owner of the lock, but was not")
	}
	if lock.Description != "Test lock" {
		t.Errorf("Expected lock description to be 'Test lock', got '%s'", lock.Description)
	}

	// Test refreshing the lock
	originalExpiry := lock.ExpiresAt
	t.Logf("Original expiry: %v", originalExpiry)
	newExpiry := originalExpiry.Add(20 * time.Minute)
	t.Logf("New expiry: %v", newExpiry)
	err = locking.RefreshLock(lockPath, newExpiry)
	if err != nil {
		t.Fatalf("Failed to refresh lock: %v", err)
	}

	// Check if the lock was refreshed
	lock, err = locking.ReadLock(lockPath)
	if err != nil {
		t.Fatalf("Failed to check lock after refresh: %v", err)
	}
	t.Logf("Lock after refresh: %+v", lock)
	if !lock.ExpiresAt.After(originalExpiry) {
		t.Errorf("Expected expiry time to be extended, but it wasn't")
	}

	// Test releasing the lock
	err = locking.ReleaseLock(lockPath)
	if err != nil {
		t.Fatalf("Failed to release lock: %v", err)
	}

	// Check if the lock was released
	lock, err = locking.ReadLock(lockPath)
	if err != nil {
		t.Fatalf("Failed to check lock after release: %v", err)
	}
	if lock != nil {
		t.Errorf("Expected lock to be released, but it still exists")
	}
}
