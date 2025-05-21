package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ocuroot/git-lock-experiment/gitlock"
)

func main() {
	repoPath := flag.String("repo", "", "Path to the git repository")
	lockPath := flag.String("lock", "locks/resource.lock", "Path to the lock file within the repository")
	action := flag.String("action", "acquire", "Action to perform: acquire, release, refresh, check")
	timeout := flag.Duration("timeout", 30*time.Second, "Timeout for acquiring a lock")
	expiry := flag.Duration("expiry", 5*time.Minute, "Expiry duration for the lock")
	description := flag.String("desc", "", "Description of the lock")

	flag.Parse()

	if *repoPath == "" {
		fmt.Println("Error: repository path is required")
		flag.Usage()
		os.Exit(1)
	}

	// Create a GitRepo instance
	repo, err := gitlock.New(*repoPath)
	if err != nil {
		log.Fatalf("Failed to create GitRepo: %v", err)
	}

	fmt.Printf("Repository: %s, Lock Key: %s\n", repo.RepoPath, repo.LockKey)

	switch *action {
	case "acquire":
		fmt.Printf("Attempting to acquire lock: %s (timeout: %s, expiry: %s)\n", *lockPath, *timeout, *expiry)
		err = repo.AcquireLock(*lockPath, *timeout, *expiry, *description)
		if err != nil {
			log.Fatalf("Failed to acquire lock: %v", err)
		}
		fmt.Println("Lock acquired successfully!")

	case "release":
		fmt.Printf("Releasing lock: %s\n", *lockPath)
		err = repo.ReleaseLock(*lockPath)
		if err != nil {
			log.Fatalf("Failed to release lock: %v", err)
		}
		fmt.Println("Lock released successfully!")

	case "refresh":
		fmt.Printf("Refreshing lock: %s (new expiry: %s)\n", *lockPath, *expiry)
		err = repo.RefreshLock(*lockPath, *expiry)
		if err != nil {
			log.Fatalf("Failed to refresh lock: %v", err)
		}
		fmt.Println("Lock refreshed successfully!")

	case "check":
		fmt.Printf("Checking lock status: %s\n", *lockPath)
		isOwner, lock, err := repo.IsLocked(*lockPath)
		if err != nil {
			log.Fatalf("Failed to check lock status: %v", err)
		}

		if lock == nil {
			fmt.Println("Resource is not locked")
		} else {
			if isOwner {
				fmt.Println("Lock is held by this process")
			} else {
				fmt.Println("Lock is held by another process")
			}
			fmt.Printf("  Owner: %s\n", lock.Owner)
			fmt.Printf("  Created: %s\n", lock.CreatedAt)
			fmt.Printf("  Expires: %s\n", lock.ExpiresAt)
			if lock.Description != "" {
				fmt.Printf("  Description: %s\n", lock.Description)
			}
		}

	default:
		fmt.Printf("Unknown action: %s\n", *action)
		flag.Usage()
		os.Exit(1)
	}
}
