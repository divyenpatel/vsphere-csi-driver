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

package storagepool

import (
	"errors"
	"testing"
	"time"

	v1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/csi/types"
)

func TestMakeStoragePoolName(t *testing.T) {
	tests := []struct {
		name     string
		dsName   string
		expected string
	}{
		{
			name:     "Simple datastore name",
			dsName:   "datastore1",
			expected: "storagepool-datastore1",
		},
		{
			name:     "Datastore name with spaces",
			dsName:   "my datastore",
			expected: "storagepool-my-datastore",
		},
		{
			name:     "Datastore name with special characters",
			dsName:   "ds@#$%name",
			expected: "storagepool-ds-name",
		},
		{
			name:     "Datastore name with dots",
			dsName:   "vsanDatastore.local",
			expected: "storagepool-vsandatastore.local",
		},
		{
			name:     "Datastore name with underscores",
			dsName:   "my_datastore_01",
			expected: "storagepool-my-datastore-01",
		},
		{
			name:     "Datastore name with mixed case",
			dsName:   "MyDataStore",
			expected: "storagepool-mydatastore",
		},
		{
			name:     "Datastore name ending with special chars",
			dsName:   "datastore---",
			expected: "storagepool-datastore",
		},
		{
			name:     "Datastore name with numbers",
			dsName:   "ds123456",
			expected: "storagepool-ds123456",
		},
		{
			name:     "Empty datastore name",
			dsName:   "",
			expected: "storagepool",
		},
		{
			name:     "Datastore with parentheses",
			dsName:   "datastore (1)",
			expected: "storagepool-datastore-1",
		},
		{
			name:     "Datastore with brackets",
			dsName:   "datastore[test]",
			expected: "storagepool-datastore-test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := makeStoragePoolName(tt.dsName)
			if result != tt.expected {
				t.Errorf("makeStoragePoolName(%q) = %q, expected %q", tt.dsName, result, tt.expected)
			}
		})
	}
}

func TestGetStoragePolicyIDFromSC(t *testing.T) {
	tests := []struct {
		name     string
		sc       *v1.StorageClass
		expected string
	}{
		{
			name: "StorageClass with storagePolicyID parameter",
			sc: &v1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
				Provisioner: types.Name,
				Parameters: map[string]string{
					"storagePolicyID": "policy-123",
				},
			},
			expected: "policy-123",
		},
		{
			name: "StorageClass with StoragePolicyID (different case)",
			sc: &v1.StorageClass{
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
			name: "StorageClass with STORAGEPOLICYID (uppercase)",
			sc: &v1.StorageClass{
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
			sc: &v1.StorageClass{
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
			sc: &v1.StorageClass{
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
			sc: &v1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
				Provisioner: "kubernetes.io/aws-ebs",
				Parameters: map[string]string{
					"storagePolicyID": "policy-123",
				},
			},
			expected: "",
		},
		{
			name: "StorageClass with empty storagePolicyID",
			sc: &v1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-sc",
				},
				Provisioner: types.Name,
				Parameters: map[string]string{
					"storagePolicyID": "",
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getStoragePolicyIDFromSC(tt.sc)
			if result != tt.expected {
				t.Errorf("getStoragePolicyIDFromSC() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestExponentialBackoff(t *testing.T) {
	tests := []struct {
		name               string
		taskFunc           func() (bool, error)
		baseDuration       time.Duration
		maxBackoffDuration time.Duration
		multiplier         float64
		retries            int
		expectedDone       bool
		expectedErr        bool
	}{
		{
			name: "Task succeeds on first try",
			taskFunc: func() (bool, error) {
				return true, nil
			},
			baseDuration:       10 * time.Millisecond,
			maxBackoffDuration: 100 * time.Millisecond,
			multiplier:         1.5,
			retries:            3,
			expectedDone:       true,
			expectedErr:        false,
		},
		{
			name: "Task fails with error",
			taskFunc: func() (bool, error) {
				return false, errors.New("task failed")
			},
			baseDuration:       10 * time.Millisecond,
			maxBackoffDuration: 100 * time.Millisecond,
			multiplier:         1.5,
			retries:            2,
			expectedDone:       false,
			expectedErr:        true,
		},
		{
			name: "Task returns false without error",
			taskFunc: func() (bool, error) {
				return false, nil
			},
			baseDuration:       10 * time.Millisecond,
			maxBackoffDuration: 100 * time.Millisecond,
			multiplier:         1.5,
			retries:            2,
			expectedDone:       false,
			expectedErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			done, err := ExponentialBackoff(tt.taskFunc, tt.baseDuration, tt.maxBackoffDuration, tt.multiplier, tt.retries)
			if done != tt.expectedDone {
				t.Errorf("ExponentialBackoff() done = %v, expected %v", done, tt.expectedDone)
			}
			if (err != nil) != tt.expectedErr {
				t.Errorf("ExponentialBackoff() error = %v, expectedErr = %v", err, tt.expectedErr)
			}
		})
	}
}

func TestExponentialBackoffSucceedsAfterRetries(t *testing.T) {
	attempts := 0
	taskFunc := func() (bool, error) {
		attempts++
		if attempts >= 3 {
			return true, nil
		}
		return false, nil
	}

	done, err := ExponentialBackoff(taskFunc, 10*time.Millisecond, 100*time.Millisecond, 1.5, 5)
	if !done {
		t.Error("ExponentialBackoff() should have succeeded after retries")
	}
	if err != nil {
		t.Errorf("ExponentialBackoff() unexpected error: %v", err)
	}
	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
}

func TestRetryOnError(t *testing.T) {
	tests := []struct {
		name               string
		taskFunc           func() error
		baseDuration       time.Duration
		maxBackoffDuration time.Duration
		multiplier         float64
		maxRetries         int
		expectedErr        bool
	}{
		{
			name: "Task succeeds on first try",
			taskFunc: func() error {
				return nil
			},
			baseDuration:       10 * time.Millisecond,
			maxBackoffDuration: 100 * time.Millisecond,
			multiplier:         1.5,
			maxRetries:         3,
			expectedErr:        false,
		},
		{
			name: "Task always fails",
			taskFunc: func() error {
				return errors.New("persistent error")
			},
			baseDuration:       10 * time.Millisecond,
			maxBackoffDuration: 100 * time.Millisecond,
			multiplier:         1.5,
			maxRetries:         2,
			expectedErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RetryOnError(tt.taskFunc, tt.baseDuration, tt.maxBackoffDuration, tt.multiplier, tt.maxRetries)
			if (err != nil) != tt.expectedErr {
				t.Errorf("RetryOnError() error = %v, expectedErr = %v", err, tt.expectedErr)
			}
		})
	}
}

func TestRetryOnErrorSucceedsAfterRetries(t *testing.T) {
	attempts := 0
	taskFunc := func() error {
		attempts++
		if attempts >= 2 {
			return nil
		}
		return errors.New("temporary error")
	}

	err := RetryOnError(taskFunc, 10*time.Millisecond, 100*time.Millisecond, 1.5, 5)
	if err != nil {
		t.Errorf("RetryOnError() unexpected error: %v", err)
	}
	if attempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempts)
	}
}

func TestDsPropsStruct(t *testing.T) {
	// Test dsProps struct initialization and fields
	props := dsProps{
		dsName:      "test-datastore",
		dsURL:       "ds:///vmfs/volumes/test-datastore/",
		dsType:      "cns.vmware.com/vsan",
		containerID: "container-123",
		inMM:        false,
		accessible:  true,
		capacity:    nil,
		freeSpace:   nil,
	}

	if props.dsName != "test-datastore" {
		t.Errorf("dsName = %q, expected %q", props.dsName, "test-datastore")
	}
	if props.dsURL != "ds:///vmfs/volumes/test-datastore/" {
		t.Errorf("dsURL = %q, expected %q", props.dsURL, "ds:///vmfs/volumes/test-datastore/")
	}
	if props.dsType != "cns.vmware.com/vsan" {
		t.Errorf("dsType = %q, expected %q", props.dsType, "cns.vmware.com/vsan")
	}
	if props.containerID != "container-123" {
		t.Errorf("containerID = %q, expected %q", props.containerID, "container-123")
	}
	if props.inMM != false {
		t.Errorf("inMM = %v, expected %v", props.inMM, false)
	}
	if props.accessible != true {
		t.Errorf("accessible = %v, expected %v", props.accessible, true)
	}
}

func TestStoragePoolConstants(t *testing.T) {
	// Test that constants are defined correctly
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{
			name:     "spTypePrefix",
			constant: spTypePrefix,
			expected: "cns.vmware.com/",
		},
		{
			name:     "spTypeLabelKey",
			constant: spTypeLabelKey,
			expected: "cns.vmware.com/StoragePoolType",
		},
		{
			name:     "spTypeAnnotationKey",
			constant: spTypeAnnotationKey,
			expected: "cns.vmware.com/StoragePoolTypeHint",
		},
		{
			name:     "vsanDsType",
			constant: vsanDsType,
			expected: "cns.vmware.com/vsan",
		},
		{
			name:     "nodeMoidAnnotation",
			constant: nodeMoidAnnotation,
			expected: "vmware-system-esxi-node-moid",
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

