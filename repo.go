package gittools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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

type FetchOptions struct {
	Depth int
}

// Fetch fetches updates from the specified remote
func (g *Repo) Fetch(remote string, options FetchOptions) error {
	args := []string{"fetch", remote}
	if options.Depth != 0 {
		args = append(args, fmt.Sprintf("--depth=%d", options.Depth))
	}
	stdout, stderr, err := g.Client.Exec(args...)
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
	err := g.Fetch(remote, FetchOptions{})
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

// RevParse executes git rev-parse with the given arguments
// Common usages include getting HEAD commit (RevParse("HEAD")),
// checking if a string is a valid reference (RevParse("--verify", ref)),
// and getting the short version of a commit (RevParse("--short", commit))
func (r *Repo) RevParse(args ...string) (string, error) {
	cmdArgs := append([]string{"rev-parse"}, args...)
	stdout, stderr, err := r.Client.Exec(cmdArgs...)
	if err != nil {
		return "", fmt.Errorf("git rev-parse failed: %w\nstdout: %s\nstderr: %s",
			err, stdout, stderr)
	}
	return strings.TrimSpace(string(stdout)), nil
}

// CatFileOptions defines options for the git cat-file command
type CatFileOptions struct {
	// Check if object exists (-e)
	Exists bool

	// Show object type (-t)
	ShowType bool

	// Show object content (-p)
	ShowContent bool

	// Show object size (-s)
	ShowSize bool

	// Object ID (commit hash, tag, etc)
	ObjectID string
}

// CatFile executes git cat-file with the provided options
// Returns:
// - exists: true if the object exists, false if not
// - content: object content, type or size depending on options (empty for -e)
// - error: system errors only, not "object not found" which is indicated by exists=false
func (r *Repo) CatFile(options CatFileOptions) (exists bool, content string, err error) {
	if options.ObjectID == "" {
		return false, "", fmt.Errorf("empty object ID provided to CatFile")
	}

	// Build arguments based on options
	args := []string{"cat-file"}

	if options.Exists {
		args = append(args, "-e")
	} else if options.ShowType {
		args = append(args, "-t")
	} else if options.ShowContent {
		args = append(args, "-p")
	} else if options.ShowSize {
		args = append(args, "-s")
	} else {
		// Default to -e if no option specified
		args = append(args, "-e")
		options.Exists = true
	}

	args = append(args, options.ObjectID)

	// Execute the command
	stdout, stderr, err := r.Client.Exec(args...)

	if err != nil {
		// For -e flag, exit status 1 means the object doesn't exist
		// This is expected behavior, not an error condition
		if options.Exists {
			return false, "", nil
		}

		// For other operations, check common error messages that indicate
		// a non-existent object rather than a system error
		if strings.Contains(string(stderr), "Not a valid object name") ||
			strings.Contains(string(stderr), "does not exist") ||
			strings.Contains(string(stderr), "could not get object") ||
			strings.Contains(string(stderr), "fatal: not a valid object") {
			return false, "", nil
		}

		return false, "", fmt.Errorf("git cat-file failed: %w\nstderr: %s", err, stderr)
	}

	// Command succeeded, object exists
	return true, strings.TrimSpace(string(stdout)), nil
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

type LogItem struct {
	Commit  string
	Author  string
	Date    string
	Message string
	Tags    []string
}

type LogOptions struct {
	Source   bool
	Oneline  bool
	Decorate bool
	Tags     bool

	Commit1 string
	Commit2 string
}

// parseOnelineFormat parses git log output in the oneline format and returns a slice of LogItems.
// Format example: "hash (refs) message"
func parseOnelineFormat(output string) []LogItem {
	var logItems []LogItem

	// Parse oneline format: "hash (refs) message"
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}

		// Extract parts from the oneline format
		item := LogItem{}

		// Extract commit hash (first part of the line before space)
		parts := strings.SplitN(line, " ", 2)
		if len(parts) > 0 {
			item.Commit = parts[0]
		}

		// Check for tag references and message
		if len(parts) > 1 {
			rest := parts[1]
			// Look for refs section: (tag: v1.0.0, ...)
			if strings.Contains(rest, "(") && strings.Contains(rest, ")") {
				refStart := strings.Index(rest, "(")
				refEnd := strings.Index(rest, ")")
				if refStart != -1 && refEnd != -1 && refEnd > refStart {
					refSection := rest[refStart+1 : refEnd]
					// Extract message after the refs section
					if refEnd+2 < len(rest) {
						item.Message = strings.TrimSpace(rest[refEnd+2:])
					}

					// Extract tags from refs
					refs := strings.Split(refSection, ",")
					for _, ref := range refs {
						ref = strings.TrimSpace(ref)
						if strings.HasPrefix(ref, "tag: ") {
							tag := strings.TrimPrefix(ref, "tag: ")
							item.Tags = append(item.Tags, tag)
						}
					}
				} else {
					// No proper refs format, treat everything as message
					item.Message = rest
				}
			} else {
				// No refs section, treat everything as message
				item.Message = rest
			}
		}

		logItems = append(logItems, item)
	}

	return logItems
}

// parseMultilineFormat parses git log output in the full format and returns a slice of LogItems.
// Format example:
// commit hash (refs)
// Author: author
// Date: date
//
//	message
func parseMultilineFormat(output string) []LogItem {
	var logItems []LogItem

	// Parse full format
	lines := strings.Split(output, "\n")
	var currentItem *LogItem
	var collectingMessage bool

	for _, line := range lines {
		if strings.HasPrefix(line, "commit ") {
			// Start a new commit
			if currentItem != nil {
				logItems = append(logItems, *currentItem)
			}

			currentItem = &LogItem{}
			collectingMessage = false

			// Extract commit hash and refs
			commitLine := strings.TrimPrefix(line, "commit ")
			parts := strings.SplitN(commitLine, " ", 2)
			if len(parts) > 0 {
				currentItem.Commit = parts[0]
			}

			// Extract tags from refs if present
			if len(parts) > 1 && strings.HasPrefix(parts[1], "(") && strings.Contains(parts[1], ")") {
				refSection := parts[1]
				refSection = strings.TrimPrefix(refSection, "(")
				refSection = strings.TrimSuffix(refSection, ")")

				refs := strings.Split(refSection, ",")
				for _, ref := range refs {
					ref = strings.TrimSpace(ref)
					if strings.HasPrefix(ref, "tag: ") {
						tag := strings.TrimPrefix(ref, "tag: ")
						currentItem.Tags = append(currentItem.Tags, tag)
					}
				}
			}
		} else if strings.HasPrefix(line, "Author: ") {
			if currentItem != nil {
				currentItem.Author = strings.TrimPrefix(line, "Author: ")
			}
		} else if strings.HasPrefix(line, "Date: ") {
			if currentItem != nil {
				currentItem.Date = strings.TrimSpace(strings.TrimPrefix(line, "Date: "))
			}
		} else if strings.TrimSpace(line) == "" {
			// Empty line after date marks the start of the commit message
			collectingMessage = true
		} else if collectingMessage && currentItem != nil {
			// Collecting the commit message
			line = strings.TrimSpace(line)
			if currentItem.Message == "" {
				currentItem.Message = line
			} else {
				currentItem.Message += "\n" + line
			}
		}
	}

	// Add the last item
	if currentItem != nil {
		logItems = append(logItems, *currentItem)
	}

	return logItems
}

type RevListOptions struct {
	// Range is the commit range to list commits from (e.g. "commit1..commit2")
	Range string

	// AncestryPath ensures that the returned commits are in the direct path
	AncestryPath bool

	// Count returns only the count of commits instead of commit hashes
	Count bool

	// MaxCount limits the number of commits returned
	MaxCount int
}

// RevList runs git rev-list with the specified options and returns the list of commit hashes
func (r *Repo) RevList(options RevListOptions) ([]string, error) {
	args := []string{"rev-list"}

	if options.AncestryPath {
		args = append(args, "--ancestry-path")
	}

	if options.Count {
		args = append(args, "--count")
	}

	if options.MaxCount > 0 {
		args = append(args, "--max-count", fmt.Sprintf("%d", options.MaxCount))
	}

	// Add the range
	if options.Range != "" {
		args = append(args, options.Range)
	}

	// Execute the command
	stdout, stderr, err := r.Client.Exec(args...)
	if err != nil {
		return nil, fmt.Errorf("git rev-list failed: %w\nstdout: %s\nstderr: %s",
			err, stdout, stderr)
	}

	// Split output into lines and clean each line
	output := strings.TrimSpace(string(stdout))
	if output == "" {
		return []string{}, nil
	}

	commits := strings.Split(output, "\n")
	for i, commit := range commits {
		commits[i] = strings.TrimSpace(commit)
	}

	return commits, nil
}

// CountCommits returns the number of commits from a reference (HEAD by default)
// This is a convenience method for RevList with the --count option
func (r *Repo) CountCommits(ref string) (int, error) {
	if ref == "" {
		ref = "HEAD"
	}

	options := RevListOptions{
		Count: true,
		Range: ref,
	}

	commits, err := r.RevList(options)
	if err != nil {
		return 0, err
	}

	if len(commits) == 0 {
		return 0, nil
	}

	// Parse the count result
	count, err := strconv.Atoi(commits[0])
	if err != nil {
		return 0, fmt.Errorf("failed to parse commit count: %w", err)
	}

	return count, nil
}

// GetCommitsBetweenOptions defines configuration options for the commit search operation
type GetCommitsBetweenOptions struct {
	// MaxDepth is the maximum depth to search for commits
	// Default: 1024
	MaxDepth int

	// OperationTimeout is the timeout for each individual git operation
	// within the commit search (fetch, rev-list, etc.)
	// Default: 30 seconds
	OperationTimeout time.Duration

	// DoNotExpandDepth controls whether the search can expand the clone depth
	// If true, the search will only use the existing clone depth and not fetch additional history
	// This is useful when you want to only search within the currently available history
	// Default: false (depth can be expanded)
	DoNotExpandDepth bool
}

// DefaultCommitSearchOptions returns the default options for commit search
func DefaultCommitSearchOptions() *GetCommitsBetweenOptions {
	return &GetCommitsBetweenOptions{
		MaxDepth:         1024,
		OperationTimeout: 30 * time.Second,
		DoNotExpandDepth: false,
	}
}

// GetCommitsBetween retrieves all commits between two specified commits (inclusive).
// It uses an exponential backoff strategy for fetching, doubling the fetch depth with each iteration
// until both commits are found.
//
// If opts.DoNotExpandDepth is true, the function will only look in the existing repository
// and not perform any fetches to expand history. This is useful when you want to work only
// with the commits already available locally, such as in CI pipelines where network access
// is limited or when performance is critical.
//
// This approach drastically reduces the number of network round trips needed compared to
// a linear depth increase, especially for commits deep in the history.
//
// Parameters:
//   - earliestCommit: The earlier commit hash
//   - latestCommit: The later commit hash
//   - opts: Optional configuration options. If nil, default options will be used.
//
// Returns the list of commits between earliestCommit and latestCommit (inclusive) if found.
// Returns nil if one or both commits were not found.
// Also returns an error if any issues occurred during the search.
func (r *Repo) GetCommitsBetween(earliestCommit string, latestCommit string, opts *GetCommitsBetweenOptions) ([]string, error) {
	// Use default options if none provided
	if opts == nil {
		opts = DefaultCommitSearchOptions()
	}

	// Validate inputs
	if earliestCommit == "" {
		return nil, fmt.Errorf("empty earliest commit")
	}
	if latestCommit == "" {
		return nil, fmt.Errorf("empty latest commit")
	}

	// Check if both commits already exist in the repository before attempting any fetches
	earliestExists, err := r.commitExists(earliestCommit, opts.OperationTimeout)
	if err != nil {
		return nil, fmt.Errorf("error checking if earliest commit exists: %w", err)
	}

	latestExists, err := r.commitExists(latestCommit, opts.OperationTimeout)
	if err != nil {
		return nil, fmt.Errorf("error checking if latest commit exists: %w", err)
	}

	if earliestExists && latestExists {
		// Both commits exist, get commits between them
		return r.getCommitsBetween(earliestCommit, latestCommit, opts.OperationTimeout)
	}

	// If DoNotExpandDepth is true, don't attempt fetching more history
	if opts.DoNotExpandDepth {
		// If commits don't exist and we're not allowed to expand depth, return nil
		return nil, nil
	}

	// Initialize the depth and maximum depth
	depth := 1
	maxDepth := opts.MaxDepth

	// Get the remote name - typically "origin" in most repositories
	remote := "origin"

	// Keep doubling the depth until we reach maxDepth
	for depth <= maxDepth {
		// Create fetch options with the current depth
		fetchOpts := FetchOptions{
			Depth: depth,
		}

		// Attempt to fetch with the current depth

		// Use a bounded timeout for each fetch operation
		fetchCtx, fetchCancel := context.WithTimeout(context.Background(), opts.OperationTimeout)

		// Use a channel to manage fetch operation with timeout
		done := make(chan error, 1)
		go func() {
			done <- r.Fetch(remote, fetchOpts)
		}()

		// Wait for either fetch completion or timeout
		var fetchErr error
		select {
		case fetchErr = <-done:
			// Fetch completed
		case <-fetchCtx.Done():
			fetchCancel()
			return nil, fmt.Errorf("fetch operation timed out after %v at depth %d", opts.OperationTimeout, depth)
		}

		// Clean up the context regardless of outcome
		fetchCancel()

		if fetchErr != nil {
			return nil, fmt.Errorf("error fetching from repository with depth %d: %w", depth, fetchErr)
		}

		// Check if both commits are now available in the repository
		earliestExists, err := r.commitExists(earliestCommit, opts.OperationTimeout)
		if err != nil {
			return nil, fmt.Errorf("error checking if earliest commit exists: %w", err)
		}

		latestExists, err := r.commitExists(latestCommit, opts.OperationTimeout)
		if err != nil {
			return nil, fmt.Errorf("error checking if latest commit exists: %w", err)
		}

		if earliestExists && latestExists {
			return r.getCommitsBetween(earliestCommit, latestCommit, opts.OperationTimeout)
		}

		// Double the depth for next iteration
		depth *= 2
	}

	// If we've reached this point, we didn't find the commit within maxDepth
	return nil, nil
}

// FindCommitWithExponentialDepth is a backward compatibility wrapper for GetCommitsBetween
// that keeps existing code working while using the new implementation.
// It searches for a commit using exponential depth fetching and returns the path from HEAD to the target.
//
// Deprecated: Use GetCommitsBetween instead.
func (r *Repo) FindCommitWithExponentialDepth(targetCommit string, opts *GetCommitsBetweenOptions) ([]string, error) {
	// Default to HEAD as the latest commit
	latestCommit, err := r.RevParse("HEAD")
	if err != nil {
		return nil, fmt.Errorf("error getting HEAD commit: %w", err)
	}

	// Use the new implementation to get commits between target and HEAD
	// For backward compatibility, the path should be from HEAD to target commit
	return r.GetCommitsBetween(targetCommit, latestCommit, opts)
}

// getCommitsBetween retrieves all commits between two specified commits (inclusive).
// The commits are returned in chronological order from earliestCommit to latestCommit.
// isShallowClone checks if the repository is a shallow clone by checking for the existence of .git/shallow file
func (r *Repo) isShallowClone() bool {
	shallowFilePath := filepath.Join(r.RepoPath, ".git", "shallow")
	_, err := os.Stat(shallowFilePath)
	return err == nil
}

func (r *Repo) getCommitsBetween(earliestCommit string, latestCommit string, timeout time.Duration) ([]string, error) {
	if earliestCommit == "" || latestCommit == "" {
		return nil, fmt.Errorf("both commits must be specified")
	}

	// First, try to determine if this is a shallow clone
	isShallow := r.isShallowClone()

	// Set up the range based on the commit relationship
	var revListOpts RevListOptions
	var reversed bool

	if !isShallow {
		// For full clones, we can reliably determine ancestry
		isAncestor, err := r.isAncestor(earliestCommit, latestCommit, timeout)
		if err != nil {
			// If we can't determine ancestry even in a full clone, try both directions
			// Default to assuming normal direction (earliest to latest)
			revListOpts = RevListOptions{
				Range: earliestCommit + ".." + latestCommit,
			}
			reversed = false
		} else if isAncestor {
			// Normal case: earliestCommit is an ancestor of latestCommit
			revListOpts = RevListOptions{
				Range: earliestCommit + ".." + latestCommit,
			}
			reversed = false
		} else {
			// Reversed case: latestCommit is an ancestor of earliestCommit
			revListOpts = RevListOptions{
				Range: latestCommit + ".." + earliestCommit,
			}
			reversed = true
		}
	} else {
		// For shallow clones, we can't reliably determine ancestry
		// Default to assuming normal direction (earliest to latest)
		// This assumption may not always be correct, but it's the best we can do
		revListOpts = RevListOptions{
			Range: earliestCommit + ".." + latestCommit,
		}
		reversed = false
	}

	// Use RevList with the range argument to get commits between the two points
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	revListDone := make(chan struct {
		commits []string
		err     error
	})

	go func() {
		// Use RevList with the options we've set up

		commits, err := r.RevList(revListOpts)
		revListDone <- struct {
			commits []string
			err     error
		}{commits, err}
	}()

	// Wait for rev-list completion or timeout
	var commits []string
	var revListErr error

	select {
	case result := <-revListDone:
		commits = result.commits
		revListErr = result.err
	case <-ctx.Done():
		cancel() // Cancel context before returning on timeout
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("rev-list operation timed out after %v", timeout)
		}
		return nil, ctx.Err() // Handle other context errors
	}

	cancel() // Cancel context for normal completion path

	if revListErr != nil {
		return nil, fmt.Errorf("error running rev-list: %w", revListErr)
	}

	// With the '..' notation, we need to make sure both endpoints are included
	// First, check if the endpoints are already in the list
	earliestFound := false
	latestFound := false
	for _, commit := range commits {
		if commit == earliestCommit {
			earliestFound = true
		}
		if commit == latestCommit {
			latestFound = true
		}
	}

	// In the reversed case, the contract of this function requires that
	// we need to maintain the earliest commit (which is HEAD) as first in the list,
	// and the latest commit (which is an ancestor) as last in the list
	if reversed {
		// First reverse the commits to get the original order
		for i, j := 0, len(commits)-1; i < j; i, j = i+1, j-1 {
			commits[i], commits[j] = commits[j], commits[i]
		}

		// For the reversed case, the order from git will be from oldest to newest
		// But we need earliest (HEAD) first and latest (ancestor) last
		// So we'll reverse it again to put the latest commit first, then
		// make sure endpoints are in the right place
		var resultCommits []string

		// Add earliest (HEAD) first
		resultCommits = append(resultCommits, earliestCommit)

		// Add any commits in between, excluding endpoints
		for _, commit := range commits {
			if commit != earliestCommit && commit != latestCommit {
				resultCommits = append(resultCommits, commit)
			}
		}

		// Add latest (ancestor) last
		resultCommits = append(resultCommits, latestCommit)

		// Replace our commit list
		commits = resultCommits
	} else {
		// Normal case: latest first, earliest last
		// Add endpoints if needed
		if !latestFound {
			commits = append([]string{latestCommit}, commits...)
		}
		if !earliestFound {
			commits = append(commits, earliestCommit)
		}
	}

	// Now commits are ordered with latest first and earliest last
	return commits, nil
}

// initialCommit returns the first commit in the repository (the root commit)
func (r *Repo) initialCommit() (string, error) {
	// Use git rev-list with --max-parents=0 to find the root commit(s)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "rev-list", "--max-parents=0", "HEAD")
	cmd.Dir = r.RepoPath

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("error finding initial commit: %w", err)
	}

	// Get the commit ID (trim newlines)
	commitID := strings.TrimSpace(string(output))
	if commitID == "" {
		return "", fmt.Errorf("no initial commit found")
	}

	return commitID, nil
}

// isAncestor checks if the potential ancestor commit is an ancestor of the descendant commit.
// Returns true if potentialAncestor is an ancestor of descendant, false otherwise.
func (r *Repo) isAncestor(potentialAncestor string, descendant string, timeout time.Duration) (bool, error) {
	if potentialAncestor == "" || descendant == "" {
		return false, fmt.Errorf("both commits must be specified")
	}

	// First check if both commits exist in the repository
	existsA, err := r.commitExists(potentialAncestor, timeout)
	if err != nil {
		return false, fmt.Errorf("error checking if ancestor commit exists: %w", err)
	}

	existsD, err := r.commitExists(descendant, timeout)
	if err != nil {
		return false, fmt.Errorf("error checking if descendant commit exists: %w", err)
	}

	// If either commit doesn't exist locally, we can't determine ancestry directly
	// So we'll assume they're not in an ancestral relationship
	if !existsA || !existsD {
		return false, nil
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Run git merge-base --is-ancestor to check if potentialAncestor is an ancestor of descendant
	cmd := exec.CommandContext(ctx, "git", "merge-base", "--is-ancestor", potentialAncestor, descendant)
	cmd.Dir = r.RepoPath

	// The command exits with status 0 if true, status 1 if false, and 128 if error
	err = cmd.Run()
	if err == nil {
		// Exit status 0 means potentialAncestor is an ancestor of descendant
		return true, nil
	}

	// Check if it's a normal exit status 1, which means "not an ancestor"
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		// Exit status 1 means potentialAncestor is NOT an ancestor of descendant
		return false, nil
	}

	// Any other error is an actual error
	return false, fmt.Errorf("error checking ancestor relationship: %w", err)
}

// commitExists checks if a commit exists in the repository.
func (r *Repo) commitExists(commitID string, timeout time.Duration) (bool, error) {
	if commitID == "" {
		return false, fmt.Errorf("empty commit ID")
	}

	// Use context with timeout for the operation
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Use a channel to make the operation cancellable with timeout
	done := make(chan struct {
		exists bool
		err    error
	})

	go func() {
		// Call the CatFile method with options
		options := CatFileOptions{
			Exists:   true,
			ObjectID: commitID,
		}
		exists, _, err := r.CatFile(options)
		done <- struct {
			exists bool
			err    error
		}{exists, err}
	}()

	// Wait for either completion or timeout
	select {
	case result := <-done:
		return result.exists, result.err

	case <-ctx.Done():
		// If the context timed out, return an error
		return false, fmt.Errorf("commit existence check timed out after %v", timeout)
	}
}

// buildCommitPath builds a path of commits from HEAD to the target commit.
// Returns a slice where the first element is HEAD and the last element is the target commit.
func (r *Repo) buildCommitPath(targetCommit string, timeout time.Duration) ([]string, error) {
	if targetCommit == "" {
		return nil, fmt.Errorf("empty target commit")
	}

	// Get the HEAD commit first with timeout
	headCtx, headCancel := context.WithTimeout(context.Background(), timeout)
	headDone := make(chan struct {
		commit string
		err    error
	})

	go func() {
		// Use the RevParse method
		commit, err := r.RevParse("HEAD")
		result := struct {
			commit string
			err    error
		}{}

		if err != nil {
			result.err = fmt.Errorf("error getting HEAD commit: %w", err)
		} else {
			result.commit = commit
		}
		headDone <- result
	}()

	// Wait for command completion or timeout
	var headCommit string
	var headErr error

	select {
	case result := <-headDone:
		// Command completed
		headCommit = result.commit
		headErr = result.err
	case <-headCtx.Done():
		headCancel()
		return nil, fmt.Errorf("HEAD commit lookup timed out after %v", timeout)
	}
	headCancel() // Always cancel the context

	if headErr != nil {
		return nil, headErr
	}

	// Now use the RevList method with timeout to get commits between HEAD and target
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	revListDone := make(chan struct {
		commits []string
		err     error
	})

	go func() {
		// Use the RevList method
		revListOpts := RevListOptions{
			Range:        targetCommit + "..HEAD",
			AncestryPath: true,
		}

		commits, err := r.RevList(revListOpts)
		revListDone <- struct {
			commits []string
			err     error
		}{commits, err}
	}()

	// Wait for rev-list completion or timeout
	var commits []string
	var revListErr error

	select {
	case result := <-revListDone:
		commits = result.commits
		revListErr = result.err
	case <-ctx.Done():
		cancel() // Cancel context before returning on timeout
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("rev-list operation timed out after %v", timeout)
		}
		return nil, ctx.Err() // Handle other context errors
	}

	cancel() // Cancel context for normal completion path

	if revListErr != nil {
		return nil, fmt.Errorf("error running rev-list: %w", revListErr)
	}

	// Build the path - first element is HEAD
	commitPath := []string{headCommit}

	// Add the intermediate commits if any
	if len(commits) > 0 {
		commitPath = append(commitPath, commits...)
	}

	// Add target commit as the last element if it's not already included
	if len(commitPath) == 0 || commitPath[len(commitPath)-1] != targetCommit {
		commitPath = append(commitPath, targetCommit)
	}

	return commitPath, nil
}

func (r *Repo) Log(options LogOptions) ([]LogItem, error) {
	args := []string{"log"}
	if options.Source {
		args = append(args, "--source")
	}
	if options.Oneline {
		args = append(args, "--oneline")
	}
	if options.Decorate {
		args = append(args, "--decorate")
	}
	if options.Tags {
		args = append(args, "--tags")
	}
	if options.Commit1 != "" {
		args = append(args, options.Commit1)
	}
	if options.Commit2 != "" {
		args = append(args, options.Commit2)
	}
	stdout, stderr, err := r.Client.Exec(args...)
	if err != nil {
		return nil, fmt.Errorf("git log failed: %w\nstdout: %s\nstderr: %s",
			err, stdout, stderr)
	}

	var logItems []LogItem

	if options.Oneline {
		logItems = parseOnelineFormat(string(stdout))
	} else {
		logItems = parseMultilineFormat(string(stdout))
	}

	return logItems, nil
}
