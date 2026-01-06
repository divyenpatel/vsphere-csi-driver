// +build ignore

// Race Condition Demonstration
// This is a standalone program that demonstrates the race condition in TaskMap.Count()
//
// To run and see the race:
//   go run -race race_demo.go
//
// Expected output with race detector:
//   WARNING: DATA RACE
//   or
//   fatal error: concurrent map read and map write

package main

import (
	"fmt"
	"sync"
	"time"
)

// Simplified TaskMap without lock in Count() - BUGGY VERSION
type TaskMapBuggy struct {
	mu sync.RWMutex
	m  map[string]int
}

func NewTaskMapBuggy() *TaskMapBuggy {
	return &TaskMapBuggy{
		m: make(map[string]int),
	}
}

func (t *TaskMapBuggy) Upsert(key string, value int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.m[key] = value
}

func (t *TaskMapBuggy) Delete(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.m, key)
}

// BUG: No lock protection!
func (t *TaskMapBuggy) Count() int {
	return len(t.m) // RACE CONDITION HERE
}

// Fixed TaskMap with lock in Count() - CORRECT VERSION
type TaskMapFixed struct {
	mu sync.RWMutex
	m  map[string]int
}

func NewTaskMapFixed() *TaskMapFixed {
	return &TaskMapFixed{
		m: make(map[string]int),
	}
}

func (t *TaskMapFixed) Upsert(key string, value int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.m[key] = value
}

func (t *TaskMapFixed) Delete(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.m, key)
}

// FIXED: With lock protection
func (t *TaskMapFixed) Count() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.m)
}

func main() {
	fmt.Println("=== Race Condition Demonstration ===")
	fmt.Println()
	
	// Test buggy version
	fmt.Println("Testing BUGGY version (without lock in Count)...")
	fmt.Println("Run with: go run -race race_demo.go")
	fmt.Println()
	
	testBuggyVersion()
	
	fmt.Println()
	fmt.Println("Testing FIXED version (with lock in Count)...")
	fmt.Println()
	
	testFixedVersion()
	
	fmt.Println()
	fmt.Println("=== Test Complete ===")
	fmt.Println("If you ran with -race flag and saw 'WARNING: DATA RACE' above,")
	fmt.Println("that demonstrates the bug in the Count() method.")
}

func testBuggyVersion() {
	taskMap := NewTaskMapBuggy()
	
	duration := 50 * time.Millisecond
	stopCh := make(chan struct{})
	
	var wg sync.WaitGroup
	
	// Writer goroutines
	for w := 0; w < 5; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			i := 0
			for {
				select {
				case <-stopCh:
					return
				default:
					key := fmt.Sprintf("task-w%d-%d", workerID, i)
					taskMap.Upsert(key, i)
					taskMap.Delete(key)
					i++
				}
			}
		}(w)
	}
	
	// Reader goroutines calling Count() - RACE HAPPENS HERE
	for r := 0; r < 5; r++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				select {
				case <-stopCh:
					return
				default:
					_ = taskMap.Count() // RACE!
				}
			}
		}(r)
	}
	
	time.Sleep(duration)
	close(stopCh)
	wg.Wait()
	
	fmt.Printf("Buggy version completed. Final count: %d\n", taskMap.Count())
}

func testFixedVersion() {
	taskMap := NewTaskMapFixed()
	
	duration := 50 * time.Millisecond
	stopCh := make(chan struct{})
	
	var wg sync.WaitGroup
	
	// Writer goroutines
	for w := 0; w < 5; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			i := 0
			for {
				select {
				case <-stopCh:
					return
				default:
					key := fmt.Sprintf("task-w%d-%d", workerID, i)
					taskMap.Upsert(key, i)
					taskMap.Delete(key)
					i++
				}
			}
		}(w)
	}
	
	// Reader goroutines calling Count() - NO RACE with lock
	for r := 0; r < 5; r++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				select {
				case <-stopCh:
					return
				default:
					_ = taskMap.Count() // Safe with lock
				}
			}
		}(r)
	}
	
	time.Sleep(duration)
	close(stopCh)
	wg.Wait()
	
	fmt.Printf("Fixed version completed. Final count: %d\n", taskMap.Count())
}

