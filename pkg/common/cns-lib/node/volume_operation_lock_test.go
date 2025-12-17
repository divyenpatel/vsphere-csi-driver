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

package node

import (
	"sync"
	"testing"
)

func TestVolumeOperationAlreadyExistsFmt(t *testing.T) {
	expected := "An operation with the given Volume ID %s already exists"
	if VolumeOperationAlreadyExistsFmt != expected {
		t.Errorf("VolumeOperationAlreadyExistsFmt = %q, expected %q",
			VolumeOperationAlreadyExistsFmt, expected)
	}
}

func TestNewVolumeLocks(t *testing.T) {
	vl := NewVolumeLocks()
	if vl == nil {
		t.Fatal("NewVolumeLocks() returned nil")
	}
	if vl.locks == nil {
		t.Error("NewVolumeLocks().locks should not be nil")
	}
}

func TestVolumeLocksTryAcquire(t *testing.T) {
	vl := NewVolumeLocks()
	volumeID := "vol-123"

	// First acquire should succeed
	acquired := vl.TryAcquire(volumeID)
	if !acquired {
		t.Error("First TryAcquire() should return true")
	}

	// Second acquire should fail
	acquired = vl.TryAcquire(volumeID)
	if acquired {
		t.Error("Second TryAcquire() should return false")
	}
}

func TestVolumeLocksRelease(t *testing.T) {
	vl := NewVolumeLocks()
	volumeID := "vol-456"

	// Acquire lock
	acquired := vl.TryAcquire(volumeID)
	if !acquired {
		t.Fatal("TryAcquire() should return true")
	}

	// Release lock
	vl.Release(volumeID)

	// Should be able to acquire again
	acquired = vl.TryAcquire(volumeID)
	if !acquired {
		t.Error("TryAcquire() after Release() should return true")
	}
}

func TestVolumeLocksMultipleVolumes(t *testing.T) {
	vl := NewVolumeLocks()
	volumes := []string{"vol-1", "vol-2", "vol-3"}

	// Acquire all volumes
	for _, vol := range volumes {
		acquired := vl.TryAcquire(vol)
		if !acquired {
			t.Errorf("TryAcquire(%q) should return true", vol)
		}
	}

	// Verify all are locked
	for _, vol := range volumes {
		acquired := vl.TryAcquire(vol)
		if acquired {
			t.Errorf("TryAcquire(%q) on locked volume should return false", vol)
		}
	}

	// Release all
	for _, vol := range volumes {
		vl.Release(vol)
	}

	// Verify all can be acquired again
	for _, vol := range volumes {
		acquired := vl.TryAcquire(vol)
		if !acquired {
			t.Errorf("TryAcquire(%q) after Release() should return true", vol)
		}
	}
}

func TestVolumeLocksReleaseNonExistent(t *testing.T) {
	vl := NewVolumeLocks()

	// Release non-existent volume should not panic
	vl.Release("non-existent-vol")
}

func TestVolumeLocksConcurrentAccess(t *testing.T) {
	vl := NewVolumeLocks()
	volumeID := "concurrent-vol"
	numGoroutines := 100

	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	// Try to acquire from multiple goroutines
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if vl.TryAcquire(volumeID) {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// Only one should succeed
	if successCount != 1 {
		t.Errorf("Expected exactly 1 successful acquire, got %d", successCount)
	}
}

func TestVolumeLocksConcurrentDifferentVolumes(t *testing.T) {
	vl := NewVolumeLocks()
	numGoroutines := 50

	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	// Try to acquire different volumes from multiple goroutines
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			volumeID := "vol-" + string(rune('a'+id%26)) + string(rune('0'+id/26))
			if vl.TryAcquire(volumeID) {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// All should succeed since they're different volumes
	if successCount != numGoroutines {
		t.Errorf("Expected %d successful acquires, got %d", numGoroutines, successCount)
	}
}

func TestVolumeLocksAcquireReleaseSequence(t *testing.T) {
	vl := NewVolumeLocks()
	volumeID := "sequence-vol"

	// Sequence of acquire-release
	for i := 0; i < 10; i++ {
		acquired := vl.TryAcquire(volumeID)
		if !acquired {
			t.Errorf("Iteration %d: TryAcquire() should return true", i)
		}
		vl.Release(volumeID)
	}
}

func TestVolumeLocksEmptyVolumeID(t *testing.T) {
	vl := NewVolumeLocks()

	// Empty volume ID should still work
	acquired := vl.TryAcquire("")
	if !acquired {
		t.Error("TryAcquire(\"\") should return true")
	}

	acquired = vl.TryAcquire("")
	if acquired {
		t.Error("Second TryAcquire(\"\") should return false")
	}

	vl.Release("")

	acquired = vl.TryAcquire("")
	if !acquired {
		t.Error("TryAcquire(\"\") after Release() should return true")
	}
}

