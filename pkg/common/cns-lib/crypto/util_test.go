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

package crypto

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/csi/types"
)

func TestGetEncryptionClassNameForPVC(t *testing.T) {
	tests := []struct {
		name     string
		pvc      *corev1.PersistentVolumeClaim
		expected string
	}{
		{
			name: "PVC with encryption class annotation",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
					Annotations: map[string]string{
						PVCEncryptionClassAnnotationName: "my-encryption-class",
					},
				},
			},
			expected: "my-encryption-class",
		},
		{
			name: "PVC without encryption class annotation",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
					Annotations: map[string]string{
						"other-annotation": "value",
					},
				},
			},
			expected: "",
		},
		{
			name: "PVC with nil annotations",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-pvc",
					Namespace:   "default",
					Annotations: nil,
				},
			},
			expected: "",
		},
		{
			name: "PVC with empty encryption class annotation",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
					Annotations: map[string]string{
						PVCEncryptionClassAnnotationName: "",
					},
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetEncryptionClassNameForPVC(tt.pvc)
			if result != tt.expected {
				t.Errorf("GetEncryptionClassNameForPVC() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestSetEncryptionClassNameForPVC(t *testing.T) {
	tests := []struct {
		name         string
		pvc          *corev1.PersistentVolumeClaim
		encClassName string
		expected     string
	}{
		{
			name: "Set encryption class on PVC with no annotations",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-pvc",
					Namespace:   "default",
					Annotations: nil,
				},
			},
			encClassName: "my-encryption-class",
			expected:     "my-encryption-class",
		},
		{
			name: "Set encryption class on PVC with existing annotations",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
					Annotations: map[string]string{
						"existing-annotation": "value",
					},
				},
			},
			encClassName: "my-encryption-class",
			expected:     "my-encryption-class",
		},
		{
			name: "Update existing encryption class",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
					Annotations: map[string]string{
						PVCEncryptionClassAnnotationName: "old-encryption-class",
					},
				},
			},
			encClassName: "new-encryption-class",
			expected:     "new-encryption-class",
		},
		{
			name: "Remove encryption class with empty string",
			pvc: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
					Annotations: map[string]string{
						PVCEncryptionClassAnnotationName: "my-encryption-class",
					},
				},
			},
			encClassName: "",
			expected:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetEncryptionClassNameForPVC(tt.pvc, tt.encClassName)
			result := GetEncryptionClassNameForPVC(tt.pvc)
			if result != tt.expected {
				t.Errorf("After SetEncryptionClassNameForPVC(), GetEncryptionClassNameForPVC() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestSetEncryptionClassNameForPVC_PreservesOtherAnnotations(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: "default",
			Annotations: map[string]string{
				"other-annotation": "other-value",
			},
		},
	}

	SetEncryptionClassNameForPVC(pvc, "my-encryption-class")

	// Verify encryption class was set
	if GetEncryptionClassNameForPVC(pvc) != "my-encryption-class" {
		t.Error("Encryption class was not set correctly")
	}

	// Verify other annotation is preserved
	if pvc.Annotations["other-annotation"] != "other-value" {
		t.Error("Other annotation was not preserved")
	}
}

func TestGetStoragePolicyID(t *testing.T) {
	tests := []struct {
		name     string
		sc       *storagev1.StorageClass
		expected string
	}{
		{
			name: "StorageClass with storagePolicyID parameter (lowercase)",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
				Provisioner: types.Name,
				Parameters: map[string]string{
					"storagepolicyid": "policy-123",
				},
			},
			expected: "policy-123",
		},
		{
			name: "StorageClass with storagePolicyID parameter (mixed case)",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
				Provisioner: types.Name,
				Parameters: map[string]string{
					"StoragePolicyID": "policy-456",
				},
			},
			expected: "policy-456",
		},
		{
			name: "StorageClass with storagePolicyID parameter (uppercase)",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
				Provisioner: types.Name,
				Parameters: map[string]string{
					"STORAGEPOLICYID": "policy-789",
				},
			},
			expected: "policy-789",
		},
		{
			name: "StorageClass without storagePolicyID parameter",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
				Provisioner: types.Name,
				Parameters: map[string]string{
					"otherParam": "value",
				},
			},
			expected: "",
		},
		{
			name: "StorageClass with nil parameters",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
				Provisioner: types.Name,
				Parameters:  nil,
			},
			expected: "",
		},
		{
			name: "StorageClass with different provisioner",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
				Provisioner: "kubernetes.io/aws-ebs",
				Parameters: map[string]string{
					"storagepolicyid": "policy-123",
				},
			},
			expected: "",
		},
		{
			name: "StorageClass with empty storagePolicyID",
			sc: &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
				Provisioner: types.Name,
				Parameters: map[string]string{
					"storagepolicyid": "",
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetStoragePolicyID(tt.sc)
			if result != tt.expected {
				t.Errorf("GetStoragePolicyID() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestCryptoConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{
			name:     "PVCEncryptionClassAnnotationName",
			constant: PVCEncryptionClassAnnotationName,
			expected: "csi.vsphere.encryption-class",
		},
		{
			name:     "DefaultEncryptionClassLabelName",
			constant: DefaultEncryptionClassLabelName,
			expected: "encryption.vmware.com/default",
		},
		{
			name:     "DefaultEncryptionClassLabelValue",
			constant: DefaultEncryptionClassLabelValue,
			expected: "true",
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

func TestCryptoErrors(t *testing.T) {
	// Test that error variables are properly initialized
	if ErrDefaultEncryptionClassNotFound == nil {
		t.Error("ErrDefaultEncryptionClassNotFound should not be nil")
	}

	if ErrMultipleDefaultEncryptionClasses == nil {
		t.Error("ErrMultipleDefaultEncryptionClasses should not be nil")
	}

	// Verify error messages contain expected content
	errMsg := ErrDefaultEncryptionClassNotFound.Error()
	if errMsg == "" {
		t.Error("ErrDefaultEncryptionClassNotFound.Error() should not be empty")
	}

	errMsg = ErrMultipleDefaultEncryptionClasses.Error()
	if errMsg == "" {
		t.Error("ErrMultipleDefaultEncryptionClasses.Error() should not be empty")
	}
}

