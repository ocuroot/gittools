package gittools

import (
	"bytes"
	"fmt"
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

	_, _, err := c2.Exec("init", "--bare", "--initial-branch="+defaultBranch, destination)
	if err != nil {
		return nil, fmt.Errorf("git init failed: %w", err)
	}

	return &Repo{
		Client:   &c2,
		RepoPath: destination,
	}, nil
}

func (c *Client) Clone(url, destination string) (*Repo, error) {
	absPath, err := filepath.Abs(destination)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Execute git clone
	output, _, err := c.Exec("clone", url, absPath)
	if err != nil {
		return nil, fmt.Errorf("git clone failed: %s: %w", output, err)
	}

	c2 := *c
	c2.WorkDir = absPath

	// After cloning, explicitly check out the main branch
	output, _, err = c2.Exec("checkout", "main")
	if err != nil {
		return nil, fmt.Errorf("failed to checkout main branch: %s: %w", output, err)
	}

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

	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}
