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
	"context"
	"sync"
	"testing"

	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/csi/service/logger"
)

func TestNormalizeUUID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Lowercase UUID",
			input:    "abc123",
			expected: "abc123",
		},
		{
			name:     "Uppercase UUID",
			input:    "ABC123",
			expected: "abc123",
		},
		{
			name:     "Mixed case UUID",
			input:    "AbC123DeF",
			expected: "abc123def",
		},
		{
			name:     "UUID with hyphens",
			input:    "ABC-123-DEF",
			expected: "abc-123-def",
		},
		{
			name:     "Empty UUID",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeUUID(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeUUID(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestErrNodeAlreadyExists(t *testing.T) {
	if ErrNodeAlreadyExists == nil {
		t.Error("ErrNodeAlreadyExists should not be nil")
	}

	expectedMsg := "another node with the same name exists"
	if ErrNodeAlreadyExists.Error() != expectedMsg {
		t.Errorf("ErrNodeAlreadyExists.Error() = %q, expected %q",
			ErrNodeAlreadyExists.Error(), expectedMsg)
	}
}

func TestDefaultCacheStoreAndLoad(t *testing.T) {
	ctx := logger.NewContextWithLogger(context.Background())

	cache := &defaultCache{
		mutex:        sync.Mutex{},
		uuidsToNames: make(map[string]string),
		namesToUUIDs: make(map[string]string),
	}

	nodeUUID := "UUID-123"
	nodeName := "node-1"

	// Test Store
	err := cache.Store(ctx, nodeUUID, nodeName)
	if err != nil {
		t.Errorf("Store() returned error: %v", err)
	}

	// Test LoadNodeNameByUUID
	loadedName, err := cache.LoadNodeNameByUUID(ctx, nodeUUID)
	if err != nil {
		t.Errorf("LoadNodeNameByUUID() returned error: %v", err)
	}
	if loadedName != nodeName {
		t.Errorf("LoadNodeNameByUUID() = %q, expected %q", loadedName, nodeName)
	}

	// Test LoadNodeUUIDByName
	loadedUUID, err := cache.LoadNodeUUIDByName(ctx, nodeName)
	if err != nil {
		t.Errorf("LoadNodeUUIDByName() returned error: %v", err)
	}
	// UUID should be normalized (lowercase)
	expectedUUID := "uuid-123"
	if loadedUUID != expectedUUID {
		t.Errorf("LoadNodeUUIDByName() = %q, expected %q", loadedUUID, expectedUUID)
	}
}

func TestDefaultCacheDeleteByUUID(t *testing.T) {
	ctx := logger.NewContextWithLogger(context.Background())

	cache := &defaultCache{
		mutex:        sync.Mutex{},
		uuidsToNames: make(map[string]string),
		namesToUUIDs: make(map[string]string),
	}

	nodeUUID := "uuid-456"
	nodeName := "node-2"

	// Store first
	err := cache.Store(ctx, nodeUUID, nodeName)
	if err != nil {
		t.Fatalf("Store() returned error: %v", err)
	}

	// Delete by UUID
	deletedName, err := cache.DeleteNodeByUUID(ctx, nodeUUID)
	if err != nil {
		t.Errorf("DeleteNodeByUUID() returned error: %v", err)
	}
	if deletedName != nodeName {
		t.Errorf("DeleteNodeByUUID() returned name %q, expected %q", deletedName, nodeName)
	}

	// Verify node is deleted
	_, err = cache.LoadNodeNameByUUID(ctx, nodeUUID)
	if err != ErrNodeNotFound {
		t.Errorf("Expected ErrNodeNotFound after delete, got: %v", err)
	}
}

func TestDefaultCacheDeleteByName(t *testing.T) {
	ctx := logger.NewContextWithLogger(context.Background())

	cache := &defaultCache{
		mutex:        sync.Mutex{},
		uuidsToNames: make(map[string]string),
		namesToUUIDs: make(map[string]string),
	}

	nodeUUID := "uuid-789"
	nodeName := "node-3"

	// Store first
	err := cache.Store(ctx, nodeUUID, nodeName)
	if err != nil {
		t.Fatalf("Store() returned error: %v", err)
	}

	// Delete by name
	deletedUUID, err := cache.DeleteNodeByName(ctx, nodeName)
	if err != nil {
		t.Errorf("DeleteNodeByName() returned error: %v", err)
	}
	if deletedUUID != nodeUUID {
		t.Errorf("DeleteNodeByName() returned UUID %q, expected %q", deletedUUID, nodeUUID)
	}

	// Verify node is deleted
	_, err = cache.LoadNodeUUIDByName(ctx, nodeName)
	if err != ErrNodeNotFound {
		t.Errorf("Expected ErrNodeNotFound after delete, got: %v", err)
	}
}

func TestDefaultCacheDeleteNonExistent(t *testing.T) {
	ctx := logger.NewContextWithLogger(context.Background())

	cache := &defaultCache{
		mutex:        sync.Mutex{},
		uuidsToNames: make(map[string]string),
		namesToUUIDs: make(map[string]string),
	}

	// Delete non-existent by UUID
	_, err := cache.DeleteNodeByUUID(ctx, "non-existent-uuid")
	if err != ErrNodeNotFound {
		t.Errorf("DeleteNodeByUUID() for non-existent should return ErrNodeNotFound, got: %v", err)
	}

	// Delete non-existent by name
	_, err = cache.DeleteNodeByName(ctx, "non-existent-name")
	if err != ErrNodeNotFound {
		t.Errorf("DeleteNodeByName() for non-existent should return ErrNodeNotFound, got: %v", err)
	}
}

func TestDefaultCacheLoadNonExistent(t *testing.T) {
	ctx := logger.NewContextWithLogger(context.Background())

	cache := &defaultCache{
		mutex:        sync.Mutex{},
		uuidsToNames: make(map[string]string),
		namesToUUIDs: make(map[string]string),
	}

	// Load non-existent by UUID
	_, err := cache.LoadNodeNameByUUID(ctx, "non-existent-uuid")
	if err != ErrNodeNotFound {
		t.Errorf("LoadNodeNameByUUID() for non-existent should return ErrNodeNotFound, got: %v", err)
	}

	// Load non-existent by name
	_, err = cache.LoadNodeUUIDByName(ctx, "non-existent-name")
	if err != ErrNodeNotFound {
		t.Errorf("LoadNodeUUIDByName() for non-existent should return ErrNodeNotFound, got: %v", err)
	}
}

func TestDefaultCacheRange(t *testing.T) {
	ctx := logger.NewContextWithLogger(context.Background())

	cache := &defaultCache{
		mutex:        sync.Mutex{},
		uuidsToNames: make(map[string]string),
		namesToUUIDs: make(map[string]string),
	}

	// Store multiple nodes
	nodes := map[string]string{
		"uuid-1": "node-1",
		"uuid-2": "node-2",
		"uuid-3": "node-3",
	}

	for uuid, name := range nodes {
		err := cache.Store(ctx, uuid, name)
		if err != nil {
			t.Fatalf("Store() returned error: %v", err)
		}
	}

	// Range and count
	count := 0
	cache.Range(ctx, func(nodeUUID, nodeName string) bool {
		count++
		return true
	})

	if count != len(nodes) {
		t.Errorf("Range() visited %d nodes, expected %d", count, len(nodes))
	}
}

func TestDefaultCacheRangeBreak(t *testing.T) {
	ctx := logger.NewContextWithLogger(context.Background())

	cache := &defaultCache{
		mutex:        sync.Mutex{},
		uuidsToNames: make(map[string]string),
		namesToUUIDs: make(map[string]string),
	}

	// Store multiple nodes
	for i := 1; i <= 5; i++ {
		err := cache.Store(ctx, "uuid-"+string(rune('0'+i)), "node-"+string(rune('0'+i)))
		if err != nil {
			t.Fatalf("Store() returned error: %v", err)
		}
	}

	// Range and break after first
	count := 0
	cache.Range(ctx, func(nodeUUID, nodeName string) bool {
		count++
		return false // Stop after first
	})

	if count != 1 {
		t.Errorf("Range() with break visited %d nodes, expected 1", count)
	}
}

func TestDefaultCacheUpdateExisting(t *testing.T) {
	ctx := logger.NewContextWithLogger(context.Background())

	cache := &defaultCache{
		mutex:        sync.Mutex{},
		uuidsToNames: make(map[string]string),
		namesToUUIDs: make(map[string]string),
	}

	nodeUUID := "uuid-update"
	oldName := "old-name"
	newName := "new-name"

	// Store with old name
	err := cache.Store(ctx, nodeUUID, oldName)
	if err != nil {
		t.Fatalf("Store() returned error: %v", err)
	}

	// Update with new name
	err = cache.Store(ctx, nodeUUID, newName)
	if err != nil {
		t.Fatalf("Store() update returned error: %v", err)
	}

	// Verify new name
	loadedName, err := cache.LoadNodeNameByUUID(ctx, nodeUUID)
	if err != nil {
		t.Errorf("LoadNodeNameByUUID() returned error: %v", err)
	}
	if loadedName != newName {
		t.Errorf("LoadNodeNameByUUID() = %q, expected %q", loadedName, newName)
	}

	// Verify old name is no longer valid
	_, err = cache.LoadNodeUUIDByName(ctx, oldName)
	if err != ErrNodeNotFound {
		t.Errorf("Old name should return ErrNodeNotFound, got: %v", err)
	}
}

func TestDefaultCacheConcurrentAccess(t *testing.T) {
	ctx := logger.NewContextWithLogger(context.Background())

	cache := &defaultCache{
		mutex:        sync.Mutex{},
		uuidsToNames: make(map[string]string),
		namesToUUIDs: make(map[string]string),
	}

	var wg sync.WaitGroup
	numGoroutines := 10

	// Concurrent stores
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			uuid := "uuid-" + string(rune('a'+id))
			name := "node-" + string(rune('a'+id))
			_ = cache.Store(ctx, uuid, name)
		}(i)
	}

	wg.Wait()

	// Verify all nodes are stored
	count := 0
	cache.Range(ctx, func(nodeUUID, nodeName string) bool {
		count++
		return true
	})

	if count != numGoroutines {
		t.Errorf("Expected %d nodes after concurrent stores, got %d", numGoroutines, count)
	}
}

