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

package syncer

import (
	"context"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/csi/service/logger"
)

func TestSyncerConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{
			name:     "syncerComponent",
			constant: syncerComponent,
			expected: "VSphere CSI Syncer",
		},
		{
			name:     "staticVolumeProvisioningFailure",
			constant: staticVolumeProvisioningFailure,
			expected: "static volume provisioning failed",
		},
		{
			name:     "staticVolumeProvisioningSuccessReason",
			constant: staticVolumeProvisioningSuccessReason,
			expected: "static volume provisioning succeeded",
		},
		{
			name:     "staticVolumeProvisioningSuccessMessage",
			constant: staticVolumeProvisioningSuccessMessage,
			expected: "Successfully created container volume",
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

func TestSyncerIntConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant int
		expected int
	}{
		{
			name:     "allowedRetriesToPatchStoragePolicyUsage",
			constant: allowedRetriesToPatchStoragePolicyUsage,
			expected: 5,
		},
		{
			name:     "volumdIDLimitPerQuery",
			constant: volumdIDLimitPerQuery,
			expected: 1000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = %d, expected %d", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

func TestHasMigratedToAnnotationUpdate(t *testing.T) {
	ctx := logger.NewContextWithLogger(context.Background())

	tests := []struct {
		name            string
		prevAnnotations map[string]string
		newAnnotations  map[string]string
		objectName      string
		expected        bool
	}{
		{
			name:            "New annotation added",
			prevAnnotations: map[string]string{},
			newAnnotations:  map[string]string{"pv.kubernetes.io/migrated-to": "csi.vsphere.vmware.com"},
			objectName:      "test-pv",
			expected:        true,
		},
		{
			name:            "Annotation already exists",
			prevAnnotations: map[string]string{"pv.kubernetes.io/migrated-to": "csi.vsphere.vmware.com"},
			newAnnotations:  map[string]string{"pv.kubernetes.io/migrated-to": "csi.vsphere.vmware.com"},
			objectName:      "test-pv",
			expected:        false,
		},
		{
			name:            "No annotation in either",
			prevAnnotations: map[string]string{},
			newAnnotations:  map[string]string{},
			objectName:      "test-pv",
			expected:        false,
		},
		{
			name:            "Nil prev annotations",
			prevAnnotations: nil,
			newAnnotations:  map[string]string{"pv.kubernetes.io/migrated-to": "csi.vsphere.vmware.com"},
			objectName:      "test-pv",
			expected:        true,
		},
		{
			name:            "Other annotations present",
			prevAnnotations: map[string]string{"other": "value"},
			newAnnotations:  map[string]string{"other": "value", "pv.kubernetes.io/migrated-to": "csi.vsphere.vmware.com"},
			objectName:      "test-pv",
			expected:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasMigratedToAnnotationUpdate(ctx, tt.prevAnnotations, tt.newAnnotations, tt.objectName)
			if result != tt.expected {
				t.Errorf("HasMigratedToAnnotationUpdate() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestIsFileVolume(t *testing.T) {
	tests := []struct {
		name     string
		pv       *v1.PersistentVolume
		expected bool
	}{
		{
			name:     "Nil PV",
			pv:       nil,
			expected: false,
		},
		{
			name: "PV with no access modes",
			pv: &v1.PersistentVolume{
				Spec: v1.PersistentVolumeSpec{
					AccessModes: []v1.PersistentVolumeAccessMode{},
				},
			},
			expected: false,
		},
		{
			name: "PV with ReadWriteOnce",
			pv: &v1.PersistentVolume{
				Spec: v1.PersistentVolumeSpec{
					AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
				},
			},
			expected: false,
		},
		{
			name: "PV with ReadWriteMany",
			pv: &v1.PersistentVolume{
				Spec: v1.PersistentVolumeSpec{
					AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
				},
			},
			expected: true,
		},
		{
			name: "PV with ReadOnlyMany",
			pv: &v1.PersistentVolume{
				Spec: v1.PersistentVolumeSpec{
					AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadOnlyMany},
				},
			},
			expected: true,
		},
		{
			name: "PV with multiple access modes including ReadWriteMany",
			pv: &v1.PersistentVolume{
				Spec: v1.PersistentVolumeSpec{
					AccessModes: []v1.PersistentVolumeAccessMode{
						v1.ReadWriteOnce,
						v1.ReadWriteMany,
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsFileVolume(tt.pv)
			if result != tt.expected {
				t.Errorf("IsFileVolume() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestGetPatchData(t *testing.T) {
	tests := []struct {
		name        string
		oldObj      interface{}
		newObj      interface{}
		expectError bool
	}{
		{
			name:        "Simple map patch",
			oldObj:      map[string]string{"key": "oldValue"},
			newObj:      map[string]string{"key": "newValue"},
			expectError: false,
		},
		{
			name:        "Add new key",
			oldObj:      map[string]string{"key1": "value1"},
			newObj:      map[string]string{"key1": "value1", "key2": "value2"},
			expectError: false,
		},
		{
			name:        "Empty objects",
			oldObj:      map[string]string{},
			newObj:      map[string]string{},
			expectError: false,
		},
		{
			name: "Struct objects",
			oldObj: struct {
				Name string `json:"name"`
			}{Name: "old"},
			newObj: struct {
				Name string `json:"name"`
			}{Name: "new"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := getPatchData(tt.oldObj, tt.newObj)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result == nil {
					t.Error("Expected non-nil patch data")
				}
			}
		})
	}
}

func TestIsDynamicallyCreatedVolume(t *testing.T) {
	ctx := logger.NewContextWithLogger(context.Background())

	tests := []struct {
		name     string
		pv       *v1.PersistentVolume
		expected bool
	}{
		{
			name: "Dynamically created CSI volume",
			pv: &v1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pv",
				},
				Spec: v1.PersistentVolumeSpec{
					PersistentVolumeSource: v1.PersistentVolumeSource{
						CSI: &v1.CSIPersistentVolumeSource{
							Driver:       "csi.vsphere.vmware.com",
							VolumeHandle: "volume-123",
							VolumeAttributes: map[string]string{
								"storage.kubernetes.io/csiProvisionerIdentity": "provisioner-id",
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "Statically provisioned CSI volume",
			pv: &v1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pv",
				},
				Spec: v1.PersistentVolumeSpec{
					PersistentVolumeSource: v1.PersistentVolumeSource{
						CSI: &v1.CSIPersistentVolumeSource{
							Driver:           "csi.vsphere.vmware.com",
							VolumeHandle:     "volume-123",
							VolumeAttributes: map[string]string{},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "Volume without CSI spec",
			pv: &v1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pv",
				},
				Spec: v1.PersistentVolumeSpec{},
			},
			expected: false,
		},
		{
			name: "CSI volume with nil VolumeAttributes",
			pv: &v1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pv",
				},
				Spec: v1.PersistentVolumeSpec{
					PersistentVolumeSource: v1.PersistentVolumeSource{
						CSI: &v1.CSIPersistentVolumeSource{
							Driver:           "csi.vsphere.vmware.com",
							VolumeHandle:     "volume-123",
							VolumeAttributes: nil,
						},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isDynamicallyCreatedVolume(ctx, tt.pv)
			if result != tt.expected {
				t.Errorf("isDynamicallyCreatedVolume() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestGetTopologySegmentsFromNodeAffinityRules(t *testing.T) {
	ctx := logger.NewContextWithLogger(context.Background())

	tests := []struct {
		name           string
		pv             *v1.PersistentVolume
		expectedLength int
	}{
		{
			name: "PV with no node affinity",
			pv: &v1.PersistentVolume{
				Spec: v1.PersistentVolumeSpec{
					NodeAffinity: nil,
				},
			},
			expectedLength: 0,
		},
		{
			name: "PV with empty node affinity",
			pv: &v1.PersistentVolume{
				Spec: v1.PersistentVolumeSpec{
					NodeAffinity: &v1.VolumeNodeAffinity{},
				},
			},
			expectedLength: 0,
		},
		{
			name: "PV with node affinity but no required",
			pv: &v1.PersistentVolume{
				Spec: v1.PersistentVolumeSpec{
					NodeAffinity: &v1.VolumeNodeAffinity{
						Required: nil,
					},
				},
			},
			expectedLength: 0,
		},
		{
			name: "PV with node affinity and selector terms",
			pv: &v1.PersistentVolume{
				Spec: v1.PersistentVolumeSpec{
					NodeAffinity: &v1.VolumeNodeAffinity{
						Required: &v1.NodeSelector{
							NodeSelectorTerms: []v1.NodeSelectorTerm{
								{
									MatchExpressions: []v1.NodeSelectorRequirement{
										{
											Key:      "topology.kubernetes.io/zone",
											Operator: v1.NodeSelectorOpIn,
											Values:   []string{"zone-a"},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedLength: 1,
		},
		{
			name: "PV with multiple node selector terms",
			pv: &v1.PersistentVolume{
				Spec: v1.PersistentVolumeSpec{
					NodeAffinity: &v1.VolumeNodeAffinity{
						Required: &v1.NodeSelector{
							NodeSelectorTerms: []v1.NodeSelectorTerm{
								{
									MatchExpressions: []v1.NodeSelectorRequirement{
										{
											Key:      "topology.kubernetes.io/zone",
											Operator: v1.NodeSelectorOpIn,
											Values:   []string{"zone-a"},
										},
									},
								},
								{
									MatchExpressions: []v1.NodeSelectorRequirement{
										{
											Key:      "topology.kubernetes.io/region",
											Operator: v1.NodeSelectorOpIn,
											Values:   []string{"region-1"},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedLength: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getTopologySegmentsFromNodeAffinityRules(ctx, tt.pv)
			if len(result) != tt.expectedLength {
				t.Errorf("getTopologySegmentsFromNodeAffinityRules() returned %d segments, expected %d",
					len(result), tt.expectedLength)
			}
		})
	}
}

