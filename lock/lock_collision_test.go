package lock

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudflare/backoff"
	"github.com/ocuroot/gittools"
)

// TestLockCollision tests the lock collision detection and timeout functionality
func TestLockCollision(t *testing.T) {
	// Use the SafeTest helper to ensure we're working in a temporary directory
	gittools.SafeTest(t, func(t *testing.T, tempDir string) {
		// Setup test repository
		localDir, _, cleanup := setupRemoteTestRepo(t)
		defer cleanup()

		// Create two separate repos pointing to the same directory
		// to simulate two different processes
		repo1, err := gittools.Open(localDir)
		if err != nil {
			t.Fatalf("Failed to create first GitRepo: %v", err)
		}

		repo2, err := gittools.Open(localDir)
		if err != nil {
			t.Fatalf("Failed to create second GitRepo: %v", err)
		}

		repo1Locking := NewRepoLocking(repo1)
		repo2Locking := NewRepoLocking(repo2)

		// Verify the repos have different lock keys
		if repo1Locking.LockKey == repo2Locking.LockKey {
			t.Fatalf("Expected different lock keys for the two repos, but they match: %s", repo1Locking.LockKey)
		}
		t.Logf("Repo1 lock key: %s", repo1Locking.LockKey)
		t.Logf("Repo2 lock key: %s", repo2Locking.LockKey)

		// Ensure the locks directory exists
		lockPath := "locks/test-resource.lock"
		lockDir := filepath.Join(localDir, "locks")
		if err := os.MkdirAll(lockDir, 0755); err != nil {
			t.Fatalf("Failed to create locks directory: %v", err)
		}
		
		// Add a .gitkeep file and commit it to ensure directory is tracked
		gitkeepPath := filepath.Join(lockDir, ".gitkeep")
		if err := os.WriteFile(gitkeepPath, []byte{}, 0644); err != nil {
			t.Fatalf("Failed to create .gitkeep file: %v", err)
		}

		// Commit the .gitkeep file
		err = repo1.Commit("Add .gitkeep file", []string{"locks/.gitkeep"})
		if err != nil {
			t.Fatalf("Failed to commit .gitkeep file: %v", err)
		}
		
		// Pull in the commit to repo2 so both repos are in sync
		currentBranch, err := repo2.CurrentBranch()
		if err != nil {
			t.Fatalf("Failed to get current branch: %v", err)
		}
		
		err = repo2.Pull("origin", currentBranch)
		if err != nil {
			t.Fatalf("Failed to pull changes into repo2: %v", err)
		}

		// Have repo1 acquire the lock
		err = repo1Locking.AcquireLock(lockPath, 10*time.Minute, "Test lock from repo1")
		if err != nil {
			t.Fatalf("Failed to acquire lock with repo1: %v", err)
		}
		t.Log("Repo1 successfully acquired the lock")

		// Verify repo1 holds the lock
		lock, err := repo1Locking.ReadLock(lockPath)
		if err != nil {
			t.Fatalf("Failed to check lock with repo1: %v", err)
		}

		ownsLock, err := repo1Locking.OwnsLock(lock)
		if err != nil {
			t.Fatalf("Failed to check lock ownership with repo1: %v", err)
		}

		if !ownsLock {
			t.Errorf("Expected repo1 to be the owner of the lock, but it's not")
		}
		if lock == nil {
			t.Fatalf("Expected lock object to be returned for repo1, got nil")
		}
		t.Logf("Lock owner: %s", lock.Owner)
		t.Logf("Lock description: %s", lock.Description)

		// Now try to acquire the same lock with repo2 with a very short timeout
		// This should fail with a timeout error
		start := time.Now()
		err = repo2Locking.AcquireLock(lockPath, 10*time.Minute, "Test lock from repo2")
		elapsed := time.Since(start)

		// Check that we got the expected error
		if err != ErrLockConflict {
			t.Errorf("Expected ErrLockConflict when acquiring locked resource, got: %v", err)
		} else {
			t.Log("Correctly received ErrLockConflict when trying to acquire an already locked resource")
		}

		// With simplified implementation, no waiting is expected
		t.Logf("Lock acquisition attempt took %v", elapsed)

		// Now have repo1 release the lock
		err = repo1Locking.ReleaseLock(lockPath)
		if err != nil {
			t.Fatalf("Failed to release lock: %v", err)
		}
		t.Log("Repo1 released the lock")

		// Now repo2 should be able to acquire the lock
		err = repo2Locking.AcquireLock(lockPath, 10*time.Minute, "Test lock from repo2 after release")
		if err != nil {
			t.Errorf("Failed to acquire lock with repo2 after release: %v", err)
		} else {
			t.Log("Repo2 successfully acquired the lock after repo1 released it")
		}

		// Verify repo2 holds the lock
		lock, err = repo2Locking.ReadLock(lockPath)
		if err != nil {
			t.Fatalf("Failed to check lock with repo2: %v", err)
		}

		ownsLock, err = repo2Locking.OwnsLock(lock)
		if err != nil {
			t.Fatalf("Failed to check lock ownership with repo2: %v", err)
		}

		if !ownsLock {
			t.Errorf("Expected repo2 to be the owner of the lock, but it's not")
		}
		if lock == nil {
			t.Fatalf("Expected lock object to be returned for repo2, got nil")
		}
		if lock.Owner != repo2Locking.LockKey {
			t.Errorf("Expected lock owner to be %s, got %s", repo2Locking.LockKey, lock.Owner)
		}
		t.Logf("Lock now owned by: %s", lock.Owner)
		t.Logf("Lock description: %s", lock.Description)
	})
}

func TestCheckoutRemote(t *testing.T) {
	// Use the SafeTest helper to ensure we're working in a temporary directory
	gittools.SafeTest(t, func(t *testing.T, tempDir string) {
		_, remoteDir, cleanup := setupRemoteTestRepo(t)
		defer cleanup()

		repo, cleanup2 := checkoutRemoteTestRepo(t, remoteDir)
		defer cleanup2()

		branch, err := repo.CurrentBranch()
		if err != nil {
			t.Fatalf("Failed to get current branch: %v", err)
		}

		if branch != "master" {
			t.Errorf("Expected current branch to be 'master', got '%s'", branch)
		}
	})
}

func TestLockConcurrentWork(t *testing.T) {
	// Use the SafeTest helper to ensure we're working in a temporary directory
	gittools.SafeTest(t, func(t *testing.T, tempDir string) {
		_, remoteDir, cleanup := setupRemoteTestRepo(t)
		defer cleanup()

		// Create the first repo and set up the locks directory with a .gitkeep file
		initialRepo, cleanupInitial := checkoutRemoteTestRepo(t, remoteDir)
		defer cleanupInitial()

		// Ensure the locks directory exists in the initial repo and add .gitkeep
		lockDir := filepath.Join(initialRepo.RepoPath, "locks")
		if err := os.MkdirAll(lockDir, 0755); err != nil {
			t.Fatalf("Failed to create locks directory in initial repo: %v", err)
		}

		// Add a .gitkeep file and commit it to ensure directory is tracked
		gitkeepPath := filepath.Join(lockDir, ".gitkeep")
		if err := os.WriteFile(gitkeepPath, []byte{}, 0644); err != nil {
			t.Fatalf("Failed to create .gitkeep file: %v", err)
		}

		// Commit the .gitkeep file
		err := initialRepo.Commit("Add .gitkeep file", []string{"locks/.gitkeep"})
		if err != nil {
			t.Fatalf("Failed to commit .gitkeep file: %v", err)
		}

		// Push the changes so all clones will have the locks directory
		branch, err := initialRepo.CurrentBranch()
		if err != nil {
			t.Fatalf("Failed to get current branch: %v", err)
		}
		err = initialRepo.Push("origin", branch)
		if err != nil {
			t.Fatalf("Failed to push locks directory setup: %v", err)
		}

		// Define the relative lock path that will be used in each repo
		lockPath := "locks/test-resource.lock"

		total := 10
		workingCount := 0

		var wg sync.WaitGroup
		wg.Add(total)

		// Run the goroutines
		for i := 0; i < total; i++ {
			repo, cleanup := checkoutRemoteTestRepo(t, remoteDir)
			defer cleanup()

			locking := NewRepoLocking(repo)

			go func(locking *Locking, i int) {
				defer wg.Done()

				var hasLock bool
				var errors []string
				maxTries := total * 2

				// Initialize backoff with reasonable defaults for a lock acquisition scenario
				// Min interval of 20ms, max duration of 1s
				b := backoff.New(1*time.Second, 10*time.Millisecond)

				// Try until maxTries is reached
				for tries := 0; tries < maxTries; tries++ {
					err := locking.AcquireLock(lockPath, 10*time.Minute, fmt.Sprintf("Test lock from repo %d", i))
					if err != nil {
						// Record the error
						errors = append(errors, err.Error())

						// Wait for the backoff duration before retrying
						<-time.After(b.Duration())
						continue
					}

					// Lock acquired successfully
					hasLock = true
					break
				}

				// Reset backoff for future uses
				b.Reset()

				if !hasLock {
					t.Logf("Logs:\n%v", strings.Join(errors, "\n"))
					t.Errorf("Repo %d - Failed to acquire lock after %d tries", i, maxTries)
					return
				}

				workingCount++

				if workingCount > 1 {
					t.Errorf("Repo %d - More than one process is working at the same time", i)
				}

				// Small delay to simulate work
				time.Sleep(time.Millisecond)
				workingCount--

				// Release the lock
				err := locking.ReleaseLock(lockPath)
				if err != nil {
					t.Errorf("Repo %d - Failed to release lock: %v", i, err)
					return
				}
			}(locking, i)
		}
		wg.Wait()

		if workingCount != 0 {
			t.Errorf("Expected workingCount to be 0, got %d", workingCount)
		}
	})
}
