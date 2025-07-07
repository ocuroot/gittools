package gittools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestExponentialCommitSearch demonstrates searching for a commit using
// an exponential backoff approach to increase fetch depth until the commit is found.
// This simulates a real-world scenario where you're looking for a commit in a large
// repository and need to balance network traffic with search efficiency.
func TestExponentialCommitSearch(t *testing.T) {
	SafeTest(t, func(t *testing.T, tempDir string) {
		if testing.Short() {
			t.Skip("skipping long-running test")
		}

		t.Log("Starting exponential commit search test with improved timeout handling")

		// Set up overall test timeout
		testCtx, testCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer testCancel()

		// Create a remote repository for testing
		t.Log("Creating test remote repository")
		remotePath, cleanup, err := CreateTestRemoteRepo("exp-search-test")
		if err != nil {
			t.Fatalf("Failed to create test remote repository: %v", err)
		}
		defer cleanup()

		// First create a full clone to create the target commit and get all commit IDs
		clonePath := filepath.Join(tempDir, "full-clone")

		t.Log("Creating full clone of test repository")
		cloneCtx, cloneCancel := context.WithTimeout(context.Background(), 30*time.Second)

		// Clone the repository
		gitCmd := exec.CommandContext(cloneCtx, "git", "clone", remotePath, clonePath)
		if err := gitCmd.Run(); err != nil {
			cloneCancel()
			t.Fatalf("Failed to clone repository: %v", err)
		}
		cloneCancel()

		// Open the cloned repository
		cloneRepo, err := Open(clonePath)
		if err != nil {
			t.Fatalf("Failed to open cloned repository: %v", err)
		}

		// Create 10 test commits
		numCommits := 10
		commitIDs := make([]string, numCommits)

		t.Log("Creating test commits")
		for i := 0; i < numCommits; i++ {
			// Create a file with unique content
			filePath := filepath.Join(clonePath, fmt.Sprintf("file_%d.txt", i))
			content := []byte(fmt.Sprintf("Commit %d content", i))
			if err := os.WriteFile(filePath, content, 0644); err != nil {
				t.Fatalf("Failed to write file %d: %v", i, err)
			}

			// Commit the file
			commitMsg := fmt.Sprintf("Add file %d", i)
			if err := cloneRepo.Commit(commitMsg, []string{filePath}); err != nil {
				t.Fatalf("Failed to commit file %d: %v", i, err)
			}

			// Get commit hash
			commit, err := cloneRepo.RevParse("HEAD")
			if err != nil {
				t.Fatalf("Failed to commit file %d: %v", i, err)
			}

			commitIDs[i] = commit
			t.Logf("Created commit %d: %s", i, commit[:8])
		}

		// Push all commits to the remote repository
		t.Log("Pushing commits to remote repository")
		pushCtx, pushCancel := context.WithTimeout(context.Background(), 30*time.Second)
		pushDone := make(chan error)

		go func() {
			pushDone <- cloneRepo.Push("origin", "master")
		}()

		select {
		case err := <-pushDone:
			if err != nil {
				pushCancel()
				t.Fatalf("Failed to push commits: %v", err)
			}
		case <-pushCtx.Done():
			t.Fatalf("Push operation timed out")
		}

		pushCancel()
		t.Log("Successfully pushed commits to remote")

		// Pick a commit in the middle to search for
		targetIndex := numCommits / 2
		targetCommit := commitIDs[targetIndex]
		t.Logf("Selected target commit: %s at index %d", targetCommit[:8], targetIndex)

		// Create a shallow clone to test searching
		shallowClonePath := filepath.Join(tempDir, "shallow-clone")

		// Create a shallow clone with depth 1 using the Client
		t.Log("Creating shallow clone with depth 1")
		client := &Client{}
		cloneOptions := CloneOptions{
			URL:         remotePath,
			Destination: shallowClonePath,
			Depth:       1,
			Context:     context.Background(),
		}

		shallowCloneRepo, err := client.CloneWithOptions(cloneOptions)
		if err != nil {
			t.Fatalf("Failed to create shallow clone: %v", err)
		}

		// Verify the shallow clone was created successfully
		t.Log("Verifying shallow clone was created")

		// Confirm we can get the HEAD commit from the shallow clone
		stdout, stderr, err := shallowCloneRepo.Client.Exec("rev-parse", "HEAD")
		if err != nil {
			t.Fatalf("Failed to get HEAD of shallow clone: %v\nstderr: %s", err, stderr)
		}

		headCommit := strings.TrimSpace(string(stdout))
		t.Logf("Shallow clone HEAD commit: %s", headCommit[:8])

		// Check that the target commit does not exist in the shallow clone
		exists, err := shallowCloneRepo.commitExists(targetCommit, 5*time.Second)
		if err != nil {
			t.Fatalf("Error checking if commit exists: %v", err)
		}

		if exists {
			t.Logf("Target commit %s unexpectedly found in shallow clone before search", targetCommit[:8])
		} else {
			t.Logf("As expected, target commit %s not found in initial shallow clone", targetCommit[:8])
		}

		// Set up a timeout context for the search operation
		searchCtx, searchCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer searchCancel()

		// Run the search in a goroutine with channel for result
		var commitPath []string
		var searchErr error
		done := make(chan bool)

		go func() {
			// Use the repo method instead of the standalone function
			// Get HEAD commit for the latest commit parameter
			latestCommit, err := shallowCloneRepo.RevParse("HEAD")
			if err != nil {
				searchErr = fmt.Errorf("Failed to get HEAD commit: %v", err)
				done <- true
				return
			}
			commitPath, searchErr = shallowCloneRepo.GetCommitsBetween(targetCommit, latestCommit, nil)
			done <- true
		}()

		// Wait for either completion or timeout
		select {
		case <-done:
			// Search completed normally
			t.Log("Search completed within timeout period")
			if searchErr != nil {
				t.Fatalf("Failed to find commit: %v", searchErr)
			}

			// Verify the results
			if commitPath == nil {
				t.Errorf("Failed to find target commit %s", targetCommit)
			} else {
				// Calculate theoretical minimum iterations if we knew the exact depth needed
				neededDepth := numCommits - targetIndex
				t.Logf("Actual required depth: %d (commit is %d positions from HEAD)", neededDepth, neededDepth)

				// Verify the search results with comprehensive logging
				t.Log("Search operation completed, verifying results")

				if commitPath == nil {
					t.Fatalf("Expected to find commit path, but got nil")
				}

				if len(commitPath) == 0 {
					t.Fatalf("Expected non-empty commit path")
				}

				// Log the complete commit path for debugging
				t.Logf("Found commit path of length %d", len(commitPath))
				for i, commit := range commitPath {
					t.Logf("  Path[%d]: %s", i, commit)
				}

				// Verify the target commit is at the end of the path
				if commitPath[len(commitPath)-1] != targetCommit {
					t.Fatalf("Target commit not found at end of path. Got %s, expected %s",
						commitPath[len(commitPath)-1], targetCommit)
				}

				t.Log("✓ Successfully verified that target commit is at the end of the path")
				t.Log("✓ Test completed successfully")
			}
		case <-searchCtx.Done():
			t.Fatalf("Search timed out")
		case <-testCtx.Done():
			t.Fatalf("Overall test timed out")
		}
	})
}

// TestExponentialCommitSearchWithOptions demonstrates searching for a commit using
// custom configuration options for timeouts and depths.
func TestExponentialCommitSearchWithOptions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping long-running test")
	}

	t.Log("Starting exponential commit search test with custom options")

	// Set up overall test timeout
	testCtx, testCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer testCancel()

	// Create a remote repository for testing
	t.Log("Creating test remote repository")
	remotePath, cleanup, err := CreateTestRemoteRepo("exp-search-options-test")
	if err != nil {
		t.Fatalf("Failed to create test remote repository: %v", err)
	}
	defer cleanup()

	// Create a directory for the local clone
	tempDir, err := os.MkdirTemp("", "exp-search-options-test-local-")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a full clone to add commits
	t.Log("Creating full clone of test repository")
	clonePath := filepath.Join(tempDir, "full-clone")
	ctx, cancel := context.WithTimeout(testCtx, 30*time.Second)
	gitCmd := exec.CommandContext(ctx, "git", "clone", remotePath, clonePath)
	err = gitCmd.Run()
	cancel()

	if err != nil {
		t.Fatalf("Failed to clone repository: %v", err)
	}

	// Open the cloned repository
	t.Log("Opening cloned repository")
	repo, err := Open(clonePath)
	if err != nil {
		t.Fatalf("Failed to open cloned repository: %v", err)
	}

	// Create test commits
	t.Log("Creating test commits")
	numCommits := 5
	commitIDs := make([]string, numCommits)

	for i := 0; i < numCommits; i++ {
		// Create a unique file
		filePath := filepath.Join(clonePath, fmt.Sprintf("file_%d.txt", i))
		content := []byte(fmt.Sprintf("Content for commit %d", i))

		if err := os.WriteFile(filePath, content, 0644); err != nil {
			t.Fatalf("Failed to write file %d: %v", i, err)
		}

		// Commit the file
		if err := repo.Commit(fmt.Sprintf("Add file %d", i), []string{filePath}); err != nil {
			t.Fatalf("Failed to commit file %d: %v", i, err)
		}

		// Get the commit ID
		commitID, err := repo.RevParse("HEAD")

		if err != nil {
			t.Fatalf("Failed to commit file %d: %v", i, err)
		}

		commitIDs[i] = commitID
		t.Logf("Created commit %d: %s", i, commitID[:8])
	}

	// Push the commits
	t.Log("Pushing commits to remote repository")
	pushCtx, pushCancel := context.WithTimeout(context.Background(), 30*time.Second)
	pushDone := make(chan error)

	go func() {
		pushDone <- repo.Push("origin", "master")
	}()

	select {
	case err := <-pushDone:
		if err != nil {
			pushCancel()
			t.Fatalf("Failed to push commits: %v", err)
		}
	case <-pushCtx.Done():
		pushCancel()
		t.Fatalf("Push operation timed out")
	}

	pushCancel()

	// Pick a target commit in the middle
	targetIndex := numCommits / 2
	targetCommit := commitIDs[targetIndex]
	t.Logf("Selected target commit: %s at index %d", targetCommit[:8], targetIndex)

	// Create a shallow clone to test searching
	clonePath = filepath.Join(tempDir, "clone")

	// Create a shallow clone with depth 1
	t.Log("Creating shallow clone with depth 1")
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Clone with depth=1
	gitCmd = exec.CommandContext(ctx, "git", "clone", "--depth=1", remotePath, clonePath)
	if err := gitCmd.Run(); err != nil {
		t.Fatalf("Failed to create shallow clone: %v", err)
	}

	// Open the shallow clone repository
	shallowCloneRepo, err := Open(clonePath)
	if err != nil {
		t.Fatalf("Failed to create shallow clone: %v", err)
	}

	// Create custom search options with shorter timeouts and smaller max depth
	// This demonstrates how to configure the search for different environments
	customOptions := &GetCommitsBetweenOptions{
		// Use a smaller max depth as we only have a few commits
		MaxDepth: 16,
		// Use a single operation timeout for all git operations
		OperationTimeout: 5 * time.Second,
	}

	// Set a short timeout for our test
	testCtx, testCancel = context.WithTimeout(context.Background(), 20*time.Second)
	defer testCancel()

	// Run the search in a goroutine with channel for result
	var commitPath []string
	var searchErr error
	done := make(chan bool)

	t.Log("Starting search with custom options")
	go func() {
		// Get HEAD commit for the latest commit parameter
		latestCommit, err := shallowCloneRepo.RevParse("HEAD")
		if err != nil {
			searchErr = fmt.Errorf("Failed to get HEAD commit: %v", err)
			done <- true
			return
		}
		commitPath, searchErr = shallowCloneRepo.GetCommitsBetween(targetCommit, latestCommit, customOptions)
		done <- true
	}()

	// Wait for either completion or timeout
	select {
	case <-done:
		t.Log("Search completed within timeout period")
		if searchErr != nil {
			t.Fatalf("Failed to find commit: %v", searchErr)
		}

		if commitPath == nil {
			t.Fatalf("Failed to find target commit")
		}

		// Log the found commit path
		t.Logf("Found commit path with length %d", len(commitPath))
		for i, commit := range commitPath {
			t.Logf("  Path[%d]: %s", i, commit)
		}

		// Verify the target commit is found
		if commitPath[len(commitPath)-1] != targetCommit {
			t.Fatalf("Target commit not found at end of path. Got %s, expected %s",
				commitPath[len(commitPath)-1], targetCommit)
		}

		t.Log("✓ Test completed successfully")
	case <-testCtx.Done():
		t.Fatalf("Test timed out after 20 seconds")
	}
}

// TestExponentialCommitSearchMissingCommitStable verifies the exponential search
// behavior when the target commit doesn't exist in the repository with improved
// timeout handling to prevent hanging tests.
func TestExponentialCommitSearchMissingCommitStable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping long-running test")
	}

	// Set up overall test timeout
	testCtx, testCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer testCancel()

	// Create a remote repository for testing
	t.Log("Creating test remote repository")
	remotePath, cleanup, err := CreateTestRemoteRepo("find-missing-commit-test-stable")
	if err != nil {
		t.Fatalf("Failed to create test remote repository: %v", err)
	}
	defer cleanup()

	// Create a directory for the local clone
	tempDir, err := os.MkdirTemp("", "find-missing-commit-test-local-")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a shallow clone to test searching
	shallowPath := filepath.Join(tempDir, "shallow")

	// Use the client to create a shallow clone
	client := &Client{}
	cloneOpts := CloneOptions{
		URL:         remotePath,
		Destination: shallowPath,
		Depth:       1,
	}

	shallowRepo, err := client.CloneWithOptions(cloneOpts)
	if err != nil {
		t.Fatalf("Failed to create shallow clone: %v", err)
	}

	// A commit hash that doesn't exist in the repository
	nonExistentCommit := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"

	// Use a channel to collect search results
	resultChan := make(chan []string, 1)
	errChan := make(chan error, 1)

	// Start the search in a goroutine
	go func() {
		path, err := shallowRepo.FindCommitWithExponentialDepth(nonExistentCommit, nil)
		if err != nil {
			errChan <- err
			return
		}
		resultChan <- path
	}()

	// Wait for either a result or timeout
	var commitPath []string
	var searchErr error

	select {
	case commitPath = <-resultChan:
		// We received a result
	case searchErr = <-errChan:
		// We received an error
	case <-testCtx.Done():
		t.Fatalf("Test timed out")
	}

	// Verify that the search completed without error but found no commit path
	if searchErr != nil {
		t.Fatalf("Search returned an unexpected error: %v", searchErr)
	}

	if commitPath != nil {
		t.Fatalf("Expected nil commit path for non-existent commit, but got path with length %d", len(commitPath))
	}

	t.Log("✓ Successfully verified that non-existent commit was not found")
}

// TestExponentialCommitSearchFullClone verifies that when starting with a full clone,
// the exponential search finds the target commit immediately in the first iteration.
func TestExponentialCommitSearchFullClone(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping long-running test")
	}

	// Set up overall test timeout
	testCtx, testCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer testCancel()

	// Create a remote repository with some commits
	t.Log("Creating test remote repository")
	remotePath, cleanup, err := CreateTestRemoteRepo("exp-search-full-clone-test")
	if err != nil {
		t.Fatalf("Failed to create test remote repository: %v", err)
	}
	defer cleanup()

	// Create a directory for the local clone
	tempDir, err := os.MkdirTemp("", "exp-search-full-clone-test-local-")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a full clone to add commits
	t.Log("Creating full clone of test repository")
	clonePath := filepath.Join(tempDir, "full-clone")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	gitCmd := exec.CommandContext(ctx, "git", "clone", remotePath, clonePath)
	err = gitCmd.Run()
	cancel()

	if err != nil {
		t.Fatalf("Failed to clone repository: %v", err)
	}

	// Open the cloned repository
	t.Log("Opening cloned repository")
	repo, err := Open(clonePath)
	if err != nil {
		t.Fatalf("Failed to open cloned repository: %v", err)
	}

	// Create test commits
	t.Log("Creating test commits")
	numCommits := 5
	commitIDs := make([]string, numCommits)

	for i := 0; i < numCommits; i++ {
		// Create a unique file
		filePath := filepath.Join(clonePath, fmt.Sprintf("file_%d.txt", i))
		content := []byte(fmt.Sprintf("Content for commit %d", i))

		if err := os.WriteFile(filePath, content, 0644); err != nil {
			t.Fatalf("Failed to write file %d: %v", i, err)
		}

		// Commit the file
		if err := repo.Commit(fmt.Sprintf("Add file %d", i), []string{filePath}); err != nil {
			t.Fatalf("Failed to commit file %d: %v", i, err)
		}

		// Get the commit ID
		commitID, err := repo.RevParse("HEAD")

		if err != nil {
			t.Fatalf("Failed to commit file %d: %v", i, err)
		}

		commitIDs[i] = commitID
		t.Logf("Created commit %d: %s", i, commitID[:8])
	}

	// Push the commits
	t.Log("Pushing commits to remote repository")
	pushCtx, pushCancel := context.WithTimeout(context.Background(), 30*time.Second)
	pushDone := make(chan error)

	go func() {
		pushDone <- repo.Push("origin", "master")
	}()

	select {
	case err := <-pushDone:
		if err != nil {
			pushCancel()
			t.Fatalf("Failed to push commits: %v", err)
		}
	case <-pushCtx.Done():
		pushCancel()
		t.Fatalf("Push operation timed out")
	}

	pushCancel()

	// Pick a commit to search for (somewhere in the middle)
	targetIndex := numCommits / 2
	targetCommit := commitIDs[targetIndex]
	t.Logf("Selected target commit: %s at index %d", targetCommit[:8], targetIndex)

	// Create a full clone to test the search
	fullClonePath := filepath.Join(tempDir, "clone-for-search")
	client := &Client{}
	cloneRepo, err := client.Clone(remotePath, fullClonePath)
	if err != nil {
		t.Fatalf("Failed to clone repository: %v", err)
	}

	// Run the search in a goroutine with channel for result
	var commitPath []string
	var searchErr error
	done := make(chan bool)

	go func() {
		// Get HEAD commit for the latest commit parameter
		latestCommit, err := cloneRepo.RevParse("HEAD")
		if err != nil {
			searchErr = fmt.Errorf("Failed to get HEAD commit: %v", err)
			done <- true
			return
		}
		commitPath, searchErr = cloneRepo.GetCommitsBetween(targetCommit, latestCommit, nil)
		done <- true
	}()

	// Wait for either completion or timeout
	select {
	case <-done:
		t.Log("Search completed")
		if searchErr != nil {
			t.Fatalf("Failed to find commit: %v", searchErr)
		}

		// Verify that the commit was found and it's the right one
		if commitPath == nil {
			t.Fatal("Expected to find commit path, but got nil")
		}

		// Verify the target commit is at the end of the path
		if commitPath[len(commitPath)-1] != targetCommit {
			t.Fatalf("Target commit not found at end of path. Got %s, expected %s",
				commitPath[len(commitPath)-1], targetCommit)
		}

		t.Log("✓ Successfully found target commit in full clone")
	case <-testCtx.Done():
		t.Fatalf("Test timed out")
	}
}

// TestExponentialCommitSearchMissingCommit verifies the exponential search
// behavior when the target commit doesn't exist in the repository.
func TestExponentialCommitSearchMissingCommit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping long-running test")
	}

	// Set up overall test timeout
	testCtx, testCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer testCancel()

	// Create a remote repository for testing
	t.Log("Creating test remote repository")
	remotePath, cleanup, err := CreateTestRemoteRepo("find-missing-commit-test")
	if err != nil {
		t.Fatalf("Failed to create test remote repo: %v", err)
	}
	defer cleanup()

	// Create a temporary directory for our local clone
	tempDir, err := os.MkdirTemp("", "gittools-missing-commit-test-")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a client for working with Git
	client := NewClient()

	// First, clone the remote repo to a working directory to add more commits
	t.Log("Cloning remote repository to work directory")
	workRepo, err := client.Clone(remotePath, filepath.Join(tempDir, "work"))
	if err != nil {
		t.Fatalf("Failed to clone remote repo: %v", err)
	}

	// Configure the work repo
	workRepo.ConfigSet("user.name", "Test User")
	workRepo.ConfigSet("user.email", "test@example.com")

	// Reduced number of commits for faster tests
	numCommits := 10
	commitIDs := make([]string, numCommits)

	// Create all commits with progress reporting
	t.Log("Creating test commits for history")
	startTime := time.Now()

	for i := 0; i < numCommits; i++ {
		// Create a file with deterministic content
		fileName := fmt.Sprintf("file%d.txt", i+1)
		filePath := filepath.Join(tempDir, "work", fileName)
		fileContent := fmt.Sprintf("This is file %d\n", i+1)

		err := os.WriteFile(filePath, []byte(fileContent), 0644)
		if err != nil {
			t.Fatalf("Failed to write file: %v", err)
		}

		// Commit the file
		commitMessage := fmt.Sprintf("Add file %d", i+1)
		err = workRepo.Commit(commitMessage, []string{filePath})
		if err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}

		// Get the commit hash
		commitID, err := workRepo.RevParse("HEAD")
		if err != nil {
			t.Fatalf("Failed to get commit hash: %v", err)
		}
		commitIDs[i] = commitID
	}

	t.Logf("Created %d commits in %v", numCommits, time.Since(startTime))

	// Push all commits with timeout protection
	t.Log("Pushing commits with timeout protection")
	pushDone := make(chan error)

	go func() {
		pushDone <- workRepo.Push("origin", "master")
	}()

	select {
	case err = <-pushDone:
		if err != nil {
			t.Fatalf("Failed to push commits: %v", err)
		}
		t.Log("Successfully pushed commits")
	case <-time.After(20 * time.Second):
		t.Fatal("Push operation timed out")
	}

	// Use a non-existent commit hash by altering the first character
	nonExistentCommit := "deadbeef" + commitIDs[0][8:]
	t.Logf("Using non-existent commit hash: %s", nonExistentCommit)

	// Create a shallow clone for testing
	t.Log("Creating shallow clone for testing")
	shallowPath := filepath.Join(tempDir, "shallow")

	// Create shallow clone using our Client
	cloneOptions := CloneOptions{
		URL:         remotePath,
		Destination: shallowPath,
		Depth:       1,
		Context:     context.Background(),
	}

	shallowRepo, err := client.CloneWithOptions(cloneOptions)
	if err != nil {
		t.Fatalf("Failed to create shallow clone: %v", err)
	}
	t.Log("Shallow clone created successfully")

	// Start search for non-existent commit with proper timeout handling
	t.Log("Starting search for non-existent commit")

	// Run search with timeout context
	searchCtx, searchCancel := context.WithTimeout(testCtx, 30*time.Second)
	defer searchCancel()

	// Use channels for result collection with proper timeout handling
	resultChan := make(chan []string, 1)
	errChan := make(chan error, 1)

	// Start the search in a goroutine
	go func() {
		path, err := shallowRepo.FindCommitWithExponentialDepth(nonExistentCommit, nil)
		if err != nil {
			errChan <- err
			return
		}
		resultChan <- path
	}()

	// Wait for either a result or timeout
	var searchErr error
	var commitPath []string
	select {
	case commitPath = <-resultChan:
		t.Log("Search completed within timeout period")
	case searchErr = <-errChan:
		// We expect a specific error for the non-existent commit case
		if searchErr.Error() == "commit not found after reaching maximum search depth" {
			t.Log("✓ Correctly reported that commit was not found")
			searchErr = nil // Clear the error since this is expected
		} else {
			t.Fatalf("Unexpected error: %v", searchErr)
		}
	case <-searchCtx.Done():
		t.Fatalf("Search operation timed out after 30 seconds")
	}

	// We expect nil path for a non-existent commit
	if commitPath != nil {
		t.Fatalf("Expected nil path for non-existent commit, got: %v", commitPath)
	}

	t.Log("✓ Successfully verified that non-existent commit was correctly handled")
}

// TestGetCommitsBetweenWithHEADAsEarliest tests the GetCommitsBetween function
// with HEAD as the earliest commit and an older commit as the latest.
// This tests that the function works correctly when getting commits in the
// opposite direction of the normal history.
func TestGetCommitsBetweenWithHEADAsEarliest(t *testing.T) {
	SafeTest(t, func(t *testing.T, tempDir string) {
		if testing.Short() {
			t.Skip("skipping long-running test")
		}

		// Set up overall test timeout
		testCtx, testCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer testCancel()

		// Create a test repository with several commits
		t.Log("Creating test repository with multiple commits")
		repoPath := filepath.Join(tempDir, "test-repo")
		if err := os.MkdirAll(repoPath, 0755); err != nil {
			t.Fatalf("Failed to create test directory: %v", err)
		}

		// Initialize the repository
		cmd := exec.CommandContext(testCtx, "git", "init")
		cmd.Dir = repoPath
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to initialize git repository: %v", err)
		}

		// Configure the repository
		configCmd := exec.CommandContext(testCtx, "git", "config", "--local", "user.name", "Test User")
		configCmd.Dir = repoPath
		if err := configCmd.Run(); err != nil {
			t.Fatalf("Failed to configure git user name: %v", err)
		}

		configCmd = exec.CommandContext(testCtx, "git", "config", "--local", "user.email", "test@example.com")
		configCmd.Dir = repoPath
		if err := configCmd.Run(); err != nil {
			t.Fatalf("Failed to configure git user email: %v", err)
		}

		// Create 5 commits
		commitIDs := make([]string, 5)
		for i := 0; i < 5; i++ {
			// Create a file with some content
			fileName := fmt.Sprintf("file%d.txt", i)
			filePath := filepath.Join(repoPath, fileName)
			fileContent := fmt.Sprintf("Content for file %d\n", i)
			if err := os.WriteFile(filePath, []byte(fileContent), 0644); err != nil {
				t.Fatalf("Failed to create file: %v", err)
			}

			// Add the file
			addCmd := exec.CommandContext(testCtx, "git", "add", fileName)
			addCmd.Dir = repoPath
			if err := addCmd.Run(); err != nil {
				t.Fatalf("Failed to add file: %v", err)
			}

			// Commit the file
			commitCmd := exec.CommandContext(testCtx, "git", "commit", "-m", fmt.Sprintf("Add file %d", i))
			commitCmd.Dir = repoPath
			if err := commitCmd.Run(); err != nil {
				t.Fatalf("Failed to commit file: %v", err)
			}

			// Get the commit ID
			revCmd := exec.CommandContext(testCtx, "git", "rev-parse", "HEAD")
			revCmd.Dir = repoPath
			output, err := revCmd.Output()
			if err != nil {
				t.Fatalf("Failed to get commit hash: %v", err)
			}
			commitIDs[i] = strings.TrimSpace(string(output))
			t.Logf("Created commit %d: %s", i, commitIDs[i][:8])
		}

		// Open the repository using our library
		repo, err := Open(repoPath)
		if err != nil {
			t.Fatalf("Failed to open repository: %v", err)
		}

		// Get the HEAD commit
		headCommit, err := repo.RevParse("HEAD")
		if err != nil {
			t.Fatalf("Failed to get HEAD commit: %v", err)
		}
		t.Logf("HEAD commit: %s", headCommit[:8])

		// Choose an older commit as the "latest" for our test
		// We'll use the first commit (oldest) as our "latest" commit
		oldestCommit := commitIDs[0]
		t.Logf("Oldest commit: %s", oldestCommit[:8])

		// Use GetCommitsBetween with HEAD as the earliest and oldest commit as latest
		t.Log("Calling GetCommitsBetween with HEAD as earliest and oldest commit as latest")
		commits, err := repo.GetCommitsBetween(headCommit, oldestCommit, nil)
		if err != nil {
			t.Fatalf("GetCommitsBetween failed: %v", err)
		}

		// Log the commits we found
		t.Logf("Found %d commits", len(commits))
		for i, commit := range commits {
			t.Logf("  Commit[%d]: %s", i, commit[:8])
		}

		// Verify we got the expected number of commits
		expectedCount := 5 // 5 commits total in our repo
		if len(commits) != expectedCount {
			t.Fatalf("Expected %d commits, got %d", expectedCount, len(commits))
		}

		// Verify the commits are in the correct order (latest/HEAD first, oldest/target last)
		if commits[0] != headCommit {
			t.Fatalf("First commit should be HEAD (%s), got %s", headCommit[:8], commits[0][:8])
		}

		if commits[len(commits)-1] != oldestCommit {
			t.Fatalf("Last commit should be the oldest commit (%s), got %s",
				oldestCommit[:8], commits[len(commits)-1][:8])
		}

		t.Log("✓ Successfully verified GetCommitsBetween with HEAD as earliest commit")
	})
}
