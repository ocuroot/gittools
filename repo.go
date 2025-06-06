package gittools

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Git push error types
var (
	// ErrPushRejected is returned when a push is rejected
	ErrPushRejected = errors.New("git push rejected")

	// ErrPushFetchFirst is returned when a push is rejected due to non-fast-forward changes
	ErrPushFetchFirst = errors.New("git push rejected: fetch-first update")

	// ErrPushNonFastForward is returned when a push is rejected due to non-fast-forward changes
	ErrPushNonFastForward = errors.New("git push rejected: non-fast-forward update")

	// ErrPushPermissionDenied is returned when a push is rejected due to permission issues
	ErrPushPermissionDenied = errors.New("git push rejected: permission denied")

	// ErrPushRemoteRefMissing is returned when the remote reference does not exist
	ErrPushRemoteRefMissing = errors.New("git push rejected: remote ref does not exist")
)

// Git rebase error types
var (
	// ErrRebaseMergeConflict is returned when a rebase encounters merge conflicts
	ErrRebaseMergeConflict = errors.New("git rebase failed: merge conflict")

	// ErrRebaseAlreadyInProgress is returned when attempting to rebase while another rebase is in progress
	ErrRebaseAlreadyInProgress = errors.New("git rebase failed: rebase already in progress")

	// ErrRebaseNoCommitsApplied is returned when a rebase doesn't apply any commits
	ErrRebaseNoCommitsApplied = errors.New("git rebase failed: no commits applied")
)

// Repo represents a Git repository
type Repo struct {
	RepoPath string
}

// New creates a GitRepo instance from an existing repository
func New(repoPath string) (*Repo, error) {
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Check if the directory exists and is a git repository
	if _, err := os.Stat(filepath.Join(absPath, ".git")); os.IsNotExist(err) {
		return nil, fmt.Errorf("not a git repository: %s", absPath)
	}

	return &Repo{
		RepoPath: absPath,
	}, nil
}

// Clone clones a git repository and returns a GitRepo instance
func Clone(url, destination string) (*Repo, error) {
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

	return &Repo{
		RepoPath: absPath,
	}, nil
}

// execGitCommand executes a git command in the repository directory
// Returns stdout, stderr, error
func (g *Repo) execGitCommand(args ...string) ([]byte, []byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.RepoPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func (g *Repo) CommitAll(message string) error {
	stdout, stderr, err := g.execGitCommand("add", "--all")
	if err != nil {
		return fmt.Errorf("git add failed: %w\nstdout: %s\nstderr: %s",
			err, stdout, stderr)
	}

	stdout, stderr, err = g.execGitCommand("commit", "-m", message)
	if err != nil {
		return fmt.Errorf("git commit failed: %w\nstdout: %s\nstderr: %s",
			err, stdout, stderr)
	}

	return nil
}

// Commit stages and commits the specified files
func (g *Repo) Commit(message string, files []string) error {
	// Add the files
	for _, file := range files {
		stdout, stderr, err := g.execGitCommand("add", file)
		if err != nil {
			return fmt.Errorf("git add failed for %s: %w\nstdout: %s\nstderr: %s",
				file, err, stdout, stderr)
		}
	}

	// Commit the changes
	stdout, stderr, err := g.execGitCommand("commit", "-m", message)
	if err != nil {
		return fmt.Errorf("git commit failed: %w\nstdout: %s\nstderr: %s",
			err, stdout, stderr)
	}

	return nil
}

// Fetch fetches updates from the specified remote
func (g *Repo) Fetch(remote string) error {
	stdout, stderr, err := g.execGitCommand("fetch", remote)
	if err != nil {
		return fmt.Errorf("git fetch failed: %w\nstdout: %s\nstderr: %s",
			err, stdout, stderr)
	}

	return nil
}

// Pull pulls changes from the specified remote and branch
func (g *Repo) Pull(remote, branch string) error {
	stdout, stderr, err := g.execGitCommand("pull", remote, branch)
	if err != nil {
		return fmt.Errorf("git pull failed: %w\nstdout: %s\nstderr: %s",
			err, stdout, stderr)
	}

	return nil
}

// Push pushes changes to the specified remote and branch
func (g *Repo) Push(remote, branch string) error {
	err := g.Fetch(remote)
	if err != nil {
		return fmt.Errorf("git fetch failed: %w", err)
	}
	stdout, stderr, err := g.execGitCommand("push", "--porcelain", remote, branch)
	if err != nil {
		// Parse the output to determine the specific error type
		outputStr := string(stdout)
		stderrStr := string(stderr)
		combinedOutput := outputStr + stderrStr

		switch {
		case strings.Contains(combinedOutput, "fetch first"):
			return fmt.Errorf("%w: %s", ErrPushFetchFirst, combinedOutput)
		case strings.Contains(combinedOutput, "non-fast-forward"):
			return fmt.Errorf("%w: %s", ErrPushNonFastForward, combinedOutput)

		case strings.Contains(combinedOutput, "permission denied") || strings.Contains(combinedOutput, "access denied"):
			return fmt.Errorf("%w: %s", ErrPushPermissionDenied, combinedOutput)

		case strings.Contains(combinedOutput, "! [remote rejected]") || strings.Contains(combinedOutput, "! [rejected]"):
			return fmt.Errorf("%w: %s", ErrPushRejected, combinedOutput)

		case strings.Contains(combinedOutput, "couldn't find remote ref") || strings.Contains(combinedOutput, "remote ref does not exist"):
			return fmt.Errorf("%w: %s", ErrPushRemoteRefMissing, combinedOutput)

		default:
			// Generic push error
			return fmt.Errorf("git push failed: %w\nstdout: %s\nstderr: %s", err, stdout, stderr)
		}
	}

	return nil
}

// Checkout switches to the specified branch
func (g *Repo) Checkout(branch string) error {
	stdout, stderr, err := g.execGitCommand("checkout", branch)
	if err != nil {
		return fmt.Errorf("git checkout failed: %w\nstdout: %s\nstderr: %s",
			err, stdout, stderr)
	}

	return nil
}

// CreateBranch creates a new branch from the current HEAD
func (g *Repo) CreateBranch(branch string) error {
	stdout, stderr, err := g.execGitCommand("branch", branch)
	if err != nil {
		return fmt.Errorf("git branch failed: %w\nstdout: %s\nstderr: %s",
			err, stdout, stderr)
	}

	return nil
}

// ResetMode represents the Git reset mode
type ResetMode int

// Reset modes
const (
	ResetSoft  ResetMode = iota // Keep changes in working dir and index
	ResetMixed                  // Keep changes in working dir but not in index
	ResetHard                   // Discard all changes
)

// String returns the Git command line option for this reset mode
func (m ResetMode) String() string {
	switch m {
	case ResetSoft:
		return "--soft"
	case ResetMixed:
		return "--mixed"
	case ResetHard:
		return "--hard"
	default:
		return "--mixed" // Default to mixed mode
	}
}

// ResetOptions defines options for resetting a Git repository
type ResetOptions struct {
	// Mode determines how the working directory and index are affected
	Mode ResetMode

	// Target is the commit to reset to (e.g., "HEAD~1", commit hash, branch name)
	Target string
}

// Reset resets the repository to a specific commit
func (g *Repo) Reset(opts ResetOptions) error {
	// Validate that a target is provided
	if opts.Target == "" {
		return fmt.Errorf("reset target cannot be empty; specify a commit, branch, or reference")
	}

	stdout, stderr, err := g.execGitCommand("reset", opts.Mode.String(), opts.Target)
	if err != nil {
		return fmt.Errorf("git reset failed: %w\nstdout: %s\nstderr: %s",
			err, stdout, stderr)
	}

	return nil
}

// ResetHard is a convenience method for hard reset to a target
func (g *Repo) ResetHard(target string) error {
	return g.Reset(ResetOptions{
		Mode:   ResetHard,
		Target: target,
	})
}

// Rebase rebases the current branch onto the specified branch or commit
func (g *Repo) Rebase(onto string) error {
	stdout, stderr, err := g.execGitCommand("rebase", onto)
	if err != nil {
		// Parse output to determine specific error type
		stdoutStr := string(stdout)
		stderrStr := string(stderr)
		combinedOutput := stdoutStr + stderrStr

		switch {
		case strings.Contains(combinedOutput, "CONFLICT") || strings.Contains(combinedOutput, "Merge conflict"):
			return fmt.Errorf("%w: %s", ErrRebaseMergeConflict, combinedOutput)

		case strings.Contains(combinedOutput, "already in progress") || strings.Contains(combinedOutput, "rebase-merge directory"):
			return fmt.Errorf("%w: %s", ErrRebaseAlreadyInProgress, combinedOutput)

		case strings.Contains(combinedOutput, "no commits applied"):
			return fmt.Errorf("%w: %s", ErrRebaseNoCommitsApplied, combinedOutput)

		default:
			return fmt.Errorf("git rebase failed: %w\nstdout: %s\nstderr: %s",
				err, stdout, stderr)
		}
	}

	return nil
}

// CurrentBranch returns the name of the current branch
func (g *Repo) CurrentBranch() (string, error) {
	stdout, stderr, err := g.execGitCommand("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		stdout2, stderr2, err2 := g.execGitCommand("branch")
		if err2 != nil {
			return "", fmt.Errorf("failed to get branch info: %w\nstdout: %s\nstderr: %s",
				err2, stdout2, stderr2)
		}
		fmt.Println(string(stdout2))

		return "", fmt.Errorf("failed to get current branch: %w\nstdout: %s\nstderr: %s",
			err, stdout, stderr)
	}

	return strings.TrimSpace(string(stdout)), nil
}

func (g *Repo) RebaseAbort() error {
	stdout, stderr, err := g.execGitCommand("rebase", "--abort")
	if err != nil {
		return fmt.Errorf("git rebase abort failed: %w\nstdout: %s\nstderr: %s",
			err, stdout, stderr)
	}

	return nil
}
