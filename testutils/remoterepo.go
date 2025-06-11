// Package testutils provides test utilities for gittools
package testutils

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ocuroot/gittools"
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

	client := &gittools.Client{}

	// Initialize bare repository
	_, err = client.InitBare(repoPath, "main")
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

	// Initialize a temp repo to make the initial commit
	repo, err := client.Init(tempDir, "main")
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to initialize temp repository: %w", err)
	}

	// Set git config for the temp repository
	repo.ConfigSet("user.name", "Test User")
	repo.ConfigSet("user.email", "test@example.com")

	// Add the bare repo as a remote

	err = repo.AddRemote("origin", repoPath)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to add remote: %w", err)
	}

	// Create initial commit with README
	readme := filepath.Join(tempDir, "README.md")
	if err := os.WriteFile(readme, []byte("# Test Repository\n"), 0644); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to write README.md: %w", err)
	}

	err = repo.CommitAll("Initial commit")
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to commit: %w", err)
	}

	err = repo.Push("origin", "main")
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to push: %w", err)
	}

	return repoPath, cleanup, nil
}
