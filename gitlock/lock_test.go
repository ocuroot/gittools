package gitlock

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func checkoutRemoteTestRepo(t *testing.T, remoteDir string) (*GitRepo, func()) {
	t.Helper()

	// Create a temporary directory for the remote repository
	localDir, err := os.MkdirTemp("", "gitlock-local-")
	if err != nil {
		t.Fatalf("Failed to create local temp directory: %v", err)
	}

	repo, err := Clone(fmt.Sprintf("file://%s", remoteDir), localDir)
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

	// Create a temporary directory for the remote repository
	remoteDir, err := os.MkdirTemp("", "gitlock-remote-")
	if err != nil {
		t.Fatalf("Failed to create remote temp directory: %v", err)
	}

	// Initialize bare git repository for the remote
	cmd := exec.Command("git", "init", "--bare", remoteDir)
	if err := cmd.Run(); err != nil {
		os.RemoveAll(remoteDir)
		t.Fatalf("Failed to initialize bare git repository: %v", err)
	}

	// Set main as the default branch for the remote
	cmd = exec.Command("git", "symbolic-ref", "HEAD", "refs/heads/main")
	cmd.Dir = remoteDir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(remoteDir)
		t.Fatalf("Failed to set default branch for remote: %v", err)
	}

	// Create a temporary directory for the local repository
	localDir, err := os.MkdirTemp("", "gitlock-local-")
	if err != nil {
		os.RemoveAll(remoteDir)
		t.Fatalf("Failed to create local temp directory: %v", err)
	}

	// Initialize git repository
	cmd = exec.Command("git", "init", localDir)
	if err := cmd.Run(); err != nil {
		os.RemoveAll(remoteDir)
		os.RemoveAll(localDir)
		t.Fatalf("Failed to initialize git repository: %v", err)
	}

	// Set git config for the test repository
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = localDir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(remoteDir)
		os.RemoveAll(localDir)
		t.Fatalf("Failed to set git config user.name: %v", err)
	}

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = localDir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(remoteDir)
		os.RemoveAll(localDir)
		t.Fatalf("Failed to set git config user.email: %v", err)
	}

	// Create an initial commit
	readme := filepath.Join(localDir, "README.md")
	if err := os.WriteFile(readme, []byte("# Test Repository\n"), 0644); err != nil {
		os.RemoveAll(remoteDir)
		os.RemoveAll(localDir)
		t.Fatalf("Failed to write README.md: %v", err)
	}

	cmd = exec.Command("git", "add", "README.md")
	cmd.Dir = localDir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(remoteDir)
		os.RemoveAll(localDir)
		t.Fatalf("Failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = localDir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(remoteDir)
		os.RemoveAll(localDir)
		t.Fatalf("Failed to git commit: %v", err)
	}

	// Rename the branch to main
	cmd = exec.Command("git", "branch", "-M", "main")
	cmd.Dir = localDir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(remoteDir)
		os.RemoveAll(localDir)
		t.Fatalf("Failed to rename branch: %v", err)
	}

	// Add the remote
	cmd = exec.Command("git", "remote", "add", "origin", remoteDir)
	cmd.Dir = localDir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(remoteDir)
		os.RemoveAll(localDir)
		t.Fatalf("Failed to add remote: %v", err)
	}

	// Push to the remote
	cmd = exec.Command("git", "push", "-u", "origin", "main")
	cmd.Dir = localDir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(remoteDir)
		os.RemoveAll(localDir)
		t.Fatalf("Failed to push to remote: %v", err)
	}

	return localDir, remoteDir, func() {
		os.RemoveAll(localDir)
		os.RemoveAll(remoteDir)
	}
}

func TestLockAcquireRelease(t *testing.T) {
	localDir, _, cleanup := setupRemoteTestRepo(t)
	defer cleanup()

	repo, err := New(localDir)
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

	err = repo.AcquireLock(lockPath, 10*time.Minute, "Test lock")
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
	lock, err := repo.ReadLock(lockPath)
	if err != nil {
		t.Fatalf("Failed to check lock: %v", err)
	}

	// Check if we own the lock
	ownsLock, err := repo.OwnsLock(lock)
	if err != nil {
		t.Fatalf("Failed to check lock ownership: %v", err)
	}

	// Add debug output to diagnose the issue
	t.Logf("Owns lock: %v", ownsLock)
	t.Logf("Lock: %+v", lock)
	t.Logf("Repo LockKey: %s", repo.LockKey)

	if lock == nil {
		t.Fatalf("Expected lock object to be returned, got nil")
	}

	if lock.Owner != repo.LockKey {
		t.Errorf("Expected lock owner to be %s, got %s", repo.LockKey, lock.Owner)
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
	err = repo.RefreshLock(lockPath, newExpiry)
	if err != nil {
		t.Fatalf("Failed to refresh lock: %v", err)
	}

	// Check if the lock was refreshed
	lock, err = repo.ReadLock(lockPath)
	if err != nil {
		t.Fatalf("Failed to check lock after refresh: %v", err)
	}
	t.Logf("Lock after refresh: %+v", lock)
	if !lock.ExpiresAt.After(originalExpiry) {
		t.Errorf("Expected expiry time to be extended, but it wasn't")
	}

	// Test releasing the lock
	err = repo.ReleaseLock(lockPath)
	if err != nil {
		t.Fatalf("Failed to release lock: %v", err)
	}

	// Check if the lock was released
	lock, err = repo.ReadLock(lockPath)
	if err != nil {
		t.Fatalf("Failed to check lock after release: %v", err)
	}
	if lock != nil {
		t.Errorf("Expected lock to be released, but it still exists")
	}
}
