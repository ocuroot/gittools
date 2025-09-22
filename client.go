package gittools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func NewClient() *Client {
	return &Client{}
}

type Client struct {
	Binary  string // Path to the git binary. If empty, "git" is used.
	WorkDir string // Directory to run git commands in. If empty, the current working directory is used.

	AuthorEmail    string
	AuthorName     string
	CommitterEmail string
	CommitterName  string
}

// SetUser is equivalent to running `git config --global user.email <email>`
// and `git config --global user.name <name>`
// It applies only to calls made via this Client and does not persist.
func (c *Client) SetUser(name, email string) {
	c.AuthorName = name
	c.AuthorEmail = email
	c.CommitterName = name
	c.CommitterEmail = email
}

func (c *Client) gitPath() string {
	if c.Binary != "" {
		return c.Binary
	}
	return "git"
}

func (c *Client) Init(destination string, defaultBranch string) (*Repo, error) {
	c2 := *c
	c2.WorkDir = destination

	_, _, err := c2.Exec("init", "--initial-branch="+defaultBranch, destination)
	if err != nil {
		return nil, fmt.Errorf("git init failed: %w", err)
	}

	return &Repo{
		Client:   &c2,
		RepoPath: destination,
	}, nil
}

func (c *Client) InitBare(destination, defaultBranch string) (*Repo, error) {
	c2 := *c
	c2.WorkDir = destination

	stdoutContent, stdErrContent, err := c2.Exec("init", "--bare", "--initial-branch="+defaultBranch, destination)
	if err != nil {
		return nil, fmt.Errorf("git init failed: %w\nstdout: %s\nstderr: %s", err, stdoutContent, stdErrContent)
	}

	return &Repo{
		Client:   &c2,
		RepoPath: destination,
	}, nil
}

// CloneOptions defines options for git clone operations
type CloneOptions struct {
	// URL of the repository to clone
	URL string

	// Destination path where the repository will be cloned
	Destination string

	// Depth for shallow clone (0 = full clone)
	Depth int

	// Branch to checkout after clone (empty = use default branch)
	Branch string

	// Context for the operation with optional timeout
	Context context.Context
}

// CloneWithOptions clones a git repository with the specified options
func (c *Client) CloneWithOptions(options CloneOptions) (*Repo, error) {
	absPath, err := filepath.Abs(options.Destination)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Build command arguments
	args := []string{"clone"}

	// Add depth parameter for shallow clone
	if options.Depth > 0 {
		args = append(args, fmt.Sprintf("--depth=%d", options.Depth))
	}

	// Add branch parameter if specified
	if options.Branch != "" {
		args = append(args, "-b", options.Branch)
	}

	// Add source and destination
	args = append(args, options.URL, absPath)

	// Use context if provided
	if options.Context != nil {
		// Create a command with context
		cmd := exec.CommandContext(options.Context, c.gitPath())
		cmd.Args = append([]string{c.gitPath()}, args...)
		if c.WorkDir != "" {
			cmd.Dir = c.WorkDir
		}

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		if err != nil {
			return nil, fmt.Errorf("git clone failed: %s: %w", stderr.String(), err)
		}
	} else {
		// If no context provided, use regular Exec method
		_, stderr, err := c.Exec(args...)
		if err != nil {
			return nil, fmt.Errorf("git clone failed: %s: %w", stderr, err)
		}
	}

	// Create a new client with the cloned repo directory
	c2 := *c
	c2.WorkDir = absPath

	// Return a new repo with the cloned directory
	return &Repo{Client: &c2}, nil
}

func (c *Client) Clone(url, destination string) (*Repo, error) {
	absPath, err := filepath.Abs(destination)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Execute git clone
	stdout, stderr, err := c.Exec("clone", url, absPath)
	if err != nil {
		return nil, fmt.Errorf("git clone failed: %s, %s: %w", stdout, stderr, err)
	}

	c2 := *c
	c2.WorkDir = absPath

	// Create and return the repo - let Git handle the default branch
	// The default branch will already be checked out by 'git clone'
	return &Repo{
		Client:   &c2,
		RepoPath: absPath,
	}, nil
}

func (c *Client) GetHash(path string) (string, error) {
	stdout, stderr, err := c.Exec("hash-object", path)
	if err != nil {
		return "", fmt.Errorf("git hash-object failed: %w\nstdout: %s\nstderr: %s",
			err, stdout, stderr)
	}
	return strings.Trim(string(stdout), "\n"), nil
}

func (c *Client) Exec(args ...string) ([]byte, []byte, error) {
	cmd := exec.Command(c.gitPath(), args...)
	if c.WorkDir != "" {
		cmd.Dir = c.WorkDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	cmd.Env = os.Environ()
	if c.AuthorName != "" {
		cmd.Env = append(cmd.Env, "GIT_AUTHOR_NAME="+c.AuthorName)
	}
	if c.AuthorEmail != "" {
		cmd.Env = append(cmd.Env, "GIT_AUTHOR_EMAIL="+c.AuthorEmail)
	}
	if c.CommitterName != "" {
		cmd.Env = append(cmd.Env, "GIT_COMMITTER_NAME="+c.CommitterName)
	}
	if c.CommitterEmail != "" {
		cmd.Env = append(cmd.Env, "GIT_COMMITTER_EMAIL="+c.CommitterEmail)
	}

	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}
