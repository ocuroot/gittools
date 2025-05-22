// Package testutils provides test utilities for gittools
package testutils

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// CreateTestRemoteRepo creates an initialized bare Git repository for testing.
// It returns the path to the bare repository and a cleanup function.
func CreateTestRemoteRepo(baseName string) (repoPath string, cleanup func(), err error) {
	// Create a directory for the bare repo
	repoPath, err = os.MkdirTemp("", baseName+"-bare-")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create bare repo directory: %w", err)
	}

	// Setup cleanup function
	cleanup = func() {
		os.RemoveAll(repoPath)
	}

	// Initialize bare repository
	cmd := exec.Command("git", "init", "--bare", "--initial-branch=main", repoPath)
	if err := cmd.Run(); err != nil {
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

	// Initialize a temp repo to make the initial commit
	cmd = exec.Command("git", "init", "--initial-branch=main", tempDir)
	if err := cmd.Run(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to initialize temp repository: %w", err)
	}

	// Set git config for the temp repository
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to set git config user.name: %w", err)
	}

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to set git config user.email: %w", err)
	}

	// Add the bare repo as a remote
	cmd = exec.Command("git", "remote", "add", "origin", repoPath)
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to add remote: %w", err)
	}

	// Create initial commit with README
	readme := filepath.Join(tempDir, "README.md")
	if err := os.WriteFile(readme, []byte("# Test Repository\n"), 0644); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to write README.md: %w", err)
	}

	// Add and commit README
	cmd = exec.Command("git", "add", "README.md")
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to add README.md: %w", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to commit: %w", err)
	}

	// Push to the bare repo
	cmd = exec.Command("git", "push", "origin", "main")
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to push: %w", err)
	}

	return repoPath, cleanup, nil
}
