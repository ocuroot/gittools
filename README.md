# gittools

[![GoDoc](https://pkg.go.dev/badge/github.com/ocuroot/gittools)](https://pkg.go.dev/github.com/ocuroot/gittools)

A Go module providing a wrapper around the local git command with a few additional utilities.

This module is primarily intended to be used in the Ocuroot project, so the future direction will be heavily dependant on the needs of that project.
Open sourcing in case it's useful to others as a library or examples.

## Installation

```bash
go get github.com/ocuroot/gittools
```

## Core Components

### GitRepo

The `GitRepo` struct provides a high-level interface to Git operations:

```go
// Initialize a new repository
repo, err := gittools.Init("/path/to/repo", gittools.GitInitOptions{
    DefaultBranch: "main",
})

// Or open an existing repository
repo, err := gittools.New("/path/to/existing/repo")

// Basic Git operations
err = repo.Add("file.txt")
err = repo.SimpleCommit("Add new file")
err = repo.Push("origin", "main")
```

### RepoLocks

The distributed locking system uses Git's conflict detection as a locking mechanism:

1. Locks are represented as files in a Git repository
2. Acquiring a lock creates or modifies a lock file
3. Git merge conflicts indicate lock contention
4. Locks can have expiration times and metadata
5. Lock history is preserved in Git commit history

## Usage Examples

### Managing Git Repositories

```go
package main

import (
    "fmt"
    "github.com/ocuroot/gittools"
)

func main() {
    // Clone a repository
    repo, err := gittools.Clone("https://github.com/example/repo.git", "./local-repo")
    if err != nil {
        panic(err)
    }
    
    // Create a new branch
    err = repo.CreateBranch("feature-branch")
    if err != nil {
        panic(err)
    }
    
    // Checkout the branch
    err = repo.Checkout("feature-branch")
    if err != nil {
        panic(err)
    }
    
    // Make changes and commit
    err = repo.SimpleCommit("Update documentation")
    if err != nil {
        panic(err)
    }
}
```

### Distributed Locking

```go
package main

import (
    "fmt"
    "time"
    "github.com/ocuroot/gittools"
)

func main() {
    // Open existing repository
    repo, err := gittools.New("./shared-repo")
    if err != nil {
        panic(err)
    }
    
    // Create a locks manager
    locks := gittools.NewRepoLocks(repo)
    
    // Try to acquire a lock with 10-minute expiration
    lockPath := "resources/database-migration.lock"
    err = locks.AcquireLock(lockPath, 10*time.Minute, "Running schema migration")
    if err != nil {
        fmt.Println("Resource is locked by another process")
        return
    }
    
    // Do work while holding the lock
    fmt.Println("Lock acquired, performing work...")
    
    // When done, release the lock
    err = locks.ReleaseLock(lockPath)
    if err != nil {
        panic(err)
    }
    fmt.Println("Lock released")
}
```

## Lock File Format

Lock files can be stored as any file in the repository. Each lock file contains JSON-formatted metadata including:

- Owner information (a ULID to identify a particular process)
- Creation timestamp
- Expiration timestamp
- Custom metadata

## Limitations

- **Not yet comprehensive**: This won't provide access to all git features, but it's a start.
- **Performance**: This isn't designed for high-contention scenarios. If you need thousands of locks per second, you'll want a dedicated locking service.
- **Latency**: Lock acquisition depends on Git operations, which adds some overhead compared to in-memory locks.
- **Expiration Handling**: Lock expiration is tracked but not automatically enforced - you'll need to implement cleanup for stale locks.
