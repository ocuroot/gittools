package lock

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ocuroot/gittools"
)

func checkoutRemoteTestRepo(t *testing.T, remoteDir string) (*gittools.Repo, func()) {
	t.Helper()
	
	// Safety check: Get the current working directory
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}
	
	// Get parent directories to check against
	parentDir := filepath.Dir(cwd)
	sourceRepoDir := filepath.Dir(parentDir)

	// Create a temporary directory for the remote repository
	localDir, err := os.MkdirTemp("", "gitlock-local-")
	if err != nil {
		t.Fatalf("Failed to create local temp directory: %v", err)
	}
	
	// Safety check: Ensure we're not using source repo dir
	if strings.Contains(localDir, sourceRepoDir) || strings.Contains(remoteDir, sourceRepoDir) {
		// Clean up the directory we just created
		os.RemoveAll(localDir)
		t.Fatalf("CRITICAL SAFETY ERROR: Test trying to use source repo directory or subdirectory")
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

	// Safety check: Get the current working directory
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}
	
	// Create a bare repository using testutils
	remoteDir, remoteCleanup, err := gittools.CreateTestRemoteRepo("gitlock-test")
	if err != nil {
		t.Fatalf("Failed to create remote repository: %v", err)
	}
	
	// Safety check: Ensure remoteDir is not the source repo
	if strings.Contains(remoteDir, cwd) {
		t.Fatalf("CRITICAL SAFETY ERROR: Test trying to use source repo. Remote dir: %s contains CWD: %s", remoteDir, cwd)
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
	// Use the SafeTest helper to ensure we're working in a temporary directory
	gittools.SafeTest(t, func(t *testing.T, tempDir string) {
		// Setup remote repository
		localDir, _, cleanup := setupRemoteTestRepo(t)
		defer cleanup()

		// Open the repo properly to initialize the client
		var err error
		repo, err := gittools.Open(localDir)
		if err != nil {
			t.Fatalf("Failed to open repository: %v", err)
		}
		
		// Create repo locking
		locking := NewRepoLocking(repo)
		locking.LockKey = "test-owner"

		// Create the locks directory if it doesn't exist
		locksDir := filepath.Join(localDir, "locks")
		if err := os.MkdirAll(locksDir, 0755); err != nil {
			t.Fatalf("Failed to create locks directory: %v", err)
		}

		// Add a .gitkeep file and commit it to ensure directory is tracked
		gitkeepPath := filepath.Join(locksDir, ".gitkeep")
		if err := os.WriteFile(gitkeepPath, []byte{}, 0644); err != nil {
			t.Fatalf("Failed to create .gitkeep file: %v", err)
		}

		// Commit the .gitkeep file
		err = repo.Commit("Add .gitkeep file", []string{"locks/.gitkeep"})
		if err != nil {
			t.Fatalf("Failed to commit .gitkeep file: %v", err)
		}

		// Test acquiring a lock
		// Use a relative path that will be scoped to the temporary repo
		lockPath := "locks/test-resource.lock"
		absLockPath := filepath.Join(localDir, lockPath)

		// Debug lock file path
		t.Logf("Lock file path: %s", absLockPath)

		err = locking.AcquireLock(lockPath, 10*time.Minute, "Test lock")
		if err != nil {
			t.Fatalf("Failed to acquire lock: %v", err)
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
	})
}
