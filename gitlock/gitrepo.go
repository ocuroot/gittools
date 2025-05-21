package gitlock

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

// ErrLockConflict is returned when a lock acquisition fails due to the resource being locked
var ErrLockConflict = errors.New("lock conflict: resource is already locked")

// GitRepo represents a Git repository with locking capabilities
type GitRepo struct {
	RepoPath string
	LockKey  string // ULID for identifying this process
}

// Lock represents a lock on a resource
type Lock struct {
	Owner       string    `json:"owner"` // ULID of the process holding the lock
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	Description string    `json:"description,omitempty"`
}

// New creates a GitRepo instance from an existing repository
func New(repoPath string) (*GitRepo, error) {
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Check if the directory exists and is a git repository
	if _, err := os.Stat(filepath.Join(absPath, ".git")); os.IsNotExist(err) {
		return nil, fmt.Errorf("not a git repository: %s", absPath)
	}

	// Generate a ULID for this instance
	id := ulid.Make().String()

	return &GitRepo{
		RepoPath: absPath,
		LockKey:  id,
	}, nil
}

// Clone clones a git repository and returns a GitRepo instance
func Clone(url, destination string) (*GitRepo, error) {
	absPath, err := filepath.Abs(destination)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Execute git clone
	cmd := exec.Command("git", "clone", url, absPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git clone failed: %s: %w", output, err)
	}

	// After cloning, explicitly check out the main branch
	cmd = exec.Command("git", "checkout", "main")
	cmd.Dir = absPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to checkout main branch: %s: %w", output, err)
	}

	// Generate a ULID for this instance
	id := ulid.Make().String()

	return &GitRepo{
		RepoPath: absPath,
		LockKey:  id,
	}, nil
}

// execGitCommand executes a git command in the repository directory
func (g *GitRepo) execGitCommand(args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.RepoPath
	return cmd.CombinedOutput()
}

// Commit stages and commits the specified files
func (g *GitRepo) Commit(message string, files []string) error {
	// Add the files
	for _, file := range files {
		_, err := g.execGitCommand("add", file)
		if err != nil {
			return fmt.Errorf("git add failed for %s: %w", file, err)
		}
	}

	// Commit the changes
	_, err := g.execGitCommand("commit", "-m", message)
	if err != nil {
		return fmt.Errorf("git commit failed: %w", err)
	}

	return nil
}

// Fetch fetches updates from the specified remote
func (g *GitRepo) Fetch(remote string) error {
	_, err := g.execGitCommand("fetch", remote)
	if err != nil {
		return fmt.Errorf("git fetch failed: %w", err)
	}

	return nil
}

// Pull pulls changes from the specified remote and branch
func (g *GitRepo) Pull(remote, branch string) error {
	output, err := g.execGitCommand("pull", remote, branch)
	if err != nil {
		return fmt.Errorf("git pull failed: %w\n%s", err, string(output))
	}

	return nil
}

// Push pushes changes to the specified remote and branch
func (g *GitRepo) Push(remote, branch string) error {
	_, err := g.execGitCommand("push", remote, branch)
	if err != nil {
		return fmt.Errorf("git push failed: %w", err)
	}

	return nil
}

// Checkout switches to the specified branch
func (g *GitRepo) Checkout(branch string) error {
	_, err := g.execGitCommand("checkout", branch)
	if err != nil {
		return fmt.Errorf("git checkout failed: %w", err)
	}

	return nil
}

// CreateBranch creates a new branch from the current HEAD
func (g *GitRepo) CreateBranch(branch string) error {
	_, err := g.execGitCommand("checkout", "-b", branch)
	if err != nil {
		return fmt.Errorf("git branch creation failed: %w", err)
	}

	return nil
}

// CurrentBranch returns the name of the current branch
func (g *GitRepo) CurrentBranch() (string, error) {
	output, err := g.execGitCommand("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		output2, err2 := g.execGitCommand("branch")
		if err2 != nil {
			return "", err2
		}
		fmt.Println(string(output2))

		return "", fmt.Errorf("failed to get current branch: %w (%v)", err, string(output))
	}

	return strings.TrimSpace(string(output)), nil
}
