package gittools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// CreateTestRemoteRepo creates an initialized bare Git repository for testing.
// The default branch defaults to "main".
// It returns the path to the bare repository and a cleanup function.
func CreateTestRemoteRepo(baseName string) (repoPath string, cleanup func(), err error) {
	return CreateTestRemoteRepoWithBranch(baseName, "main")
}

// CreateTestRemoteRepoWithBranch creates an initialized bare Git repository for testing.
// It returns the path to the bare repository and a cleanup function.
func CreateTestRemoteRepoWithBranch(baseName string, defaultBranch string) (repoPath string, cleanup func(), err error) {
	// Create a directory for the bare repo
	repoPath, err = os.MkdirTemp("", baseName+"-bare-")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create bare repo directory: %w", err)
	}

	// Setup cleanup function
	cleanup = func() {
		os.RemoveAll(repoPath)
	}

	client := &Client{}

	// Initialize bare repository
	_, err = client.InitBare(repoPath, defaultBranch)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to initialize bare repository: %w", err)
	}

	// Create a temporary directory for an initial commit
	tempDir, err := os.MkdirTemp("", baseName+"-init-")
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Clone the bare repository
	_, err = client.Clone(repoPath, tempDir)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to clone repository: %w", err)
	}

	// Create a README file
	readmePath := filepath.Join(tempDir, "README.md")
	readmeContent := []byte("# Test Repository\n\nThis is a test repository created for gittools tests.")
	if err := os.WriteFile(readmePath, readmeContent, 0644); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to create README file: %w", err)
	}

	// Initialize the repository
	tempRepo, err := Open(tempDir)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to open cloned repository: %w", err)
	}

	// Add README and commit
	err = tempRepo.Commit("Initial commit", []string{readmePath})
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to add and commit README file: %w", err)
	}

	// Push to bare repository
	if err := tempRepo.Push("origin", "main"); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to push initial commit: %w", err)
	}

	return repoPath, cleanup, nil
}

// PushWithTimeout executes a git push with a timeout to prevent hanging tests
func PushWithTimeout(t *testing.T, repo *Repo, remote, branch string, timeoutSeconds int) error {
	t.Helper()
	t.Logf("Pushing commits to %s/%s with %d second timeout", remote, branch, timeoutSeconds)

	// Set up a timeout context for the push operation
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	// Channel for push completion
	done := make(chan error, 1)
	go func() {
		done <- repo.Push(remote, branch)
	}()

	// Wait for completion or timeout
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return fmt.Errorf("push operation timed out after %d seconds", timeoutSeconds)
	}
}

// GitExec runs a git command with timeout protection
func GitExec(t *testing.T, repoPath string, timeoutSeconds int, args ...string) ([]byte, error) {
	t.Helper()

	// Set up a timeout context
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	// Create and execute the command
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath

	// Run the command and return output
	return cmd.CombinedOutput()
}

// RunWithTimeout runs a function with a timeout and returns its result
func RunWithTimeout(t *testing.T, operation string, timeoutSeconds int, fn func() (interface{}, error)) (interface{}, error) {
	t.Helper()

	// Create timeout context
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	// Use a channel to collect the result
	ch := make(chan struct {
		res interface{}
		err error
	}, 1)

	// Run the function in a goroutine
	go func() {
		res, err := fn()
		ch <- struct {
			res interface{}
			err error
		}{res, err}
	}()

	// Wait for either the operation to complete or the context to timeout
	select {
	case result := <-ch:
		return result.res, result.err
	case <-ctx.Done():
		return nil, fmt.Errorf("%s operation timed out after %d seconds", operation, timeoutSeconds)
	}
}

// IsSafeDirectory checks if a directory path is safe to use for testing
// It ensures the directory is not part of the source code repository
func IsSafeDirectory(dir, sourceRepoDir string) bool {
	// Never allow operations on the source repo directory
	if strings.Contains(dir, sourceRepoDir) {
		return false
	}

	// Only allow operations on temp directories
	isTemp := strings.Contains(dir, os.TempDir()) ||
		strings.HasPrefix(dir, "/tmp") ||
		strings.HasPrefix(dir, os.TempDir())

	return isTemp
}

// SafeTest wraps a test function to ensure it runs in a temporary directory
// This prevents tests from accidentally operating on the source code repository
func SafeTest(t *testing.T, testFn func(t *testing.T, tempDir string)) {
	t.Helper()

	// Get source code repository path
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("Failed to get current file path")
	}

	// Determine the source repo directory (two directories up from this file)
	sourceRepoDir := filepath.Dir(filepath.Dir(filename))

	// Get the current working directory
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}

	// Create a temporary directory for this test
	tempDir, err := os.MkdirTemp("", "gittools-test-")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	// Safety check: ensure temp directory is not in source repo
	if !IsSafeDirectory(tempDir, sourceRepoDir) {
		t.Fatalf("CRITICAL SAFETY ERROR: Test directory is not safe: %s", tempDir)
	}

	// Change to the temp directory
	if err := os.Chdir(tempDir); err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	// Set up deferred cleanup
	defer func() {
		// Change back to the original working directory
		if err := os.Chdir(originalWd); err != nil {
			t.Fatalf("Failed to restore original working directory: %v", err)
		}

		// Remove the temporary directory
		os.RemoveAll(tempDir)
	}()

	// Run the test function with the temp directory path
	testFn(t, tempDir)
}
