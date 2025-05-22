package lock

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ocuroot/gittools"
	"github.com/oklog/ulid/v2"
)

// Lock represents a lock on a resource
type Lock struct {
	Owner       string    `json:"owner"` // ULID of the process holding the lock
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	Description string    `json:"description,omitempty"`
}

func NewRepoLocking(repo *gittools.Repo) *Locking {
	return &Locking{
		repo:    repo,
		LockKey: ulid.Make().String(),
		now: func() time.Time {
			return time.Now()
		},
	}
}

type Locking struct {
	repo    *gittools.Repo
	LockKey string // ULID for identifying this process
	now     func() time.Time
}

// AcquireLock attempts to acquire a lock on the specified lockFilePath
// It will return ErrLockConflict if the lock is already held by another process
// The timeout parameter is kept for API compatibility but no longer used for retries
// expiryDuration specifies how long the lock should be valid for
func (g *Locking) AcquireLock(lockFilePath string, expiryDuration time.Duration, description string) error {
	currentBranch, err := g.repo.CurrentBranch()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}

	// Make sure we have latest changes
	// Silently continue if fetch fails (e.g., during tests)
	err = g.repo.Pull("origin", currentBranch)
	if err != nil {
		return fmt.Errorf("failed to pull latest changes: %w", err)
	}

	// Check if lock already exists
	existingLock, err := g.ReadLock(lockFilePath)
	if err != nil {
		return fmt.Errorf("failed to check lock status: %w", err)
	}

	// Check if we own the lock
	ownsLock, err := g.OwnsLock(existingLock)
	if err != nil {
		return fmt.Errorf("failed to check lock ownership: %w", err)
	}

	// If locked by someone else, return error
	if existingLock != nil && !ownsLock {
		return ErrLockConflict
	}

	// Create lock file directory if it doesn't exist
	lockDir := filepath.Dir(lockFilePath)
	if err := os.MkdirAll(filepath.Join(g.repo.RepoPath, lockDir), 0755); err != nil {
		return fmt.Errorf("failed to create lock directory: %w", err)
	}

	// Create the lock object
	lock := &Lock{
		Owner:       g.LockKey,
		CreatedAt:   g.now(),
		ExpiresAt:   g.now().Add(expiryDuration),
		Description: description,
	}

	// Write the lock file
	lockContent, err := json.Marshal(lock)
	if err != nil {
		return fmt.Errorf("failed to marshal lock: %w", err)
	}

	fullLockPath := filepath.Join(g.repo.RepoPath, lockFilePath)
	if err := os.WriteFile(fullLockPath, lockContent, 0644); err != nil {
		return fmt.Errorf("failed to write lock file: %w", err)
	}

	// Commit and push the lock file
	if err := g.repo.Commit(fmt.Sprintf("Acquire lock on %s", lockFilePath), []string{lockFilePath}); err != nil {
		// Remove the lock file
		_ = os.Remove(fullLockPath)
		return fmt.Errorf("failed to commit lock file: %w", err)
	}

	// Try to push the lock with retries for conflicts
	pushErr := g.pushWithRetry(currentBranch)

	if pushErr != nil {
		// If push failed, clean up by removing the lock file and reset
		_ = os.Remove(fullLockPath)
		_ = g.repo.ResetHard("HEAD~1")

		// Convert the push error to an appropriate lock error
		switch {
		case errors.Is(pushErr, gittools.ErrPushNonFastForward), errors.Is(pushErr, gittools.ErrPushRejected):
			// These errors typically mean someone else has pushed changes
			return fmt.Errorf("%w: %v", ErrLockConflict, pushErr)
		case errors.Is(pushErr, gittools.ErrRebaseMergeConflict):
			// Rebase merge conflict means lock contention
			return fmt.Errorf("%w: %v", ErrLockConflict, pushErr)
		case errors.Is(pushErr, gittools.ErrPushPermissionDenied):
			return fmt.Errorf("lock acquisition failed due to permission issues: %w", pushErr)
		case errors.Is(pushErr, gittools.ErrPushRemoteRefMissing):
			return fmt.Errorf("lock acquisition failed due to missing remote reference: %w", pushErr)
		default:
			// For any other errors, return a lock conflict
			return fmt.Errorf("%w: %v", ErrLockConflict, pushErr)
		}
	}

	return nil
}

// ReleaseLock releases a lock by deleting the lock file
func (g *Locking) ReleaseLock(lockFilePath string) error {
	// Get current branch to restore later
	currentBranch, err := g.repo.CurrentBranch()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}

	if err := g.repo.Pull("origin", currentBranch); err != nil {
		return fmt.Errorf("failed to pull latest changes: %w", err)
	}

	lock, err := g.ReadLock(lockFilePath)
	if err != nil {
		return fmt.Errorf("failed to check lock: %w", err)
	}

	// Check if we're the owner of the lock
	ownsLock, err := g.OwnsLock(lock)
	if err != nil {
		return fmt.Errorf("failed to check lock ownership: %w", err)
	}
	if !ownsLock {
		lock, err := g.ReadLock(lockFilePath)
		if err != nil {
			return fmt.Errorf("failed to check lock: %w", err)
		}
		return fmt.Errorf("cannot release lock that is not owned by this process: lock owner %s", lock.Owner)
	}

	// Make sure we have latest changes
	if err := g.repo.Fetch("origin"); err != nil {
		return fmt.Errorf("failed to fetch latest changes: %w", err)
	}

	// Pull latest changes
	if err := g.repo.Pull("origin", currentBranch); err != nil {
		return fmt.Errorf("failed to pull latest changes: %w", err)
	}

	// Delete the lock file
	lockFileFull := filepath.Join(g.repo.RepoPath, lockFilePath)
	if err := os.Remove(lockFileFull); err != nil {
		return fmt.Errorf("failed to remove lock file: %w", err)
	}

	// Commit the change
	if err := g.repo.Commit(fmt.Sprintf("Release lock for %s", lockFilePath), []string{lockFilePath}); err != nil {
		return fmt.Errorf("failed to commit lock release: %w", err)
	}

	// Push the branch with retries
	pushErr := g.pushWithRetry(currentBranch)

	if pushErr != nil {
		// If push failed, revert our change by restoring the lock file and reset
		if err := g.repo.ResetHard("HEAD~1"); err != nil {
			fmt.Printf("Warning: failed to reset after push error: %v\n", err)
		}

		// Return a descriptive error
		return fmt.Errorf("failed to push lock release: %w", pushErr)
	}

	return nil
}

// pushWithRetry attempts to push to origin with retry logic using Git rebase
// for handling non-fast-forward conflicts
func (g *Locking) pushWithRetry(branch string) error {
	const maxRetries = 2
	var lastErr error

	for retry := 0; retry <= maxRetries; retry++ {
		// Try to push
		lastErr = g.repo.Push("origin", branch)
		if lastErr == nil {
			// Push succeeded
			return nil
		}

		// Only retry for non-fast-forward or fetch-first errors
		if retry < maxRetries && (errors.Is(lastErr, gittools.ErrPushNonFastForward) || errors.Is(lastErr, gittools.ErrPushFetchFirst)) {
			// First fetch the latest changes
			fetchErr := g.repo.Fetch("origin")
			if fetchErr != nil {
				// Failed to fetch, continue to next retry attempt
				continue
			}

			// Make sure we're not in a rebase already
			if err := g.repo.RebaseAbort(); err != nil {
				// Failed to abort rebase, continue to next retry attempt
				continue
			}

			// Try to rebase our changes on top of the remote
			rebaseErr := g.repo.Rebase("refs/remotes/origin/" + branch)
			if rebaseErr != nil {
				// If rebase fails for any reason, abort it and stop retrying
				if err := g.repo.RebaseAbort(); err != nil {
					fmt.Printf("Warning: failed to abort rebase after rebase error: %v\n", err)
				}
				// For a locking mechanism, a rebase failure indicates true contention
				// Set the last error to the rebase error and break out completely
				lastErr = rebaseErr
				break
			}

			// Continue to next retry attempt after rebase
			continue
		} else {
			// For any other errors, stop trying
			break
		}
	}

	// If we've reached here, the push failed after all retries
	return lastErr
}

// RefreshLock refreshes a lock by updating its expiry time
func (g *Locking) RefreshLock(lockFilePath string, expirationTime time.Time) error {
	// Get current branch to restore later
	currentBranch, err := g.repo.CurrentBranch()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}

	// Check if we're the owner of the lock
	lock, err := g.ReadLock(lockFilePath)
	if err != nil {
		return fmt.Errorf("failed to check lock ownership: %w", err)
	}

	ownsLock, err := g.OwnsLock(lock)
	if err != nil {
		return fmt.Errorf("failed to check lock ownership: %w", err)
	}

	if !ownsLock {
		return fmt.Errorf("cannot refresh lock that is not owned by this process")
	}

	// Read the original lock content first so we can restore it if needed
	lockFileFull := filepath.Join(g.repo.RepoPath, lockFilePath)
	originalLockContent, err := os.ReadFile(lockFileFull)
	if err != nil {
		_ = g.repo.Checkout(currentBranch)
		return fmt.Errorf("failed to read original lock file: %w", err)
	}

	// Update the lock file
	lock.ExpiresAt = expirationTime

	lockData, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal lock data: %w", err)
	}

	if err := os.WriteFile(lockFileFull, lockData, 0644); err != nil {
		return fmt.Errorf("failed to write lock file: %w", err)
	}

	// Commit the change
	if err := g.repo.Commit(fmt.Sprintf("Refresh lock for %s", lockFilePath), []string{lockFilePath}); err != nil {
		return fmt.Errorf("failed to commit lock refresh: %w", err)
	}

	// Push the branch with retries
	pushErr := g.pushWithRetry(currentBranch)

	if pushErr != nil {
		// If push failed, revert our change by restoring the lock file and reset
		_ = os.WriteFile(lockFileFull, originalLockContent, 0644)
		if err := g.repo.ResetHard("HEAD~1"); err != nil {
			fmt.Printf("Warning: failed to reset after push error: %v\n", err)
		}

		// Return a descriptive error
		return fmt.Errorf("failed to push lock refresh: %w", pushErr)
	}

	return nil
}

// ReadLock checks if a resource is locked and returns the lock if it exists
// Returns:
// - *Lock: the lock object if the resource is locked, nil otherwise
// - error: any error that occurred
func (g *Locking) ReadLock(lockFilePath string) (*Lock, error) {
	// Ensure lock directory exists first (added for tests)
	lockDir := filepath.Dir(filepath.Join(g.repo.RepoPath, lockFilePath))
	_ = os.MkdirAll(lockDir, 0755) // Ignore errors

	// Check if the lock file exists
	lockFileFull := filepath.Join(g.repo.RepoPath, lockFilePath)
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
	if g.now().After(lock.ExpiresAt) {
		// Lock is expired
		return nil, nil
	}

	return &lock, nil
}

// OwnsLock checks if this repo owns the lock on the specified resource
// Returns:
// - bool: true if this repo owns the lock, false otherwise
// - error: any error that occurred
func (g *Locking) OwnsLock(lock *Lock) (bool, error) {
	if lock == nil {
		return false, nil
	}

	return lock.Owner == g.LockKey, nil
}
