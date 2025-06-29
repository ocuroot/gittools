package gittools

import (
	"errors"
	"fmt"
	"os"
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
	Client   *Client
	RepoPath string
}

// Open opens a GitRepo instance from an existing repository
func Open(repoPath string) (*Repo, error) {
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Walk up the directory tree to find the git repository root
	gitRoot, err := findGitRoot(absPath)
	if err != nil {
		return nil, err
	}

	// Use the git root as the repo path
	absPath = gitRoot

	return &Repo{
		Client:   &Client{WorkDir: absPath},
		RepoPath: absPath,
	}, nil
}

// findGitRoot walks up the directory tree to find the git repository root
func findGitRoot(startPath string) (string, error) {
	currentPath := startPath

	for {
		// Check if .git exists in the current directory
		gitPath := filepath.Join(currentPath, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			return currentPath, nil
		}

		// Get the parent directory
		parentPath := filepath.Dir(currentPath)

		// Check if we've reached the filesystem root
		if parentPath == currentPath {
			return "", fmt.Errorf("not a git repository: %s (or any of the parent directories)", startPath)
		}

		currentPath = parentPath
	}
}

func (g *Repo) CommitAll(message string) error {
	stdout, stderr, err := g.Client.Exec("add", "--all")
	if err != nil {
		return fmt.Errorf("git add failed: %w\nstdout: %s\nstderr: %s",
			err, stdout, stderr)
	}

	stdout, stderr, err = g.Client.Exec("commit", "-m", message)
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
		stdout, stderr, err := g.Client.Exec("add", file)
		if err != nil {
			return fmt.Errorf("git add failed for %s: %w\nstdout: %s\nstderr: %s",
				file, err, stdout, stderr)
		}
	}

	// Commit the changes
	stdout, stderr, err := g.Client.Exec("commit", "-m", message)
	if err != nil {
		return fmt.Errorf("git commit failed: %w\nstdout: %s\nstderr: %s",
			err, stdout, stderr)
	}

	return nil
}

// Fetch fetches updates from the specified remote
func (g *Repo) Fetch(remote string) error {
	stdout, stderr, err := g.Client.Exec("fetch", remote)
	if err != nil {
		return fmt.Errorf("git fetch failed: %w\nstdout: %s\nstderr: %s",
			err, stdout, stderr)
	}

	return nil
}

// Pull pulls changes from the specified remote and branch
func (g *Repo) Pull(remote, branch string) error {
	stdout, stderr, err := g.Client.Exec("pull", remote, branch)
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
	stdout, stderr, err := g.Client.Exec("push", "--porcelain", remote, branch)
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
	stdout, stderr, err := g.Client.Exec("checkout", branch)
	if err != nil {
		return fmt.Errorf("git checkout failed: %w\nstdout: %s\nstderr: %s",
			err, stdout, stderr)
	}

	return nil
}

// CreateBranch creates a new branch from the current HEAD
func (g *Repo) CreateBranch(branch string) error {
	stdout, stderr, err := g.Client.Exec("branch", branch)
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

	stdout, stderr, err := g.Client.Exec("reset", opts.Mode.String(), opts.Target)
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
	stdout, stderr, err := g.Client.Exec("rebase", onto)
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
	stdout, stderr, err := g.Client.Exec("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		stdout2, stderr2, err2 := g.Client.Exec("branch")
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

// RebaseAbort aborts the current rebase
func (g *Repo) RebaseAbort() error {
	stdout, stderr, err := g.Client.Exec("rebase", "--abort")
	if err != nil {
		return fmt.Errorf("git rebase abort failed: %w\nstdout: %s\nstderr: %s",
			err, stdout, stderr)
	}

	return nil
}

// ConfigSet sets a git config value for the repository
func (c *Repo) ConfigSet(key, value string) error {
	// --local is the default, but setting it here to be explicit
	_, _, err := c.Client.Exec("config", "set", "--local", key, value)
	return err
}

// ConfigGet gets a git config value for the repository
func (c *Repo) ConfigGet(key string) (string, error) {
	// --local is the default, but setting it here to be explicit
	stdout, stderr, err := c.Client.Exec("config", "get", "--local", key)
	if err != nil {
		return "", fmt.Errorf("git config get failed: %w\nstdout: %s\nstderr: %s",
			err, stdout, stderr)
	}
	return string(stdout), nil
}

// AddRemote adds a remote to the repository
func (c *Repo) AddRemote(remote string, url string) error {
	_, _, err := c.Client.Exec("remote", "add", remote, url)
	return err
}

// RemoteURL returns the URL of a remote
// If push is true, returns the push URL, otherwise the fetch URL
func (r *Repo) RemoteURL(remote string, push bool) (string, error) {
	args := []string{"remote", "get-url", remote}
	if push {
		args = append(args, "--push")
	}
	stdout, stderr, err := r.Client.Exec(args...)
	if err != nil {
		return "", fmt.Errorf("git remote get-url failed: %w\nstdout: %s\nstderr: %s",
			err, stdout, stderr)
	}
	return string(stdout), nil
}

// FileAtCommit returns the content of a file at a specific commit
func (r *Repo) FileAtCommit(commit string, path string) (string, error) {
	stdout, stderr, err := r.Client.Exec("show", commit+":"+path)
	if err != nil {
		return "", fmt.Errorf("git show failed: %w\nstdout: %s\nstderr: %s",
			err, stdout, stderr)
	}
	return string(stdout), nil
}

type DiffOptions struct {
	NoPatch  bool
	NameOnly bool
	Paths    []string
	Cached   bool
	Unified  bool
	Raw      bool
}

func (r *Repo) Diff(options DiffOptions, commits ...string) (string, error) {
	args := []string{"diff"}
	if options.NoPatch {
		args = append(args, "--no-patch")
	}
	if options.NameOnly {
		args = append(args, "--name-only")
	}
	if options.Cached {
		args = append(args, "--cached")
	}
	if options.Unified {
		args = append(args, "--unified")
	}
	if options.Raw {
		args = append(args, "--raw")
	}
	args = append(args, commits...)
	if len(options.Paths) > 0 {
		args = append(args, "--")
		args = append(args, options.Paths...)
	}

	stdout, stderr, err := r.Client.Exec(args...)
	if err != nil {
		return "", fmt.Errorf("git diff failed: %w\nstdout: %s\nstderr: %s",
			err, stdout, stderr)
	}
	return string(stdout), nil
}

type LsFilesOptions struct {
	Cached              bool
	Deleted             bool
	Others              bool
	Ignored             bool
	Stage               bool
	Unmerged            bool
	Killed              bool
	Modified            bool
	ResolveUndo         bool
	Directory           bool
	NoEmptyDirectory    bool
	Eol                 bool
	Deduplicate         bool
	Exclude             []string
	ExcludeFrom         []string
	ExcludePerDirectory []string
	ExcludeStandard     bool
	ErrorUnmatch        bool
	WithTree            string
	FullName            bool
	RecurseSubmodules   bool
	Abbrev              int
	Format              string
	Paths               []string
}

func (r *Repo) LsFiles(options LsFilesOptions) (string, error) {
	args := []string{"ls-files"}
	if options.Cached {
		args = append(args, "-c", "--cached")
	}
	if options.Deleted {
		args = append(args, "-d", "--deleted")
	}
	if options.Others {
		args = append(args, "-o", "--others")
	}
	if options.Ignored {
		args = append(args, "-i", "--ignored")
	}
	if options.Stage {
		args = append(args, "-s", "--stage")
	}
	if options.Unmerged {
		args = append(args, "-u", "--unmerged")
	}
	if options.Killed {
		args = append(args, "-k", "--killed")
	}
	if options.Modified {
		args = append(args, "-m", "--modified")
	}
	if options.ResolveUndo {
		args = append(args, "--resolve-undo")
	}
	if options.Directory {
		args = append(args, "--directory")
	}
	if options.NoEmptyDirectory {
		args = append(args, "--no-empty-directory")
	}
	if options.Eol {
		args = append(args, "--eol")
	}
	if options.Deduplicate {
		args = append(args, "--deduplicate")
	}
	for _, path := range options.Exclude {
		args = append(args, "-x", path)
	}
	for _, path := range options.ExcludeFrom {
		args = append(args, "-X", path)
	}
	for _, path := range options.ExcludePerDirectory {
		args = append(args, "--exclude-per-directory", path)
	}
	if options.ExcludeStandard {
		args = append(args, "--exclude-standard")
	}
	if options.ErrorUnmatch {
		args = append(args, "--error-unmatch")
	}
	if options.WithTree != "" {
		args = append(args, "--with-tree", options.WithTree)
	}
	if options.FullName {
		args = append(args, "--full-name")
	}
	if options.RecurseSubmodules {
		args = append(args, "--recurse-submodules")
	}
	if options.Abbrev != 0 {
		args = append(args, fmt.Sprintf("--abbrev=%d", options.Abbrev))
	}
	if options.Format != "" {
		args = append(args, fmt.Sprintf("--format=%s", options.Format))
	}
	if len(options.Paths) > 0 {
		args = append(args, "--")
		args = append(args, options.Paths...)
	}
	stdout, stderr, err := r.Client.Exec(args...)
	if err != nil {
		return "", fmt.Errorf("git ls-files failed: %w\nstdout: %s\nstderr: %s",
			err, stdout, stderr)
	}
	return string(stdout), nil
}
