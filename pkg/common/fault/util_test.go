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

package fault

import (
	"context"
	"testing"

	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/csi/service/logger"
)

func TestIsNonStorageFault(t *testing.T) {
	tests := []struct {
		name     string
		fault    string
		expected bool
	}{
		{
			name:     "VimFaultInvalidHostState is a non-storage fault",
			fault:    VimFaultInvalidHostState,
			expected: true,
		},
		{
			name:     "VimFaultHostNotConnected is a non-storage fault",
			fault:    VimFaultHostNotConnected,
			expected: true,
		},
		{
			name:     "VimFaultNotFound is not a non-storage fault",
			fault:    VimFaultNotFound,
			expected: false,
		},
		{
			name:     "VimFaultInvalidState is not a non-storage fault",
			fault:    VimFaultInvalidState,
			expected: false,
		},
		{
			name:     "CSIInternalFault is not a non-storage fault",
			fault:    CSIInternalFault,
			expected: false,
		},
		{
			name:     "Empty string is not a non-storage fault",
			fault:    "",
			expected: false,
		},
		{
			name:     "Random string is not a non-storage fault",
			fault:    "random.fault.type",
			expected: false,
		},
		{
			name:     "VimFaultCNSFault is not a non-storage fault",
			fault:    VimFaultCNSFault,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNonStorageFault(tt.fault)
			if result != tt.expected {
				t.Errorf("IsNonStorageFault(%q) = %v, expected %v", tt.fault, result, tt.expected)
			}
		})
	}
}

func TestAddCsiNonStoragePrefix(t *testing.T) {
	// Initialize logger for context
	ctx := logger.NewContextWithLogger(context.Background())

	tests := []struct {
		name     string
		fault    string
		expected string
	}{
		{
			name:     "Add prefix to non-empty fault",
			fault:    "InvalidHostState",
			expected: CSINonStorageFaultPrefix + "InvalidHostState",
		},
		{
			name:     "Add prefix to VimFault",
			fault:    "vim.fault.NotFound",
			expected: CSINonStorageFaultPrefix + "vim.fault.NotFound",
		},
		{
			name:     "Empty fault string returns empty",
			fault:    "",
			expected: "",
		},
		{
			name:     "Add prefix to simple fault name",
			fault:    "TestFault",
			expected: CSINonStorageFaultPrefix + "TestFault",
		},
		{
			name:     "Add prefix to fault with special characters",
			fault:    "fault.with.dots",
			expected: CSINonStorageFaultPrefix + "fault.with.dots",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AddCsiNonStoragePrefix(ctx, tt.fault)
			if result != tt.expected {
				t.Errorf("AddCsiNonStoragePrefix(ctx, %q) = %q, expected %q", tt.fault, result, tt.expected)
			}
		})
	}
}

func TestVimNonStorageFaultsList(t *testing.T) {
	// Verify the list contains expected faults
	expectedFaults := []string{VimFaultInvalidHostState, VimFaultHostNotConnected}

	if len(VimNonStorageFaultsList) != len(expectedFaults) {
		t.Errorf("VimNonStorageFaultsList length = %d, expected %d",
			len(VimNonStorageFaultsList), len(expectedFaults))
	}

	for _, expected := range expectedFaults {
		found := false
		for _, actual := range VimNonStorageFaultsList {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected fault %q not found in VimNonStorageFaultsList", expected)
		}
	}
}

func TestFaultConstants(t *testing.T) {
	// Verify fault constant values
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{
			name:     "CSITaskInfoEmptyFault",
			constant: CSITaskInfoEmptyFault,
			expected: "csi.fault.TaskInfoEmpty",
		},
		{
			name:     "CSINonStorageFaultPrefix",
			constant: CSINonStorageFaultPrefix,
			expected: "csi.fault.nonstorage.",
		},
		{
			name:     "VimFaultPrefix",
			constant: VimFaultPrefix,
			expected: "vim.fault.",
		},
		{
			name:     "CSIVmUuidNotFoundFault",
			constant: CSIVmUuidNotFoundFault,
			expected: "csi.fault.nonstorage.VmUuidNotFound",
		},
		{
			name:     "CSIVmNotFoundFault",
			constant: CSIVmNotFoundFault,
			expected: "csi.fault.nonstorage.VmNotFound",
		},
		{
			name:     "CSIDiskNotDetachedFault",
			constant: CSIDiskNotDetachedFault,
			expected: "csi.fault.nonstorage.DiskNotDetached",
		},
		{
			name:     "CSIDatacenterNotFoundFault",
			constant: CSIDatacenterNotFoundFault,
			expected: "csi.fault.DatacenterNotFound",
		},
		{
			name:     "CSIVCenterNotFoundFault",
			constant: CSIVCenterNotFoundFault,
			expected: "csi.fault.VCenterNotFound",
		},
		{
			name:     "CSIInternalFault",
			constant: CSIInternalFault,
			expected: "csi.fault.Internal",
		},
		{
			name:     "CSINotFoundFault",
			constant: CSINotFoundFault,
			expected: "csi.fault.NotFound",
		},
		{
			name:     "CSIInvalidArgumentFault",
			constant: CSIInvalidArgumentFault,
			expected: "csi.fault.InvalidArgument",
		},
		{
			name:     "CSIUnimplementedFault",
			constant: CSIUnimplementedFault,
			expected: "csi.fault.Unimplemented",
		},
		{
			name:     "VimFaultInvalidHostState",
			constant: VimFaultInvalidHostState,
			expected: "vim.fault.InvalidHostState",
		},
		{
			name:     "VimFaultHostNotConnected",
			constant: VimFaultHostNotConnected,
			expected: "vim.fault.HostNotConnected",
		},
		{
			name:     "VimFaultNotFound",
			constant: VimFaultNotFound,
			expected: "vim.fault.NotFound",
		},
		{
			name:     "VimFaultInvalidState",
			constant: VimFaultInvalidState,
			expected: "vim.fault.InvalidState",
		},
		{
			name:     "VimFaultInvalidDatastore",
			constant: VimFaultInvalidDatastore,
			expected: "vim.fault.InvalidDatastore",
		},
		{
			name:     "VimFaultTaskInProgress",
			constant: VimFaultTaskInProgress,
			expected: "vim.fault.TaskInProgress",
		},
		{
			name:     "VimFaultInvalidArgument",
			constant: VimFaultInvalidArgument,
			expected: "vim.fault.InvalidArgument",
		},
		{
			name:     "VimFaultCNSFault",
			constant: VimFaultCNSFault,
			expected: "vim.fault.CnsFault",
		},
		{
			name:     "VimFaultNotSupported",
			constant: VimFaultNotSupported,
			expected: "vim.fault.NotSupported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = %q, expected %q", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

