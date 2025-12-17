/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package volume

import (
	"sync"
	"testing"

	"github.com/vmware/govmomi/vim25/types"
)

func TestNewTaskMap(t *testing.T) {
	tm := NewTaskMap()
	if tm == nil {
		t.Fatal("NewTaskMap() returned nil")
	}

	// Verify the map is empty
	if tm.Count() != 0 {
		t.Errorf("NewTaskMap().Count() = %d, expected 0", tm.Count())
	}
}

func TestTaskMap_Upsert(t *testing.T) {
	tm := NewTaskMap()

	// Create a task reference
	taskRef := types.ManagedObjectReference{
		Type:  "Task",
		Value: "task-1",
	}

	taskDetails := TaskDetails{
		Reference:        taskRef,
		MarkedForRemoval: false,
	}

	// Insert a new task
	tm.Upsert(taskRef, taskDetails)

	// Verify the task was inserted
	if tm.Count() != 1 {
		t.Errorf("Count after Upsert = %d, expected 1", tm.Count())
	}

	// Retrieve and verify the task
	retrieved, ok := tm.Get(taskRef)
	if !ok {
		t.Fatal("Get() returned false for inserted task")
	}
	if retrieved.Reference != taskDetails.Reference {
		t.Errorf("Retrieved task Reference = %v, expected %v", retrieved.Reference, taskDetails.Reference)
	}
	if retrieved.MarkedForRemoval != taskDetails.MarkedForRemoval {
		t.Errorf("Retrieved task MarkedForRemoval = %v, expected %v", retrieved.MarkedForRemoval, taskDetails.MarkedForRemoval)
	}
}

func TestTaskMap_UpsertUpdate(t *testing.T) {
	tm := NewTaskMap()

	taskRef := types.ManagedObjectReference{
		Type:  "Task",
		Value: "task-1",
	}

	// Insert initial task
	initialDetails := TaskDetails{
		Reference:        taskRef,
		MarkedForRemoval: false,
	}
	tm.Upsert(taskRef, initialDetails)

	// Update the task
	updatedDetails := TaskDetails{
		Reference:        taskRef,
		MarkedForRemoval: true,
	}
	tm.Upsert(taskRef, updatedDetails)

	// Verify count is still 1
	if tm.Count() != 1 {
		t.Errorf("Count after update = %d, expected 1", tm.Count())
	}

	// Verify the task was updated
	retrieved, ok := tm.Get(taskRef)
	if !ok {
		t.Fatal("Get() returned false for updated task")
	}
	if retrieved.MarkedForRemoval != updatedDetails.MarkedForRemoval {
		t.Errorf("Retrieved task MarkedForRemoval = %v, expected %v", retrieved.MarkedForRemoval, updatedDetails.MarkedForRemoval)
	}
}

func TestTaskMap_Delete(t *testing.T) {
	tm := NewTaskMap()

	taskRef := types.ManagedObjectReference{
		Type:  "Task",
		Value: "task-1",
	}

	taskDetails := TaskDetails{
		Reference:        taskRef,
		MarkedForRemoval: false,
	}

	// Insert and then delete
	tm.Upsert(taskRef, taskDetails)
	tm.Delete(taskRef)

	// Verify the task was deleted
	if tm.Count() != 0 {
		t.Errorf("Count after Delete = %d, expected 0", tm.Count())
	}

	// Verify Get returns false
	_, ok := tm.Get(taskRef)
	if ok {
		t.Error("Get() returned true for deleted task")
	}
}

func TestTaskMap_DeleteNonExistent(t *testing.T) {
	tm := NewTaskMap()

	taskRef := types.ManagedObjectReference{
		Type:  "Task",
		Value: "non-existent",
	}

	// Delete a non-existent task should not panic
	tm.Delete(taskRef)

	if tm.Count() != 0 {
		t.Errorf("Count after deleting non-existent = %d, expected 0", tm.Count())
	}
}

func TestTaskMap_Get(t *testing.T) {
	tm := NewTaskMap()

	taskRef := types.ManagedObjectReference{
		Type:  "Task",
		Value: "task-1",
	}

	// Get non-existent task
	_, ok := tm.Get(taskRef)
	if ok {
		t.Error("Get() returned true for non-existent task")
	}

	// Insert and get
	taskDetails := TaskDetails{
		Reference:        taskRef,
		MarkedForRemoval: false,
	}
	tm.Upsert(taskRef, taskDetails)

	retrieved, ok := tm.Get(taskRef)
	if !ok {
		t.Fatal("Get() returned false for existing task")
	}
	if retrieved.Reference != taskDetails.Reference {
		t.Errorf("Retrieved task Reference = %v, expected %v", retrieved.Reference, taskDetails.Reference)
	}
}

func TestTaskMap_GetAll(t *testing.T) {
	tm := NewTaskMap()

	// GetAll on empty map
	all := tm.GetAll()
	if len(all) != 0 {
		t.Errorf("GetAll() on empty map returned %d items, expected 0", len(all))
	}

	// Insert multiple tasks
	tasks := []struct {
		ref     types.ManagedObjectReference
		details TaskDetails
	}{
		{
			ref: types.ManagedObjectReference{Type: "Task", Value: "task-1"},
			details: TaskDetails{
				Reference:        types.ManagedObjectReference{Type: "Task", Value: "task-1"},
				MarkedForRemoval: false,
			},
		},
		{
			ref: types.ManagedObjectReference{Type: "Task", Value: "task-2"},
			details: TaskDetails{
				Reference:        types.ManagedObjectReference{Type: "Task", Value: "task-2"},
				MarkedForRemoval: true,
			},
		},
		{
			ref: types.ManagedObjectReference{Type: "Task", Value: "task-3"},
			details: TaskDetails{
				Reference:        types.ManagedObjectReference{Type: "Task", Value: "task-3"},
				MarkedForRemoval: false,
			},
		},
	}

	for _, task := range tasks {
		tm.Upsert(task.ref, task.details)
	}

	// GetAll should return all tasks
	all = tm.GetAll()
	if len(all) != len(tasks) {
		t.Errorf("GetAll() returned %d items, expected %d", len(all), len(tasks))
	}

	// Verify all task values are present
	refs := make(map[string]bool)
	for _, task := range all {
		refs[task.Reference.Value] = true
	}

	for _, task := range tasks {
		if !refs[task.ref.Value] {
			t.Errorf("Task %q not found in GetAll() result", task.ref.Value)
		}
	}
}

func TestTaskMap_Count(t *testing.T) {
	tm := NewTaskMap()

	// Empty map
	if tm.Count() != 0 {
		t.Errorf("Count() on empty map = %d, expected 0", tm.Count())
	}

	// Add tasks and verify count
	for i := 0; i < 5; i++ {
		taskRef := types.ManagedObjectReference{
			Type:  "Task",
			Value: "task-" + string(rune('0'+i)),
		}
		tm.Upsert(taskRef, TaskDetails{Reference: taskRef})

		if tm.Count() != i+1 {
			t.Errorf("Count() after %d inserts = %d, expected %d", i+1, tm.Count(), i+1)
		}
	}
}

func TestTaskMap_ConcurrentAccess(t *testing.T) {
	tm := NewTaskMap()
	var wg sync.WaitGroup

	// Number of goroutines for each operation
	numGoroutines := 100

	// Concurrent inserts
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			taskRef := types.ManagedObjectReference{
				Type:  "Task",
				Value: "task-" + string(rune(id)),
			}
			tm.Upsert(taskRef, TaskDetails{Reference: taskRef})
		}(i)
	}
	wg.Wait()

	// Concurrent reads
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			taskRef := types.ManagedObjectReference{
				Type:  "Task",
				Value: "task-" + string(rune(id)),
			}
			tm.Get(taskRef)
		}(i)
	}
	wg.Wait()

	// Concurrent GetAll
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			tm.GetAll()
		}()
	}
	wg.Wait()

	// Concurrent deletes
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			taskRef := types.ManagedObjectReference{
				Type:  "Task",
				Value: "task-" + string(rune(id)),
			}
			tm.Delete(taskRef)
		}(i)
	}
	wg.Wait()

	// Map should be empty after all deletes
	if tm.Count() != 0 {
		t.Errorf("Count() after concurrent operations = %d, expected 0", tm.Count())
	}
}

func TestTaskMap_MixedConcurrentOperations(t *testing.T) {
	tm := NewTaskMap()
	var wg sync.WaitGroup

	// Run mixed operations concurrently
	numOperations := 50

	// Writers
	wg.Add(numOperations)
	for i := 0; i < numOperations; i++ {
		go func(id int) {
			defer wg.Done()
			taskRef := types.ManagedObjectReference{
				Type:  "Task",
				Value: "task-" + string(rune(id%10)), // Use modulo to create conflicts
			}
			tm.Upsert(taskRef, TaskDetails{Reference: taskRef})
		}(i)
	}

	// Readers
	wg.Add(numOperations)
	for i := 0; i < numOperations; i++ {
		go func(id int) {
			defer wg.Done()
			taskRef := types.ManagedObjectReference{
				Type:  "Task",
				Value: "task-" + string(rune(id%10)),
			}
			tm.Get(taskRef)
		}(i)
	}

	// Count readers
	wg.Add(numOperations)
	for i := 0; i < numOperations; i++ {
		go func() {
			defer wg.Done()
			tm.Count()
		}()
	}

	// GetAll readers
	wg.Add(numOperations)
	for i := 0; i < numOperations; i++ {
		go func() {
			defer wg.Done()
			tm.GetAll()
		}()
	}

	wg.Wait()

	// Just verify no panics occurred and count is reasonable
	count := tm.Count()
	if count < 0 || count > 10 {
		t.Errorf("Count() = %d, expected between 0 and 10", count)
	}
}

func TestTaskDetails_Struct(t *testing.T) {
	// Test TaskDetails struct initialization
	ref := types.ManagedObjectReference{
		Type:  "Task",
		Value: "task-123",
	}

	details := TaskDetails{
		Reference:        ref,
		MarkedForRemoval: true,
		ResultCh:         make(chan TaskResult, 1),
	}

	if details.Reference != ref {
		t.Errorf("Reference = %v, expected %v", details.Reference, ref)
	}
	if details.MarkedForRemoval != true {
		t.Errorf("MarkedForRemoval = %v, expected %v", details.MarkedForRemoval, true)
	}
	if details.ResultCh == nil {
		t.Error("ResultCh should not be nil")
	}
}

func TestTaskResult_Struct(t *testing.T) {
	// Test TaskResult struct
	taskInfo := &types.TaskInfo{
		Key:  "task-1",
		Task: types.ManagedObjectReference{Type: "Task", Value: "task-1"},
	}

	result := TaskResult{
		TaskInfo: taskInfo,
		Err:      nil,
	}

	if result.TaskInfo != taskInfo {
		t.Errorf("TaskInfo = %v, expected %v", result.TaskInfo, taskInfo)
	}
	if result.Err != nil {
		t.Errorf("Err = %v, expected nil", result.Err)
	}
}
