package gittools_test

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ocuroot/gittools"
	"github.com/ocuroot/gittools/lock"
	"github.com/ocuroot/gittools/testutils"
)

// This example demonstrates how to clone a repository and perform basic operations with it.
func Example_repositoryManagement() {
	// Create a bare repository that we can clone from
	remoteRepo, remoteCleanup, err := testutils.CreateTestRemoteRepo("gittools-example")
	if err != nil {
		fmt.Printf("Failed to create bare repository: %v\n", err)
		return
	}
	defer remoteCleanup()

	// Create a temporary directory for cloning
	cloneDir, err := os.MkdirTemp("", "gittools-clone-")
	if err != nil {
		fmt.Printf("Failed to create temp directory: %v\n", err)
		return
	}
	defer os.RemoveAll(cloneDir)

	// Clone the repository using our library
	repo, err := gittools.Clone(remoteRepo, cloneDir)
	if err != nil {
		fmt.Printf("Failed to clone repository: %v\n", err)
		return
	}

	// Create a new file
	filePath := filepath.Join(cloneDir, "example.txt")
	err = os.WriteFile(filePath, []byte("Hello, Git!\n"), 0644)
	if err != nil {
		fmt.Printf("Failed to write file: %v\n", err)
		return
	}

	// Commit the file (Commit method handles both adding and committing)
	err = repo.Commit("Add example file", []string{"example.txt"})
	if err != nil {
		fmt.Printf("Failed to commit: %v\n", err)
		return
	}

	// Create and switch to a new branch
	err = repo.CreateBranch("feature-branch")
	if err != nil {
		fmt.Printf("Failed to create branch: %v\n", err)
		return
	}

	err = repo.Checkout("feature-branch")
	if err != nil {
		fmt.Printf("Failed to checkout branch: %v\n", err)
		return
	}

	// Make another change and commit
	err = os.WriteFile(filePath, []byte("Hello, Git!\nAnother line.\n"), 0644)
	if err != nil {
		fmt.Printf("Failed to update file: %v\n", err)
		return
	}

	// Commit the updated file
	err = repo.Commit("Update example file", []string{"example.txt"})
	if err != nil {
		fmt.Printf("Failed to commit update: %v\n", err)
		return
	}

	// Push the feature branch to remote
	err = repo.Push("origin", "feature-branch")
	if err != nil {
		fmt.Printf("Failed to push branch: %v\n", err)
		return
	}

	// Get current branch
	branch, err := repo.CurrentBranch()
	if err != nil {
		fmt.Printf("Failed to get current branch: %v\n", err)
		return
	}
	fmt.Printf("Current branch: %s\n", branch)

	// Output:
	// Current branch: feature-branch
}

// This example demonstrates how to use the distributed locking system.
func Example_distributedLocking() {
	// Create a bare repository in a temp dir that we can clone from
	remoteRepo, remoteCleanup, err := testutils.CreateTestRemoteRepo("gittools-lock-example")
	if err != nil {
		fmt.Printf("Failed to create bare repository: %v\n", err)
		return
	}
	defer remoteCleanup()

	// Create a temporary directory for cloning
	cloneDir, err := os.MkdirTemp("", "gittools-lock-clone-")
	if err != nil {
		fmt.Printf("Failed to create temp directory: %v\n", err)
		return
	}
	defer os.RemoveAll(cloneDir)

	// Clone the repository using our library
	repo, err := gittools.Clone(remoteRepo, cloneDir)
	if err != nil {
		fmt.Printf("Failed to clone repository: %v\n", err)
		return
	}

	// Create a locks manager
	locking := lock.NewRepoLocking(repo)

	// Try to acquire a lock with 10-minute expiration
	lockPath := "resources/database-migration.lock"
	err = locking.AcquireLock(lockPath, 10*time.Minute, "Running schema migration")
	if err != nil {
		fmt.Printf("Failed to acquire lock: %v\n", err)
		return
	}
	fmt.Println("Lock acquired, performing work...")

	// When done, release the lock
	err = locking.ReleaseLock(lockPath)
	if err != nil {
		fmt.Printf("Failed to release lock: %v\n", err)
		return
	}
	fmt.Println("Lock released")

	// Output:
	// Lock acquired, performing work...
	// Lock released
}


