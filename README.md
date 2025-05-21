# Git Lock Experiment

An experiment for managing distributed locks using Git repositories and merge conflicts to detect collision.

## Concept

This experiment explores the idea of using Git's merge conflict detection as a distributed locking mechanism. The core concept is:

1. Locks are represented as files in a Git repository
2. Attempting to acquire a lock means creating or modifying a lock file in a branch and opening a PR
3. If the PR has conflicts, someone else has the lock (conflict detected)
4. If the PR has no conflicts, you've successfully acquired the lock
5. Releasing a lock means deleting the lock file and merging the PR

## Use Cases

- Distributed resource locking without a dedicated locking service
- Coordination between CI/CD systems
- Simple mutex implementation for shared resources
- Transparent lock history via Git commit history

## How to Use

The repository includes three utility scripts:

### Acquiring a Lock

```bash
./lock.sh <resource_id> [description]
```

This script:
- Creates a new branch for the lock operation
- Creates or updates a lock file for the resource
- Pushes the branch and outputs instructions to create a PR

### Releasing a Lock

```bash
./unlock.sh <resource_id>
```

This script:
- Creates a branch for releasing the lock
- Removes the lock file
- Pushes the branch and outputs instructions to create a PR

### Checking Lock Status

```bash
./check-locks.sh
```

This script outputs information about all current locks and pending lock operations.

## Lock File Format

Lock files are stored in the `locks/` directory with the naming convention `<resource_id>.lock`. Each lock file contains JSON-formatted metadata including:

- Owner information
- Creation timestamp
- Expiration timestamp
- Custom metadata

See the README in the `locks/` directory for more details.

## Limitations

- This approach works best for low-contention resources
- Lock acquisition is not immediate (depends on PR creation time)
- Requires manual PR creation and merging (could be automated with GitHub Actions)
- No automatic expiration (though the locks include expiry timestamps)

## Future Improvements

- Add GitHub Actions to automate PR creation and merging
- Implement automatic lock expiration
- Add validation for lock file format
- Create a simple CLI tool to manage locks programmatically
