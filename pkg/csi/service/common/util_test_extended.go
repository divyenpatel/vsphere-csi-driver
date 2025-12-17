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

package common

import (
	"context"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/vmware/govmomi/vim25/types"

	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/csi/service/logger"
)

func TestGetUUIDFromProviderID(t *testing.T) {
	tests := []struct {
		name       string
		providerID string
		expected   string
	}{
		{
			name:       "Provider ID with prefix",
			providerID: "vsphere://4237539071f943a3a77056803bcd7baa",
			expected:   "4237539071f943a3a77056803bcd7baa",
		},
		{
			name:       "Provider ID without prefix",
			providerID: "4237539071f943a3a77056803bcd7baa",
			expected:   "4237539071f943a3a77056803bcd7baa",
		},
		{
			name:       "Empty provider ID",
			providerID: "",
			expected:   "",
		},
		{
			name:       "Provider ID with only prefix",
			providerID: "vsphere://",
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetUUIDFromProviderID(tt.providerID)
			if result != tt.expected {
				t.Errorf("GetUUIDFromProviderID(%q) = %q, expected %q", tt.providerID, result, tt.expected)
			}
		})
	}
}

func TestFormatDiskUUID(t *testing.T) {
	tests := []struct {
		name     string
		uuid     string
		expected string
	}{
		{
			name:     "UUID with hyphens",
			uuid:     "42375390-71f9-43a3-a770-56803bcd7baa",
			expected: "4237539071f943a3a77056803bcd7baa",
		},
		{
			name:     "UUID with spaces",
			uuid:     "42375390 71f9 43a3 a770 56803bcd7baa",
			expected: "4237539071f943a3a77056803bcd7baa",
		},
		{
			name:     "UUID with mixed case",
			uuid:     "42375390-71F9-43A3-A770-56803BCD7BAA",
			expected: "4237539071f943a3a77056803bcd7baa",
		},
		{
			name:     "UUID already formatted",
			uuid:     "4237539071f943a3a77056803bcd7baa",
			expected: "4237539071f943a3a77056803bcd7baa",
		},
		{
			name:     "Empty UUID",
			uuid:     "",
			expected: "",
		},
		{
			name:     "UUID with spaces and hyphens",
			uuid:     "42375390 - 71f9 - 43a3 - a770 - 56803bcd7baa",
			expected: "4237539071f943a3a77056803bcd7baa",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatDiskUUID(tt.uuid)
			if result != tt.expected {
				t.Errorf("FormatDiskUUID(%q) = %q, expected %q", tt.uuid, result, tt.expected)
			}
		})
	}
}

func TestRoundUpSize(t *testing.T) {
	tests := []struct {
		name                string
		volumeSizeBytes     int64
		allocationUnitBytes int64
		expected            int64
	}{
		{
			name:                "Exact multiple",
			volumeSizeBytes:     1024,
			allocationUnitBytes: 512,
			expected:            2,
		},
		{
			name:                "Needs rounding up",
			volumeSizeBytes:     1025,
			allocationUnitBytes: 512,
			expected:            3,
		},
		{
			name:                "Single unit",
			volumeSizeBytes:     100,
			allocationUnitBytes: 512,
			expected:            1,
		},
		{
			name:                "Zero volume size",
			volumeSizeBytes:     0,
			allocationUnitBytes: 512,
			expected:            0,
		},
		{
			name:                "Large volume",
			volumeSizeBytes:     10737418240, // 10GB
			allocationUnitBytes: 1073741824,  // 1GB
			expected:            10,
		},
		{
			name:                "Volume size equals allocation unit",
			volumeSizeBytes:     512,
			allocationUnitBytes: 512,
			expected:            1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RoundUpSize(tt.volumeSizeBytes, tt.allocationUnitBytes)
			if result != tt.expected {
				t.Errorf("RoundUpSize(%d, %d) = %d, expected %d",
					tt.volumeSizeBytes, tt.allocationUnitBytes, result, tt.expected)
			}
		})
	}
}

func TestGetLabelsMapFromKeyValue(t *testing.T) {
	tests := []struct {
		name     string
		labels   []types.KeyValue
		expected map[string]string
	}{
		{
			name:     "Empty labels",
			labels:   []types.KeyValue{},
			expected: map[string]string{},
		},
		{
			name: "Single label",
			labels: []types.KeyValue{
				{Key: "key1", Value: "value1"},
			},
			expected: map[string]string{"key1": "value1"},
		},
		{
			name: "Multiple labels",
			labels: []types.KeyValue{
				{Key: "key1", Value: "value1"},
				{Key: "key2", Value: "value2"},
				{Key: "key3", Value: "value3"},
			},
			expected: map[string]string{
				"key1": "value1",
				"key2": "value2",
				"key3": "value3",
			},
		},
		{
			name: "Duplicate keys (last wins)",
			labels: []types.KeyValue{
				{Key: "key1", Value: "value1"},
				{Key: "key1", Value: "value2"},
			},
			expected: map[string]string{"key1": "value2"},
		},
		{
			name: "Empty key and value",
			labels: []types.KeyValue{
				{Key: "", Value: ""},
			},
			expected: map[string]string{"": ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetLabelsMapFromKeyValue(tt.labels)
			if len(result) != len(tt.expected) {
				t.Errorf("GetLabelsMapFromKeyValue() returned map with %d entries, expected %d",
					len(result), len(tt.expected))
			}
			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("GetLabelsMapFromKeyValue()[%q] = %q, expected %q", k, result[k], v)
				}
			}
		})
	}
}

func TestIsFileVolumeRequest(t *testing.T) {
	ctx := logger.NewContextWithLogger(context.Background())

	tests := []struct {
		name         string
		capabilities []*csi.VolumeCapability
		expected     bool
	}{
		{
			name:         "Nil capabilities",
			capabilities: nil,
			expected:     false,
		},
		{
			name:         "Empty capabilities",
			capabilities: []*csi.VolumeCapability{},
			expected:     false,
		},
		{
			name: "Single node writer",
			capabilities: []*csi.VolumeCapability{
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			expected: false,
		},
		{
			name: "Multi node reader only",
			capabilities: []*csi.VolumeCapability{
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
					},
				},
			},
			expected: true,
		},
		{
			name: "Multi node single writer",
			capabilities: []*csi.VolumeCapability{
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER,
					},
				},
			},
			expected: true,
		},
		{
			name: "Multi node multi writer",
			capabilities: []*csi.VolumeCapability{
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
					},
				},
			},
			expected: true,
		},
		{
			name: "Mixed capabilities with file volume",
			capabilities: []*csi.VolumeCapability{
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsFileVolumeRequest(ctx, tt.capabilities)
			if result != tt.expected {
				t.Errorf("IsFileVolumeRequest() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestIsVolumeReadOnly(t *testing.T) {
	tests := []struct {
		name       string
		capability *csi.VolumeCapability
		expected   bool
	}{
		{
			name: "Single node reader only",
			capability: &csi.VolumeCapability{
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
				},
			},
			expected: true,
		},
		{
			name: "Multi node reader only",
			capability: &csi.VolumeCapability{
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
				},
			},
			expected: true,
		},
		{
			name: "Single node writer",
			capability: &csi.VolumeCapability{
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
			expected: false,
		},
		{
			name: "Multi node single writer",
			capability: &csi.VolumeCapability{
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER,
				},
			},
			expected: false,
		},
		{
			name: "Multi node multi writer",
			capability: &csi.VolumeCapability{
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsVolumeReadOnly(tt.capability)
			if result != tt.expected {
				t.Errorf("IsVolumeReadOnly() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

