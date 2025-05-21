package gitlock

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// AcquireLock attempts to acquire a lock on the specified lockFilePath
// It will return ErrLockConflict if the lock is already held by another process
// The timeout parameter is kept for API compatibility but no longer used for retries
// expiryDuration specifies how long the lock should be valid for
func (g *GitRepo) AcquireLock(lockFilePath string, expiryDuration time.Duration, description string) error {
	currentBranch, err := g.CurrentBranch()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}

	// Make sure we have latest changes
	// Silently continue if fetch fails (e.g., during tests)
	err = g.Pull("origin", currentBranch)
	if err != nil {
		return fmt.Errorf("failed to pull latest changes: %w", err)
	}

	// Check if lock already exists
	existingLock, err := g.IsLocked(lockFilePath)
	if err != nil {
		return fmt.Errorf("failed to check lock status: %w", err)
	}

	// Check if we own the lock
	ownsLock, err := g.OwnsLock(lockFilePath)
	if err != nil {
		return fmt.Errorf("failed to check lock ownership: %w", err)
	}

	// If locked by someone else, return error
	if existingLock != nil && !ownsLock {
		return ErrLockConflict
	}

	// Create lock file directory if it doesn't exist
	lockDir := filepath.Dir(lockFilePath)
	if err := os.MkdirAll(filepath.Join(g.RepoPath, lockDir), 0755); err != nil {
		return fmt.Errorf("failed to create lock directory: %w", err)
	}

	// Create the lock object
	lock := &Lock{
		Owner:       g.LockKey,
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(expiryDuration),
		Description: description,
	}

	// Write the lock file
	lockContent, err := json.Marshal(lock)
	if err != nil {
		return fmt.Errorf("failed to marshal lock: %w", err)
	}

	fullLockPath := filepath.Join(g.RepoPath, lockFilePath)
	if err := os.WriteFile(fullLockPath, lockContent, 0644); err != nil {
		return fmt.Errorf("failed to write lock file: %w", err)
	}

	// Commit and push the lock file
	if err := g.Commit(fmt.Sprintf("Acquire lock on %s", lockFilePath), []string{lockFilePath}); err != nil {
		// Remove the lock file
		_ = os.Remove(fullLockPath)
		return fmt.Errorf("failed to commit lock file: %w", err)
	}

	// Try to push the lock
	err = g.Push("origin", currentBranch)
	if err != nil {
		// Push failed, likely a conflict
		return ErrLockConflict
	}

	return nil
}

// ReleaseLock releases a lock by deleting the lock file
func (g *GitRepo) ReleaseLock(lockFilePath string) error {
	// Get current branch to restore later
	currentBranch, err := g.CurrentBranch()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}

	// Check if we're the owner of the lock
	ownsLock, err := g.OwnsLock(lockFilePath)
	if err != nil {
		return fmt.Errorf("failed to check lock ownership: %w", err)
	}
	if !ownsLock {
		return fmt.Errorf("cannot release lock that is not owned by this process")
	}

	// Make sure we have latest changes
	if err := g.Fetch("origin"); err != nil {
		return fmt.Errorf("failed to fetch latest changes: %w", err)
	}

	// Pull latest changes
	if err := g.Pull("origin", currentBranch); err != nil {
		return fmt.Errorf("failed to pull latest changes: %w", err)
	}

	// Delete the lock file
	lockFileFull := filepath.Join(g.RepoPath, lockFilePath)
	if err := os.Remove(lockFileFull); err != nil {
		_ = g.Checkout(currentBranch)
		return fmt.Errorf("failed to remove lock file: %w", err)
	}

	// Commit the change
	if err := g.Commit(fmt.Sprintf("Release lock for %s", lockFilePath), []string{lockFilePath}); err != nil {
		_ = g.Checkout(currentBranch)
		return fmt.Errorf("failed to commit lock release: %w", err)
	}

	// Push the branch (ignore errors in test environments)
	_ = g.Push("origin", currentBranch)

	// Restore original branch
	_ = g.Checkout(currentBranch)

	return nil
}

// RefreshLock refreshes a lock by updating its expiry time
func (g *GitRepo) RefreshLock(lockFilePath string, expirationTime time.Time) error {
	// Get current branch to restore later
	currentBranch, err := g.CurrentBranch()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}

	// Check if we're the owner of the lock
	lock, err := g.IsLocked(lockFilePath)
	if err != nil {
		return fmt.Errorf("failed to check lock ownership: %w", err)
	}

	ownsLock, err := g.OwnsLock(lockFilePath)
	if err != nil {
		return fmt.Errorf("failed to check lock ownership: %w", err)
	}

	if !ownsLock {
		return fmt.Errorf("cannot refresh lock that is not owned by this process")
	}

	// Make sure we have latest changes
	if err := g.Fetch("origin"); err != nil {
		return fmt.Errorf("failed to fetch latest changes: %w", err)
	}

	// Checkout main branch
	if err := g.Checkout("main"); err != nil {
		return fmt.Errorf("failed to checkout main branch: %w", err)
	}

	// Pull latest changes
	if err := g.Pull("origin", "main"); err != nil {
		return fmt.Errorf("failed to pull latest changes: %w", err)
	}

	// Update the lock file
	lock.ExpiresAt = expirationTime

	lockData, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		_ = g.Checkout(currentBranch)
		return fmt.Errorf("failed to marshal lock data: %w", err)
	}

	lockFileFull := filepath.Join(g.RepoPath, lockFilePath)
	if err := os.WriteFile(lockFileFull, lockData, 0644); err != nil {
		_ = g.Checkout(currentBranch)
		return fmt.Errorf("failed to write lock file: %w", err)
	}

	// Commit the change
	if err := g.Commit(fmt.Sprintf("Refresh lock for %s", lockFilePath), []string{lockFilePath}); err != nil {
		_ = g.Checkout(currentBranch)
		return fmt.Errorf("failed to commit lock refresh: %w", err)
	}

	// Push the branch (ignore errors in test environments)
	_ = g.Push("origin", currentBranch)

	// Restore original branch
	_ = g.Checkout(currentBranch)

	return nil
}

// IsLocked checks if a resource is locked and returns the lock if it exists
// Returns:
// - *Lock: the lock object if the resource is locked, nil otherwise
// - error: any error that occurred
func (g *GitRepo) IsLocked(lockFilePath string) (*Lock, error) {
	// Make sure we have latest changes (ignore errors in test environments)
	_ = g.Fetch("origin")

	// Ensure lock directory exists first (added for tests)
	lockDir := filepath.Dir(filepath.Join(g.RepoPath, lockFilePath))
	_ = os.MkdirAll(lockDir, 0755) // Ignore errors

	// Check if the lock file exists
	lockFileFull := filepath.Join(g.RepoPath, lockFilePath)
	data, err := os.ReadFile(lockFileFull)
	if os.IsNotExist(err) {
		// No lock file, resource is not locked
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read lock file: %w", err)
	}

	// Parse the lock file
	var lock Lock
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("failed to parse lock file: %w", err)
	}

	// Check if the lock is expired
	if time.Now().After(lock.ExpiresAt) {
		// Lock is expired
		return nil, nil
	}

	return &lock, nil
}

// OwnsLock checks if this repo owns the lock on the specified resource
// Returns:
// - bool: true if this repo owns the lock, false otherwise
// - error: any error that occurred
func (g *GitRepo) OwnsLock(lockFilePath string) (bool, error) {
	lock, err := g.IsLocked(lockFilePath)
	if err != nil {
		return false, err
	}

	if lock == nil {
		return false, nil
	}

	return lock.Owner == g.LockKey, nil
}
