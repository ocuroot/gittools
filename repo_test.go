package gittools

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestOpen(t *testing.T) {
	tempDir := setupTestRepo(t)

	repo, err := Open(tempDir)
	if err != nil {
		t.Fatalf("Failed to create GitRepo: %v", err)
	}

	if repo.RepoPath != tempDir {
		t.Errorf("Expected RepoPath to be %s, got %s", tempDir, repo.RepoPath)
	}
}

func TestBasicGitOperations(t *testing.T) {
	tempDir := setupTestRepo(t)

	repo, err := Open(tempDir)
	if err != nil {
		t.Fatalf("Failed to create GitRepo: %v", err)
	}

	// Test creating a branch
	if err := repo.CreateBranch("test-branch"); err != nil {
		t.Fatalf("Failed to create branch: %v", err)
	}

	// Test checkout to the new branch
	if err := repo.Checkout("test-branch"); err != nil {
		t.Fatalf("Failed to checkout test-branch: %v", err)
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

	// Add a remote and retrieve the URL
	if err := repo.AddRemote("origin", "https://github.com/ocuroot/gittools.git"); err != nil {
		t.Fatalf("Failed to add remote: %v", err)
	}

	remoteURL, err := repo.RemoteURL("origin", false)
	if err != nil {
		t.Fatalf("Failed to get remote URL: %v", err)
	}
	t.Logf("Remote URL: %s", remoteURL)

	remotePushURL, err := repo.RemoteURL("origin", true)
	if err != nil {
		t.Fatalf("Failed to get remote push URL: %v", err)
	}
	t.Logf("Remote push URL: %s", remotePushURL)
}

func TestGetHash(t *testing.T) {
	// Create a temporary file
	tempFile, err := os.CreateTemp("", "gittools-test-")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())

	expectedHash := "a5c19667710254f835085b99726e523457150e03"
	expectedContent := []byte("Hello, world\n")
	if _, err := tempFile.Write(expectedContent); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}

	// Create a repository and add the file
	client := Client{}
	hash, err := client.GetHash(tempFile.Name())
	if err != nil {
		t.Fatalf("Failed to get hash: %v", err)
	}

	if hash != expectedHash {
		t.Errorf("Expected hash %q, got %q", expectedHash, hash)
	}
}

func TestFileAtCommit(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "gittools-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	// Create a repository and add the file
	client := Client{}
	repo, err := client.Init(tempDir, "main")
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	tempFile := filepath.Join(tempDir, "test.txt")
	expectedContent := []byte("Hello, world\n")
	if err := os.WriteFile(tempFile, expectedContent, 0644); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}

	if err := repo.CommitAll("Add test file"); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Get the file at the commit
	file, err := repo.FileAtCommit("HEAD", "test.txt")
	if err != nil {
		t.Fatalf("Failed to get file: %v", err)
	}

	if !bytes.Equal([]byte(file), expectedContent) {
		t.Errorf("Expected file contents to be '%s', got '%s'", expectedContent, file)
	}

	// Make a change to the file and look at the original commit
	updatedContent := []byte("Updated content\n")
	if err := os.WriteFile(tempFile, updatedContent, 0644); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}

	if err := repo.CommitAll("Change test file"); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	file, err = repo.FileAtCommit("HEAD", "test.txt")
	if err != nil {
		t.Fatalf("Failed to get file: %v", err)
	}

	if !bytes.Equal([]byte(file), updatedContent) {
		t.Errorf("Expected file contents to be '%s', got '%s'", updatedContent, file)
	}

	file, err = repo.FileAtCommit("HEAD~1", "test.txt")
	if err != nil {
		t.Fatalf("Failed to get file: %v", err)
	}

	if !bytes.Equal([]byte(file), expectedContent) {
		t.Errorf("Expected file contents to be '%s', got '%s'", expectedContent, file)
	}
}

func setupTestRepo(t *testing.T) string {
	t.Helper()
	var err error

	client := Client{}

	// Create a temporary directory for the repository
	tempDir, err := os.MkdirTemp("", "gitlock-test-")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	// Initialize git repository
	repo, err := client.Init(tempDir, "main")
	if err != nil {
		t.Fatalf("Failed to initialize git repository: %v", err)
	}

	repo.ConfigSet("user.name", "Test User")
	repo.ConfigSet("user.email", "test@example.com")

	// Create an initial commit
	readme := filepath.Join(tempDir, "README.md")
	if err := os.WriteFile(readme, []byte("# Test Repository\n"), 0644); err != nil {
		t.Fatalf("Failed to write README.md: %v", err)
	}

	err = repo.CommitAll("Initial commit")
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Return the temp directory
	return tempDir
}
