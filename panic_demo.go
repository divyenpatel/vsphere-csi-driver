// +build ignore

// Panic Demonstration for TaskMap.Count() race condition
// This program tries to trigger an actual panic (not just race detection)
// by creating maximum contention on the map.
//
// To run:
//   go run panic_demo.go
//
// Note: Panics are timing-dependent and may not occur every time.
// The race detector (go run -race) is more reliable for detecting the bug.
//
// Expected output (when panic occurs):
//   fatal error: concurrent map read and map write

package main

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

type TaskMap struct {
	mu sync.RWMutex
	m  map[string]int
}

func NewTaskMap() *TaskMap {
	return &TaskMap{
		m: make(map[string]int),
	}
}

func (t *TaskMap) Upsert(key string, value int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.m[key] = value
}

func (t *TaskMap) Delete(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.m, key)
}

// BUG: No lock protection - this can panic!
func (t *TaskMap) CountBuggy() int {
	return len(t.m)
}

func main() {
	fmt.Println("=== Attempting to Trigger Panic ===")
	fmt.Println("This demonstrates the 'concurrent map read and map write' panic")
	fmt.Println()
	fmt.Printf("Running on %d CPU cores\n", runtime.NumCPU())
	fmt.Println("Creating maximum contention on the map...")
	fmt.Println()
	
	// Use all available CPUs to maximize chance of panic
	runtime.GOMAXPROCS(runtime.NumCPU())
	
	taskMap := NewTaskMap()
	
	// Run for longer to increase chance of panic
	duration := 2 * time.Second
	stopCh := make(chan struct{})
	
	var wg sync.WaitGroup
	
	// Create many writer goroutines for maximum contention
	numWriters := runtime.NumCPU() * 4
	for w := 0; w < numWriters; w++ {
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
					// No sleep - tight loop for maximum contention
					taskMap.Delete(key)
					i++
				}
			}
		}(w)
	}
	
	// Create many reader goroutines calling Count()
	numReaders := runtime.NumCPU() * 4
	for r := 0; r < numReaders; r++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				select {
				case <-stopCh:
					return
				default:
					// Tight loop - no sleep
					_ = taskMap.CountBuggy() // THIS CAN PANIC!
				}
			}
		}(r)
	}
	
	// Progress indicator
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				fmt.Print(".")
			}
		}
	}()
	
	fmt.Printf("Running %d writers and %d readers for %v...\n", 
		numWriters, numReaders, duration)
	
	time.Sleep(duration)
	close(stopCh)
	wg.Wait()
	
	fmt.Println()
	fmt.Println()
	fmt.Println("=== Test Completed Without Panic ===")
	fmt.Println()
	fmt.Println("Note: The panic is timing-dependent and may not occur every time.")
	fmt.Println("However, the race condition still exists and can be detected with:")
	fmt.Println("  go run -race panic_demo.go")
	fmt.Println()
	fmt.Println("In production, this could manifest as:")
	fmt.Println("  - Occasional panics under high load")
	fmt.Println("  - More frequent panics with many concurrent operations")
	fmt.Println("  - Undefined behavior even without panic")
	fmt.Printf("\nFinal count: %d\n", taskMap.CountBuggy())
}

