package gitlock

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func setupTestRepo(t *testing.T) (string, func()) {
	t.Helper()

	// Create a temporary directory for the repository
	tempDir, err := os.MkdirTemp("", "gitlock-test-")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	// Initialize git repository
	cmd := exec.Command("git", "init", tempDir)
	if err := cmd.Run(); err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to initialize git repository: %v", err)
	}

	// Set git config for the test repository
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to set git config user.name: %v", err)
	}

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to set git config user.email: %v", err)
	}

	// Create an initial commit
	readme := filepath.Join(tempDir, "README.md")
	if err := os.WriteFile(readme, []byte("# Test Repository\n"), 0644); err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to write README.md: %v", err)
	}

	cmd = exec.Command("git", "add", "README.md")
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to git commit: %v", err)
	}

	// Create "main" branch (newer git defaults to "main", older to "master")
	cmd = exec.Command("git", "branch", "-M", "main")
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to rename branch to main: %v", err)
	}

	// Return the temp directory and a cleanup function
	return tempDir, func() {
		os.RemoveAll(tempDir)
	}
}

func TestNew(t *testing.T) {
	tempDir, cleanup := setupTestRepo(t)
	defer cleanup()

	repo, err := New(tempDir)
	if err != nil {
		t.Fatalf("Failed to create GitRepo: %v", err)
	}

	if repo.RepoPath != tempDir {
		t.Errorf("Expected RepoPath to be %s, got %s", tempDir, repo.RepoPath)
	}

	if repo.LockKey == "" {
		t.Errorf("Expected LockKey to be set, got empty string")
	}
}

func TestBasicGitOperations(t *testing.T) {
	tempDir, cleanup := setupTestRepo(t)
	defer cleanup()

	repo, err := New(tempDir)
	if err != nil {
		t.Fatalf("Failed to create GitRepo: %v", err)
	}

	// Test creating a branch
	if err := repo.CreateBranch("test-branch"); err != nil {
		t.Fatalf("Failed to create branch: %v", err)
	}

	// Test getting current branch
	branch, err := repo.CurrentBranch()
	if err != nil {
		t.Fatalf("Failed to get current branch: %v", err)
	}
	
	if branch != "test-branch" {
		t.Errorf("Expected current branch to be 'test-branch', got '%s'", branch)
	}

	// Test checkout
	if err := repo.Checkout("main"); err != nil {
		t.Fatalf("Failed to checkout main: %v", err)
	}

	branch, err = repo.CurrentBranch()
	if err != nil {
		t.Fatalf("Failed to get current branch: %v", err)
	}
	
	if branch != "main" {
		t.Errorf("Expected current branch to be 'main', got '%s'", branch)
	}

	// Test commit
	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("Test content\n"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	if err := repo.Commit("Test commit", []string{"test.txt"}); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}
}

// Other tests can be implemented as needed
