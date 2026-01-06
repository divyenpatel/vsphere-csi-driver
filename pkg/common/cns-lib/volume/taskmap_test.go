package volume

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/vmware/govmomi/vim25/types"
)

// TestTaskMapCountRace demonstrates the race condition in Count() method
// Run with: go test -race -run TestTaskMapCountRace
// This test should FAIL with the current implementation (no lock in Count)
// and PASS after adding RLock to Count()
func TestTaskMapCountRace(t *testing.T) {
	taskMap := NewTaskMap()
	
	// Number of operations - increase this to make race more likely
	iterations := 10000
	
	var wg sync.WaitGroup
	wg.Add(3)
	
	// Goroutine 1: Continuously add tasks
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			ref := types.ManagedObjectReference{
				Type:  "Task",
				Value: fmt.Sprintf("task-add-%d", i),
			}
			taskMap.Upsert(ref, TaskDetails{
				Reference:        ref,
				MarkedForRemoval: false,
				ResultCh:         make(chan TaskResult, 1),
			})
		}
	}()
	
	// Goroutine 2: Continuously delete tasks
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			ref := types.ManagedObjectReference{
				Type:  "Task",
				Value: fmt.Sprintf("task-delete-%d", i),
			}
			// Add first
			taskMap.Upsert(ref, TaskDetails{
				Reference:        ref,
				MarkedForRemoval: false,
				ResultCh:         make(chan TaskResult, 1),
			})
			// Then delete
			taskMap.Delete(ref)
		}
	}()
	
	// Goroutine 3: Continuously call Count() - THIS IS WHERE THE RACE HAPPENS
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = taskMap.Count() // Race condition here!
		}
	}()
	
	wg.Wait()
	
	t.Logf("Test completed successfully. Final count: %d", taskMap.Count())
}

// TestTaskMapCountRaceAggressive is a more aggressive version that's more likely to trigger the panic
// Run with: go test -race -run TestTaskMapCountRaceAggressive
func TestTaskMapCountRaceAggressive(t *testing.T) {
	taskMap := NewTaskMap()
	
	// Run for a short duration with maximum concurrency
	duration := 100 * time.Millisecond
	stopCh := make(chan struct{})
	
	var wg sync.WaitGroup
	
	// Start multiple writer goroutines
	numWriters := 10
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
					ref := types.ManagedObjectReference{
						Type:  "Task",
						Value: fmt.Sprintf("task-w%d-%d", workerID, i),
					}
					taskMap.Upsert(ref, TaskDetails{
						Reference:        ref,
						MarkedForRemoval: false,
						ResultCh:         make(chan TaskResult, 1),
					})
					taskMap.Delete(ref)
					i++
				}
			}
		}(w)
	}
	
	// Start multiple reader goroutines calling Count()
	numReaders := 10
	for r := 0; r < numReaders; r++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				select {
				case <-stopCh:
					return
				default:
					_ = taskMap.Count() // Race condition here!
				}
			}
		}(r)
	}
	
	// Let it run for the specified duration
	time.Sleep(duration)
	close(stopCh)
	wg.Wait()
	
	t.Logf("Aggressive test completed. Final count: %d", taskMap.Count())
}

// TestTaskMapCountWithGetAll simulates the actual usage pattern in RemoveTasksMarkedForDeletion
// This mimics the real-world scenario where Count() is called before and after GetAll() and Delete()
func TestTaskMapCountWithGetAll(t *testing.T) {
	taskMap := NewTaskMap()
	
	// Pre-populate with some tasks
	for i := 0; i < 100; i++ {
		ref := types.ManagedObjectReference{
			Type:  "Task",
			Value: fmt.Sprintf("initial-task-%d", i),
		}
		taskMap.Upsert(ref, TaskDetails{
			Reference:        ref,
			MarkedForRemoval: i%2 == 0, // Mark half for removal
			ResultCh:         make(chan TaskResult, 1),
		})
	}
	
	var wg sync.WaitGroup
	wg.Add(2)
	
	// Goroutine 1: Simulates RemoveTasksMarkedForDeletion pattern
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			// This is the pattern from listview.go:472-487
			countBefore := taskMap.Count() // Race here!
			
			allTasks := taskMap.GetAll()
			for _, task := range allTasks {
				if task.MarkedForRemoval {
					taskMap.Delete(task.Reference)
				}
			}
			
			countAfter := taskMap.Count() // Race here!
			
			_ = countBefore
			_ = countAfter
		}
	}()
	
	// Goroutine 2: Continuously add/remove tasks (simulates concurrent CSI operations)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			ref := types.ManagedObjectReference{
				Type:  "Task",
				Value: fmt.Sprintf("concurrent-task-%d", i),
			}
			taskMap.Upsert(ref, TaskDetails{
				Reference:        ref,
				MarkedForRemoval: false,
				ResultCh:         make(chan TaskResult, 1),
			})
			taskMap.Delete(ref)
		}
	}()
	
	wg.Wait()
	
	t.Logf("RemoveTasksMarkedForDeletion pattern test completed. Final count: %d", taskMap.Count())
}

// TestTaskMapBasicOperations tests basic functionality (should always pass)
func TestTaskMapBasicOperations(t *testing.T) {
	taskMap := NewTaskMap()
	
	// Test empty map
	if count := taskMap.Count(); count != 0 {
		t.Errorf("Expected count 0, got %d", count)
	}
	
	// Test Upsert
	ref1 := types.ManagedObjectReference{Type: "Task", Value: "task-1"}
	taskMap.Upsert(ref1, TaskDetails{
		Reference:        ref1,
		MarkedForRemoval: false,
		ResultCh:         make(chan TaskResult, 1),
	})
	
	if count := taskMap.Count(); count != 1 {
		t.Errorf("Expected count 1, got %d", count)
	}
	
	// Test Get
	details, ok := taskMap.Get(ref1)
	if !ok {
		t.Error("Expected to find task-1")
	}
	if details.Reference.Value != "task-1" {
		t.Errorf("Expected task-1, got %s", details.Reference.Value)
	}
	
	// Test Upsert multiple
	ref2 := types.ManagedObjectReference{Type: "Task", Value: "task-2"}
	taskMap.Upsert(ref2, TaskDetails{
		Reference:        ref2,
		MarkedForRemoval: false,
		ResultCh:         make(chan TaskResult, 1),
	})
	
	if count := taskMap.Count(); count != 2 {
		t.Errorf("Expected count 2, got %d", count)
	}
	
	// Test GetAll
	allTasks := taskMap.GetAll()
	if len(allTasks) != 2 {
		t.Errorf("Expected 2 tasks, got %d", len(allTasks))
	}
	
	// Test Delete
	taskMap.Delete(ref1)
	if count := taskMap.Count(); count != 1 {
		t.Errorf("Expected count 1 after delete, got %d", count)
	}
	
	// Test Get after delete
	_, ok = taskMap.Get(ref1)
	if ok {
		t.Error("Expected task-1 to be deleted")
	}
	
	// Clean up
	taskMap.Delete(ref2)
	if count := taskMap.Count(); count != 0 {
		t.Errorf("Expected count 0 after deleting all, got %d", count)
	}
}

// TestTaskMapConcurrentReads tests that multiple concurrent reads are safe (should always pass)
func TestTaskMapConcurrentReads(t *testing.T) {
	taskMap := NewTaskMap()
	
	// Pre-populate
	for i := 0; i < 100; i++ {
		ref := types.ManagedObjectReference{
			Type:  "Task",
			Value: fmt.Sprintf("task-%d", i),
		}
		taskMap.Upsert(ref, TaskDetails{
			Reference:        ref,
			MarkedForRemoval: false,
			ResultCh:         make(chan TaskResult, 1),
		})
	}
	
	var wg sync.WaitGroup
	numReaders := 20
	
	// Multiple goroutines reading concurrently (this should be safe with RLock)
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				_ = taskMap.Count()
				_ = taskMap.GetAll()
				ref := types.ManagedObjectReference{
					Type:  "Task",
					Value: fmt.Sprintf("task-%d", j%100),
				}
				_, _ = taskMap.Get(ref)
			}
		}()
	}
	
	wg.Wait()
	
	if count := taskMap.Count(); count != 100 {
		t.Errorf("Expected count 100, got %d", count)
	}
}

// BenchmarkTaskMapCount benchmarks the Count() method
func BenchmarkTaskMapCount(b *testing.B) {
	taskMap := NewTaskMap()
	
	// Pre-populate with some tasks
	for i := 0; i < 1000; i++ {
		ref := types.ManagedObjectReference{
			Type:  "Task",
			Value: fmt.Sprintf("task-%d", i),
		}
		taskMap.Upsert(ref, TaskDetails{
			Reference:        ref,
			MarkedForRemoval: false,
			ResultCh:         make(chan TaskResult, 1),
		})
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = taskMap.Count()
	}
}

// BenchmarkTaskMapConcurrentCountAndWrites benchmarks concurrent Count() calls with writes
func BenchmarkTaskMapConcurrentCountAndWrites(b *testing.B) {
	taskMap := NewTaskMap()
	
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				// Write operation
				ref := types.ManagedObjectReference{
					Type:  "Task",
					Value: fmt.Sprintf("task-%d", i),
				}
				taskMap.Upsert(ref, TaskDetails{
					Reference:        ref,
					MarkedForRemoval: false,
					ResultCh:         make(chan TaskResult, 1),
				})
			} else {
				// Read operation
				_ = taskMap.Count()
			}
			i++
		}
	})
}

