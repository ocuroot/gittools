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

### Repo

The `Repo` struct provides a high-level interface to Git operations:

The `Repo` struct provides functionality such as:

- Repository initialization and cloning
- Branch management
- Commit operations
- Pull and push operations
- Merge and rebase handling

See the [package documentation](https://pkg.go.dev/github.com/ocuroot/gittools) for usage examples.

### Locking

The `lock` package provides a distributed locking system using Git's conflict detection as a locking mechanism:

1. Locks are represented as files in a Git repository
2. Acquiring a lock creates or modifies a lock file
3. Git merge conflicts indicate lock contention
4. Locks can have expiration times and metadata
5. Lock history is preserved in Git commit history

## Documentation

For detailed usage examples, please refer to the [GoDoc documentation](https://pkg.go.dev/github.com/ocuroot/gittools). The package includes testable examples that demonstrate how to use the various components.

## Lock File Format

Lock files can be stored as any file in the repository. Each lock file contains JSON-formatted metadata including:

- Owner information (a ULID to identify a particular process)
- Creation timestamp
- Expiration timestamp
- Description metadata

## Limitations

- **Not yet comprehensive**: This won't provide access to all git features, but it's a start.
- **Performance**: This isn't designed for high-contention scenarios. If you need thousands of locks per second, you'll want a dedicated locking service.
- **Latency**: Lock acquisition depends on Git operations, which adds some overhead compared to in-memory locks.
- **Expiration Handling**: Lock expiration is tracked but not automatically enforced - you'll need to implement cleanup for stale locks.
