package tools

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestWithFileMutationQueue_SerializesSameFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.txt")
	var mu sync.Mutex
	active := 0
	maxActive := 0
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res := withFileMutationQueue(path, func() Result {
				mu.Lock()
				active++
				if active > maxActive {
					maxActive = active
				}
				mu.Unlock()

				time.Sleep(5 * time.Millisecond)

				mu.Lock()
				active--
				mu.Unlock()
				return Result{OK: true, Output: fmt.Sprintf("%d", i)}
			})
			if !res.OK {
				t.Errorf("unexpected error: %s", res.Error)
			}
		}(i)
	}
	wg.Wait()
	if maxActive != 1 {
		t.Fatalf("same-file mutations should serialize, maxActive=%d", maxActive)
	}
}
