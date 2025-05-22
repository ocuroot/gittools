package gittools_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/ocuroot/gittools"
	"github.com/ocuroot/gittools/lock"
)

// This example demonstrates how to initialize a new Git repository
// and perform basic operations with it.
func Example_repositoryManagement() {
	// Create a temporary directory for example
	tempDir, err := os.MkdirTemp("", "gittools-example-")
	if err != nil {
		fmt.Printf("Failed to create temp directory: %v\n", err)
		return
	}
	defer os.RemoveAll(tempDir) // Clean up when done

	// Initialize git repository manually first (since there's no Init function)
	cmd := exec.Command("git", "init", "--initial-branch=main", tempDir)
	if err := cmd.Run(); err != nil {
		fmt.Printf("Failed to initialize git repository: %v\n", err)
		return
	}

	// Set git config for the test repository
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		fmt.Printf("Failed to set git config user.name: %v\n", err)
		return
	}

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		fmt.Printf("Failed to set git config user.email: %v\n", err)
		return
	}

	// Create a Repo instance from the initialized repository
	repo, err := gittools.New(tempDir)
	if err != nil {
		fmt.Printf("Failed to initialize repository: %v\n", err)
		return
	}

	// Create a new file
	filePath := filepath.Join(tempDir, "example.txt")
	err = os.WriteFile(filePath, []byte("Hello, Git!"), 0644)
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
	// Create two temporary directories - one for the remote and one for the local repo
	remoteDir, err := os.MkdirTemp("", "gittools-remote-")
	if err != nil {
		fmt.Printf("Failed to create remote directory: %v\n", err)
		return
	}
	defer os.RemoveAll(remoteDir)

	tempDir, err := os.MkdirTemp("", "gittools-lock-example-")
	if err != nil {
		fmt.Printf("Failed to create temp directory: %v\n", err)
		return
	}
	defer os.RemoveAll(tempDir) // Clean up when done

	// Initialize remote repository
	cmd := exec.Command("git", "init", "--bare", "--initial-branch=main", remoteDir)
	if err := cmd.Run(); err != nil {
		fmt.Printf("Failed to initialize remote repository: %v\n", err)
		return
	}

	// Initialize local repository
	cmd = exec.Command("git", "init", "--initial-branch=main", tempDir)
	if err := cmd.Run(); err != nil {
		fmt.Printf("Failed to initialize git repository: %v\n", err)
		return
	}

	// Set git config for the test repository
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		fmt.Printf("Failed to set git config user.name: %v\n", err)
		return
	}

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		fmt.Printf("Failed to set git config user.email: %v\n", err)
		return
	}

	// Add the remote to the local repository
	cmd = exec.Command("git", "remote", "add", "origin", remoteDir)
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		fmt.Printf("Failed to add remote: %v\n", err)
		return
	}

	// Create a Repo instance from the initialized repository
	repo, err := gittools.New(tempDir)
	if err != nil {
		fmt.Printf("Failed to initialize repository: %v\n", err)
		return
	}

	// Create an initial commit (required for HEAD to be valid)
	readme := filepath.Join(tempDir, "README.md")
	if err := os.WriteFile(readme, []byte("# Test Repository\n"), 0644); err != nil {
		fmt.Printf("Failed to write README.md: %v\n", err)
		return
	}

	// Commit the README file to create an initial commit
	err = repo.Commit("Initial commit", []string{"README.md"})
	if err != nil {
		fmt.Printf("Failed to make initial commit: %v\n", err)
		return
	}

	// Push to the remote so pull operations will work
	err = repo.Push("origin", "main")
	if err != nil {
		fmt.Printf("Failed to push to remote: %v\n", err)
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
