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
	"testing"
)

func TestByteConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant int64
		expected int64
	}{
		{
			name:     "MbInBytes",
			constant: MbInBytes,
			expected: 1024 * 1024,
		},
		{
			name:     "GbInBytes",
			constant: GbInBytes,
			expected: 1024 * 1024 * 1024,
		},
		{
			name:     "DefaultGbDiskSize",
			constant: DefaultGbDiskSize,
			expected: 10,
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

func TestDiskTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{
			name:     "DiskTypeBlockVolume",
			constant: DiskTypeBlockVolume,
			expected: "vSphere CNS Block Volume",
		},
		{
			name:     "DiskTypeFileVolume",
			constant: DiskTypeFileVolume,
			expected: "vSphere CNS File Volume",
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

func TestAttributeConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{
			name:     "AttributeDiskType",
			constant: AttributeDiskType,
			expected: "type",
		},
		{
			name:     "AttributeDatastoreURL",
			constant: AttributeDatastoreURL,
			expected: "datastoreurl",
		},
		{
			name:     "AttributeStoragePolicyName",
			constant: AttributeStoragePolicyName,
			expected: "storagepolicyname",
		},
		{
			name:     "AttributeStoragePolicyID",
			constant: AttributeStoragePolicyID,
			expected: "storagepolicyid",
		},
		{
			name:     "AttributeFsType",
			constant: AttributeFsType,
			expected: "fstype",
		},
		{
			name:     "AttributeStoragePool",
			constant: AttributeStoragePool,
			expected: "storagepool",
		},
		{
			name:     "AttributeHostLocal",
			constant: AttributeHostLocal,
			expected: "hostlocal",
		},
		{
			name:     "AttributePvName",
			constant: AttributePvName,
			expected: "csi.storage.k8s.io/pv/name",
		},
		{
			name:     "AttributePvcName",
			constant: AttributePvcName,
			expected: "csi.storage.k8s.io/pvc/name",
		},
		{
			name:     "AttributePvcNamespace",
			constant: AttributePvcNamespace,
			expected: "csi.storage.k8s.io/pvc/namespace",
		},
		{
			name:     "AttributeStorageClassName",
			constant: AttributeStorageClassName,
			expected: "csi.storage.k8s.io/sc/name",
		},
		{
			name:     "AttributeFirstClassDiskUUID",
			constant: AttributeFirstClassDiskUUID,
			expected: "diskUUID",
		},
		{
			name:     "AttributeVmUUID",
			constant: AttributeVmUUID,
			expected: "vmUUID",
		},
		{
			name:     "AttributeFakeAttached",
			constant: AttributeFakeAttached,
			expected: "fake-attach",
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

func TestFsTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{
			name:     "Ext4FsType",
			constant: Ext4FsType,
			expected: "ext4",
		},
		{
			name:     "Ext3FsType",
			constant: Ext3FsType,
			expected: "ext3",
		},
		{
			name:     "XFSType",
			constant: XFSType,
			expected: "xfs",
		},
		{
			name:     "NfsV4FsType",
			constant: NfsV4FsType,
			expected: "nfs4",
		},
		{
			name:     "NTFSFsType",
			constant: NTFSFsType,
			expected: "ntfs",
		},
		{
			name:     "NfsFsType",
			constant: NfsFsType,
			expected: "nfs",
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

func TestVolumeTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{
			name:     "BlockVolumeType",
			constant: BlockVolumeType,
			expected: "BLOCK",
		},
		{
			name:     "FileVolumeType",
			constant: FileVolumeType,
			expected: "FILE",
		},
		{
			name:     "UnknownVolumeType",
			constant: UnknownVolumeType,
			expected: "UNKNOWN",
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

func TestProviderPrefix(t *testing.T) {
	expected := "vsphere://"
	if ProviderPrefix != expected {
		t.Errorf("ProviderPrefix = %q, expected %q", ProviderPrefix, expected)
	}
}

func TestNfsAccessPointKeys(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{
			name:     "Nfsv3AccessPointKey",
			constant: Nfsv3AccessPointKey,
			expected: "NFSv3",
		},
		{
			name:     "Nfsv4AccessPointKey",
			constant: Nfsv4AccessPointKey,
			expected: "NFSv4.1",
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

func TestLinkedCloneConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{
			name:     "LinkedClonePVCLabel",
			constant: LinkedClonePVCLabel,
			expected: "linked-clone",
		},
		{
			name:     "AttributeIsLinkedClone",
			constant: AttributeIsLinkedClone,
			expected: "csi.vsphere.volume/fast-provisioning",
		},
		{
			name:     "AttributeIsLinkedCloneKey",
			constant: AttributeIsLinkedCloneKey,
			expected: "csi.vsphere.k8s.io/linked-clone",
		},
		{
			name:     "LinkedCloneCountLabel",
			constant: LinkedCloneCountLabel,
			expected: "csi.vsphere.volume/linked-clone-count",
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

func TestHostMoidAnnotationKey(t *testing.T) {
	expected := "vmware-system-esxi-node-moid"
	if HostMoidAnnotationKey != expected {
		t.Errorf("HostMoidAnnotationKey = %q, expected %q", HostMoidAnnotationKey, expected)
	}
}

func TestVersionConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant int
		expected int
	}{
		{
			name:     "MinSupportedVCenterMajor",
			constant: MinSupportedVCenterMajor,
			expected: 6,
		},
		{
			name:     "MinSupportedVCenterMinor",
			constant: MinSupportedVCenterMinor,
			expected: 7,
		},
		{
			name:     "MinSupportedVCenterPatch",
			constant: MinSupportedVCenterPatch,
			expected: 3,
		},
		{
			name:     "SnapshotSupportedVCenterMajor",
			constant: SnapshotSupportedVCenterMajor,
			expected: 7,
		},
		{
			name:     "SnapshotSupportedVCenterMinor",
			constant: SnapshotSupportedVCenterMinor,
			expected: 0,
		},
		{
			name:     "SnapshotSupportedVCenterPatch",
			constant: SnapshotSupportedVCenterPatch,
			expected: 3,
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

func TestDefaultK8sCloudOperatorServicePort(t *testing.T) {
	if defaultK8sCloudOperatorServicePort != 10000 {
		t.Errorf("defaultK8sCloudOperatorServicePort = %d, expected 10000",
			defaultK8sCloudOperatorServicePort)
	}
}

func TestCSINamespace(t *testing.T) {
	// GetCSINamespace should return a non-empty string
	namespace := GetCSINamespace()
	if namespace == "" {
		t.Error("GetCSINamespace() should return a non-empty string")
	}
}

func TestVSphereVersionStrings(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{
			name:     "VSphere67u3Version",
			constant: VSphere67u3Version,
			expected: "6.7.3",
		},
		{
			name:     "VSphere7Version",
			constant: VSphere7Version,
			expected: "7.0.0",
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

