package tools

import (
	"os"
	"path/filepath"
	"sync"
)

var fileMutationQueues = struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}{locks: map[string]*sync.Mutex{}}

func fileMutationKey(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = filepath.Clean(path)
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real
	}
	// Missing files can't be realpathed yet; use the
	// cleaned absolute target path like Pi does.
	return abs
}

// withFileMutationQueue serializes mutations targeting
// the same file while allowing different files to mutate
// concurrently. Ported from Pi's file-mutation-queue
// behavior to prevent concurrent write/edit calls from
// clobbering each other.
func withFileMutationQueue(path string, fn func() Result) Result {
	key := fileMutationKey(path)
	fileMutationQueues.mu.Lock()
	lock := fileMutationQueues.locks[key]
	if lock == nil {
		lock = &sync.Mutex{}
		fileMutationQueues.locks[key] = lock
	}
	fileMutationQueues.mu.Unlock()

	lock.Lock()
	defer func() {
		lock.Unlock()
		fileMutationQueues.mu.Lock()
		// Remove idle lock if no goroutine acquired it
		// between unlock and global-lock acquisition.
		// TryLock is available on sync.Mutex in modern Go.
		if lock.TryLock() {
			lock.Unlock()
			delete(fileMutationQueues.locks, key)
		}
		fileMutationQueues.mu.Unlock()
	}()
	return fn()
}

func mkdirAllForFile(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}
