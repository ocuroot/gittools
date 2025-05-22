package lock

import "errors"

// ErrLockConflict is returned when a lock acquisition fails due to the resource being locked
var ErrLockConflict = errors.New("lock conflict: resource is already locked")
