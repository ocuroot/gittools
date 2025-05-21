# Locks Directory

This directory contains lock files that represent active locks in the system.

## Lock File Format

Each lock file should follow the naming convention: `<resource_id>.lock`

The content of the lock file should be JSON formatted with the following structure:

```json
{
  "owner": "user_id",
  "created_at": "timestamp",
  "expires_at": "timestamp",
  "metadata": {
    "description": "optional description of what this lock is for",
    "additional_fields": "as needed"
  }
}
```

## Usage

When acquiring a lock:
1. Create a new branch from main
2. Create or update the lock file
3. Push and create a PR
4. If the PR has conflicts, someone else has the lock

When releasing a lock:
1. Delete the lock file
2. Push and merge the PR
