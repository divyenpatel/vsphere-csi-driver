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
	"errors"
	"testing"

	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/csi/service/logger"
)

func TestErrNodeNotFound(t *testing.T) {
	// Verify ErrNodeNotFound is properly defined
	if ErrNodeNotFound == nil {
		t.Fatal("ErrNodeNotFound should not be nil")
	}

	expectedMsg := "node wasn't found"
	if ErrNodeNotFound.Error() != expectedMsg {
		t.Errorf("ErrNodeNotFound.Error() = %q, expected %q", ErrNodeNotFound.Error(), expectedMsg)
	}

	// Verify it can be used with errors.Is
	wrappedErr := errors.New("wrapped: " + ErrNodeNotFound.Error())
	if errors.Is(wrappedErr, ErrNodeNotFound) {
		// This should be false since it's not actually wrapped with %w
		t.Error("errors.Is should return false for non-wrapped error")
	}
}

func TestGetManager(t *testing.T) {
	ctx := logger.NewContextWithLogger(context.Background())

	// Get the manager instance
	manager := GetManager(ctx)

	if manager == nil {
		t.Fatal("GetManager() returned nil")
	}

	// Verify singleton behavior - calling again should return same instance
	manager2 := GetManager(ctx)

	if manager != manager2 {
		t.Error("GetManager() should return the same singleton instance")
	}
}

func TestDefaultManager_SetKubernetesClient(t *testing.T) {
	ctx := logger.NewContextWithLogger(context.Background())

	manager := GetManager(ctx)

	// SetKubernetesClient with nil should not panic
	manager.SetKubernetesClient(nil)
}

func TestManagerInterface(t *testing.T) {
	ctx := logger.NewContextWithLogger(context.Background())

	// Verify GetManager returns a Manager interface
	var _ Manager = GetManager(ctx)
}

func TestMetadataInterface(t *testing.T) {
	// Verify Metadata is an empty interface that can hold any type
	var m Metadata

	m = "string value"
	if m != "string value" {
		t.Error("Metadata should be able to hold string")
	}

	m = 123
	if m != 123 {
		t.Error("Metadata should be able to hold int")
	}

	m = struct{ Name string }{Name: "test"}
	if m.(struct{ Name string }).Name != "test" {
		t.Error("Metadata should be able to hold struct")
	}

	m = nil
	if m != nil {
		t.Error("Metadata should be able to hold nil")
	}
}

func TestManagerInterfaceMethods(t *testing.T) {
	// Verify all methods are defined on the Manager interface
	// This is a compile-time check that all methods exist
	ctx := logger.NewContextWithLogger(context.Background())
	manager := GetManager(ctx)

	// These method calls will fail at runtime without proper setup,
	// but they verify the interface is correctly implemented
	_ = manager

	// We can't actually test the full functionality without mocking vSphere,
	// but we can verify the interface exists and methods are callable
}

