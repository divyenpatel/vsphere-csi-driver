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
	"context"
	"errors"
	"testing"

	cnstypes "github.com/vmware/govmomi/cns/types"
	"github.com/vmware/govmomi/vim25/types"

	csifault "sigs.k8s.io/vsphere-csi-driver/v3/pkg/common/fault"
	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/csi/service/logger"
)

func TestIsStaticallyProvisioned_BlockVolume(t *testing.T) {
	tests := []struct {
		name     string
		spec     *cnstypes.CnsVolumeCreateSpec
		expected bool
	}{
		{
			name: "Block volume with BackingDiskId is statically provisioned",
			spec: &cnstypes.CnsVolumeCreateSpec{
				VolumeType: string(cnstypes.CnsVolumeTypeBlock),
				BackingObjectDetails: &cnstypes.CnsBlockBackingDetails{
					BackingDiskId: "disk-123",
				},
			},
			expected: true,
		},
		{
			name: "Block volume with BackingDiskUrlPath is statically provisioned",
			spec: &cnstypes.CnsVolumeCreateSpec{
				VolumeType: string(cnstypes.CnsVolumeTypeBlock),
				BackingObjectDetails: &cnstypes.CnsBlockBackingDetails{
					BackingDiskUrlPath: "/vmfs/volumes/datastore1/disk.vmdk",
				},
			},
			expected: true,
		},
		{
			name: "Block volume without backing details is dynamically provisioned",
			spec: &cnstypes.CnsVolumeCreateSpec{
				VolumeType: string(cnstypes.CnsVolumeTypeBlock),
				BackingObjectDetails: &cnstypes.CnsBlockBackingDetails{
					BackingDiskId:      "",
					BackingDiskUrlPath: "",
				},
			},
			expected: false,
		},
		{
			name: "Block volume with nil BackingObjectDetails",
			spec: &cnstypes.CnsVolumeCreateSpec{
				VolumeType:           string(cnstypes.CnsVolumeTypeBlock),
				BackingObjectDetails: nil,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isStaticallyProvisioned(tt.spec)
			if result != tt.expected {
				t.Errorf("isStaticallyProvisioned() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestGetNvmeUUID(t *testing.T) {
	ctx := logger.NewContextWithLogger(context.Background())

	tests := []struct {
		name        string
		uuid        string
		expectError bool
	}{
		{
			name:        "Valid UUID",
			uuid:        "52a3d6f0-c4b5-4e8a-9d1f-2e3b4c5d6e7f",
			expectError: false,
		},
		{
			name:        "Another valid UUID",
			uuid:        "00000000-0000-0000-0000-000000000000",
			expectError: false,
		},
		{
			name:        "Invalid UUID format",
			uuid:        "invalid-uuid",
			expectError: true,
		},
		{
			name:        "Empty UUID",
			uuid:        "",
			expectError: true,
		},
		{
			name:        "UUID without dashes",
			uuid:        "52a3d6f0c4b54e8a9d1f2e3b4c5d6e7f",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := getNvmeUUID(ctx, tt.uuid)
			if tt.expectError {
				if err == nil {
					t.Errorf("getNvmeUUID(%q) expected error, got nil", tt.uuid)
				}
			} else {
				if err != nil {
					t.Errorf("getNvmeUUID(%q) unexpected error: %v", tt.uuid, err)
				}
				if result == "" {
					t.Errorf("getNvmeUUID(%q) returned empty result", tt.uuid)
				}
			}
		})
	}
}

func TestExtractFaultTypeFromErr(t *testing.T) {
	ctx := logger.NewContextWithLogger(context.Background())

	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "Non-SOAP fault returns CSIInternalFault",
			err:      errors.New("regular error"),
			expected: csifault.CSIInternalFault,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractFaultTypeFromErr(ctx, tt.err)
			if result != tt.expected {
				t.Errorf("ExtractFaultTypeFromErr() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestExtractFaultTypeFromVolumeResponseResult(t *testing.T) {
	ctx := logger.NewContextWithLogger(context.Background())

	tests := []struct {
		name     string
		resp     *cnstypes.CnsVolumeOperationResult
		expected string
	}{
		{
			name: "Response with no fault",
			resp: &cnstypes.CnsVolumeOperationResult{
				Fault: nil,
			},
			expected: "",
		},
		{
			name: "Response with fault but nil inner fault",
			resp: &cnstypes.CnsVolumeOperationResult{
				Fault: &types.LocalizedMethodFault{
					Fault: nil,
				},
			},
			expected: "*types.LocalizedMethodFault",
		},
		{
			name: "Response with NotFound fault",
			resp: &cnstypes.CnsVolumeOperationResult{
				Fault: &types.LocalizedMethodFault{
					Fault: &types.NotFound{},
				},
			},
			expected: csifault.VimFaultPrefix + "NotFound",
		},
		{
			name: "Response with InvalidArgument fault",
			resp: &cnstypes.CnsVolumeOperationResult{
				Fault: &types.LocalizedMethodFault{
					Fault: &types.InvalidArgument{},
				},
			},
			expected: csifault.VimFaultPrefix + "InvalidArgument",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractFaultTypeFromVolumeResponseResult(ctx, tt.resp)
			if result != tt.expected {
				t.Errorf("ExtractFaultTypeFromVolumeResponseResult() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestIsNotFoundFault(t *testing.T) {
	ctx := logger.NewContextWithLogger(context.Background())

	tests := []struct {
		name      string
		faultType string
		expected  bool
	}{
		{
			name:      "vim.fault.NotFound returns true",
			faultType: "vim.fault.NotFound",
			expected:  true,
		},
		{
			name:      "Different fault returns false",
			faultType: "vim.fault.InvalidState",
			expected:  false,
		},
		{
			name:      "Empty string returns false",
			faultType: "",
			expected:  false,
		},
		{
			name:      "Case sensitive - lowercase returns false",
			faultType: "vim.fault.notfound",
			expected:  false,
		},
		{
			name:      "Similar but different fault returns false",
			faultType: "vim.fault.NotFoundFault",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNotFoundFault(ctx, tt.faultType)
			if result != tt.expected {
				t.Errorf("IsNotFoundFault(%q) = %v, expected %v", tt.faultType, result, tt.expected)
			}
		})
	}
}

func TestIsNotSupportedFaultType(t *testing.T) {
	ctx := logger.NewContextWithLogger(context.Background())

	tests := []struct {
		name      string
		faultType string
		expected  bool
	}{
		{
			name:      "vim25:NotSupported returns true",
			faultType: "vim25:NotSupported",
			expected:  true,
		},
		{
			name:      "Different fault returns false",
			faultType: "vim.fault.NotSupported",
			expected:  false,
		},
		{
			name:      "Empty string returns false",
			faultType: "",
			expected:  false,
		},
		{
			name:      "Case sensitive check",
			faultType: "vim25:notsupported",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNotSupportedFaultType(ctx, tt.faultType)
			if result != tt.expected {
				t.Errorf("IsNotSupportedFaultType(%q) = %v, expected %v", tt.faultType, result, tt.expected)
			}
		})
	}
}

func TestIsCnsVolumeAlreadyExistsFault(t *testing.T) {
	ctx := logger.NewContextWithLogger(context.Background())

	tests := []struct {
		name      string
		faultType string
		expected  bool
	}{
		{
			name:      "vim.fault.CnsVolumeAlreadyExistsFault returns true",
			faultType: "vim.fault.CnsVolumeAlreadyExistsFault",
			expected:  true,
		},
		{
			name:      "Different fault returns false",
			faultType: "vim.fault.NotFound",
			expected:  false,
		},
		{
			name:      "Empty string returns false",
			faultType: "",
			expected:  false,
		},
		{
			name:      "Case sensitive check",
			faultType: "vim.fault.cnsvolumealreadyexistsfault",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsCnsVolumeAlreadyExistsFault(ctx, tt.faultType)
			if result != tt.expected {
				t.Errorf("IsCnsVolumeAlreadyExistsFault(%q) = %v, expected %v", tt.faultType, result, tt.expected)
			}
		})
	}
}

func TestIsNotSupportedFault(t *testing.T) {
	ctx := logger.NewContextWithLogger(context.Background())

	tests := []struct {
		name     string
		fault    *types.LocalizedMethodFault
		expected bool
	}{
		{
			name: "Fault with nil inner fault returns false",
			fault: &types.LocalizedMethodFault{
				Fault: nil,
			},
			expected: false,
		},
		{
			name: "Non-CnsFault returns false",
			fault: &types.LocalizedMethodFault{
				Fault: &types.NotFound{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.fault == nil {
				// Skip nil fault test as the function doesn't handle nil
				t.Skip("Skipping nil fault test")
			}
			result := IsNotSupportedFault(ctx, tt.fault)
			if result != tt.expected {
				t.Errorf("IsNotSupportedFault() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestBatchAttachRequest(t *testing.T) {
	// Test BatchAttachRequest struct initialization
	controllerKey := int32(1000)
	unitNumber := int32(0)
	encrypted := true

	req := BatchAttachRequest{
		VolumeID:        "vol-123",
		SharingMode:     "sharingNone",
		DiskMode:        "persistent",
		ControllerKey:   &controllerKey,
		UnitNumber:      &unitNumber,
		BackingType:     "block",
		VolumeEncrypted: &encrypted,
	}

	if req.VolumeID != "vol-123" {
		t.Errorf("VolumeID = %q, expected %q", req.VolumeID, "vol-123")
	}
	if req.SharingMode != "sharingNone" {
		t.Errorf("SharingMode = %q, expected %q", req.SharingMode, "sharingNone")
	}
	if req.DiskMode != "persistent" {
		t.Errorf("DiskMode = %q, expected %q", req.DiskMode, "persistent")
	}
	if *req.ControllerKey != 1000 {
		t.Errorf("ControllerKey = %d, expected %d", *req.ControllerKey, 1000)
	}
	if *req.UnitNumber != 0 {
		t.Errorf("UnitNumber = %d, expected %d", *req.UnitNumber, 0)
	}
	if req.BackingType != "block" {
		t.Errorf("BackingType = %q, expected %q", req.BackingType, "block")
	}
	if *req.VolumeEncrypted != true {
		t.Errorf("VolumeEncrypted = %v, expected %v", *req.VolumeEncrypted, true)
	}
}

func TestBatchAttachResult(t *testing.T) {
	// Test BatchAttachResult struct initialization
	result := BatchAttachResult{
		Error:     errors.New("attach failed"),
		DiskUUID:  "uuid-123",
		VolumeID:  "vol-123",
		FaultType: "vim.fault.NotFound",
	}

	if result.Error == nil {
		t.Error("Error should not be nil")
	}
	if result.DiskUUID != "uuid-123" {
		t.Errorf("DiskUUID = %q, expected %q", result.DiskUUID, "uuid-123")
	}
	if result.VolumeID != "vol-123" {
		t.Errorf("VolumeID = %q, expected %q", result.VolumeID, "vol-123")
	}
	if result.FaultType != "vim.fault.NotFound" {
		t.Errorf("FaultType = %q, expected %q", result.FaultType, "vim.fault.NotFound")
	}
}
