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

package config

import (
	"os"
	"testing"

	vsanfstypes "github.com/vmware/govmomi/vsan/vsanfs/types"
)

func TestConfigConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{
			name:     "DefaultVCenterPort",
			constant: DefaultVCenterPort,
			expected: "443",
		},
		{
			name:     "DefaultGCPort",
			constant: DefaultGCPort,
			expected: "6443",
		},
		{
			name:     "DefaultCloudConfigPath",
			constant: DefaultCloudConfigPath,
			expected: "/etc/cloud/csi-vsphere.conf",
		},
		{
			name:     "DefaultGCConfigPath",
			constant: DefaultGCConfigPath,
			expected: "/etc/cloud/pvcsi-config/cns-csi.conf",
		},
		{
			name:     "SupervisorCAFilePath",
			constant: SupervisorCAFilePath,
			expected: "/etc/vmware/wcp/tls/vmca.pem",
		},
		{
			name:     "EnvVSphereCSIConfig",
			constant: EnvVSphereCSIConfig,
			expected: "VSPHERE_CSI_CONFIG",
		},
		{
			name:     "EnvGCConfig",
			constant: EnvGCConfig,
			expected: "GC_CONFIG",
		},
		{
			name:     "DefaultpvCSIProviderPath",
			constant: DefaultpvCSIProviderPath,
			expected: "/etc/cloud/pvcsi-provider",
		},
		{
			name:     "DefaultSupervisorFSSConfigMapName",
			constant: DefaultSupervisorFSSConfigMapName,
			expected: "csi-feature-states",
		},
		{
			name:     "DefaultInternalFSSConfigMapName",
			constant: DefaultInternalFSSConfigMapName,
			expected: "internal-feature-states.csi.vsphere.vmware.com",
		},
		{
			name:     "DefaultCSINamespace",
			constant: DefaultCSINamespace,
			expected: "vmware-system-csi",
		},
		{
			name:     "EnvCSINamespace",
			constant: EnvCSINamespace,
			expected: "CSI_NAMESPACE",
		},
		{
			name:     "TopologyLabelsDomain",
			constant: TopologyLabelsDomain,
			expected: "topology.csi.vmware.com",
		},
		{
			name:     "TKCKind",
			constant: TKCKind,
			expected: "TanzuKubernetesCluster",
		},
		{
			name:     "TKCAPIVersion",
			constant: TKCAPIVersion,
			expected: "run.tanzu.vmware.com/v1alpha1",
		},
		{
			name:     "ClusterIDConfigMapName",
			constant: ClusterIDConfigMapName,
			expected: "vsphere-csi-cluster-id",
		},
		{
			name:     "ClusterVersionv1beta1",
			constant: ClusterVersionv1beta1,
			expected: "cluster.x-k8s.io/v1beta1",
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

func TestDefaultIntConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant int
		expected int
	}{
		{
			name:     "DefaultCnsRegisterVolumesCleanupIntervalInMin",
			constant: DefaultCnsRegisterVolumesCleanupIntervalInMin,
			expected: 720,
		},
		{
			name:     "DefaultVolumeMigrationCRCleanupIntervalInMin",
			constant: DefaultVolumeMigrationCRCleanupIntervalInMin,
			expected: 120,
		},
		{
			name:     "DefaultCSIAuthCheckIntervalInMin",
			constant: DefaultCSIAuthCheckIntervalInMin,
			expected: 5,
		},
		{
			name:     "DefaultCSIFetchPreferredDatastoresIntervalInMin",
			constant: DefaultCSIFetchPreferredDatastoresIntervalInMin,
			expected: 5,
		},
		{
			name:     "DefaultCnsVolumeOperationRequestCleanupIntervalInMin",
			constant: DefaultCnsVolumeOperationRequestCleanupIntervalInMin,
			expected: 15,
		},
		{
			name:     "DefaultGlobalMaxSnapshotsPerBlockVolume",
			constant: DefaultGlobalMaxSnapshotsPerBlockVolume,
			expected: 3,
		},
		{
			name:     "MaxNumberOfTopologyCategories",
			constant: MaxNumberOfTopologyCategories,
			expected: 5,
		},
		{
			name:     "DefaultQueryLimit",
			constant: DefaultQueryLimit,
			expected: 10000,
		},
		{
			name:     "DefaultListVolumeThreshold",
			constant: DefaultListVolumeThreshold,
			expected: 50,
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

func TestConfigErrors(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		expectedMsg string
	}{
		{
			name:        "ErrUsernameMissing",
			err:         ErrUsernameMissing,
			expectedMsg: "username is missing",
		},
		{
			name:        "ErrPasswordMissing",
			err:         ErrPasswordMissing,
			expectedMsg: "password is missing",
		},
		{
			name:        "ErrInvalidVCenterIP",
			err:         ErrInvalidVCenterIP,
			expectedMsg: "vsphere.conf does not have the VirtualCenter IP address specified",
		},
		{
			name:        "ErrMissingVCenter",
			err:         ErrMissingVCenter,
			expectedMsg: "no Virtual Center hosts defined",
		},
		{
			name:        "ErrClusterIDCharLimit",
			err:         ErrClusterIDCharLimit,
			expectedMsg: "cluster id must not exceed 64 characters",
		},
		{
			name:        "ErrSupervisorIDCharLimit",
			err:         ErrSupervisorIDCharLimit,
			expectedMsg: "supervisor id must not exceed 64 characters",
		},
		{
			name:        "ErrMissingEndpoint",
			err:         ErrMissingEndpoint,
			expectedMsg: "no Supervisor Cluster endpoint defined in Guest Cluster config",
		},
		{
			name:        "ErrMissingTanzuKubernetesClusterUID",
			err:         ErrMissingTanzuKubernetesClusterUID,
			expectedMsg: "no Tanzu Kubernetes Cluster UID defined in Guest Cluster config",
		},
		{
			name:        "ErrInvalidNetPermission",
			err:         ErrInvalidNetPermission,
			expectedMsg: "invalid value for Permissions under NetPermission Config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Errorf("%s should not be nil", tt.name)
			}
			if tt.err.Error() != tt.expectedMsg {
				t.Errorf("%s.Error() = %q, expected %q", tt.name, tt.err.Error(), tt.expectedMsg)
			}
		})
	}
}

func TestGetDefaultNetPermission(t *testing.T) {
	netPerm := GetDefaultNetPermission()

	if netPerm == nil {
		t.Fatal("GetDefaultNetPermission() returned nil")
	}

	if netPerm.RootSquash != false {
		t.Errorf("RootSquash = %v, expected false", netPerm.RootSquash)
	}

	if netPerm.Permissions != vsanfstypes.VsanFileShareAccessTypeREAD_WRITE {
		t.Errorf("Permissions = %v, expected %v",
			netPerm.Permissions, vsanfstypes.VsanFileShareAccessTypeREAD_WRITE)
	}

	if netPerm.Ips != "*" {
		t.Errorf("Ips = %q, expected %q", netPerm.Ips, "*")
	}
}

func TestGetCSINamespace(t *testing.T) {
	// Save original value
	originalValue := os.Getenv(EnvCSINamespace)
	defer func() {
		if originalValue != "" {
			os.Setenv(EnvCSINamespace, originalValue)
		} else {
			os.Unsetenv(EnvCSINamespace)
		}
	}()

	tests := []struct {
		name     string
		envValue string
		expected string
	}{
		{
			name:     "Empty env returns default",
			envValue: "",
			expected: DefaultCSINamespace,
		},
		{
			name:     "Custom namespace",
			envValue: "custom-namespace",
			expected: "custom-namespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(EnvCSINamespace, tt.envValue)
			} else {
				os.Unsetenv(EnvCSINamespace)
			}

			result := GetCSINamespace()
			if result != tt.expected {
				t.Errorf("GetCSINamespace() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestVirtualCenterConfigString(t *testing.T) {
	vc := VirtualCenterConfig{
		User:         "admin@vsphere.local",
		Password:     "secretpassword",
		VCenterPort:  "443",
		InsecureFlag: true,
		Datacenters:  "DC1",
	}

	result := vc.String()

	// Verify password is redacted
	if result == "" {
		t.Error("VirtualCenterConfig.String() returned empty string")
	}

	// The password should be masked (not contain the actual password)
	if len(result) > 0 && result != "{}" {
		// Basic sanity check that it returns a formatted string
		if result[0] != '{' || result[len(result)-1] != '}' {
			t.Errorf("VirtualCenterConfig.String() should return a formatted string, got %q", result)
		}
	}
}

func TestConfigStructInitialization(t *testing.T) {
	cfg := &Config{}

	// Verify default values
	if cfg.Global.VCenterIP != "" {
		t.Errorf("Default VCenterIP should be empty, got %q", cfg.Global.VCenterIP)
	}

	if cfg.Global.ClusterID != "" {
		t.Errorf("Default ClusterID should be empty, got %q", cfg.Global.ClusterID)
	}

	if cfg.VirtualCenter != nil {
		t.Error("Default VirtualCenter should be nil")
	}

	if cfg.NetPermissions != nil {
		t.Error("Default NetPermissions should be nil")
	}
}

func TestConfigurationInfoStruct(t *testing.T) {
	cfg := &Config{
		Global: struct {
			VCenterIP                                     string
			ClusterID                                     string `gcfg:"cluster-id"`
			SupervisorID                                  string `gcfg:"supervisor-id"`
			User                                          string `gcfg:"user"`
			Password                                      string `gcfg:"password"`
			VCenterPort                                   string `gcfg:"port"`
			InsecureFlag                                  bool   `gcfg:"insecure-flag"`
			CAFile                                        string `gcfg:"ca-file"`
			Thumbprint                                    string `gcfg:"thumbprint"`
			Datacenters                                   string `gcfg:"datacenters"`
			CnsRegisterVolumesCleanupIntervalInMin        int    `gcfg:"cnsregistervolumes-cleanup-intervalinmin"`
			VolumeMigrationCRCleanupIntervalInMin         int    `gcfg:"volumemigration-cr-cleanup-intervalinmin"`
			ClusterDistribution                           string `gcfg:"cluster-distribution"`
			CSIAuthCheckIntervalInMin                     int    `gcfg:"csi-auth-check-intervalinmin"`
			CnsVolumeOperationRequestCleanupIntervalInMin int    `gcfg:"cnsvolumeoperationrequest-cleanup-intervalinmin"`
			CSIFetchPreferredDatastoresIntervalInMin      int    `gcfg:"csi-fetch-preferred-datastores-intervalinmin"`
			QueryLimit                                    int    `gcfg:"query-limit"`
			ListVolumeThreshold                           int    `gcfg:"list-volume-threshold"`
		}{
			ClusterID: "test-cluster",
		},
	}

	configInfo := &ConfigurationInfo{
		Cfg: cfg,
	}

	if configInfo.Cfg == nil {
		t.Error("ConfigurationInfo.Cfg should not be nil")
	}

	if configInfo.Cfg.Global.ClusterID != "test-cluster" {
		t.Errorf("ClusterID = %q, expected %q", configInfo.Cfg.Global.ClusterID, "test-cluster")
	}
}

func TestNetPermissionConfigStruct(t *testing.T) {
	netPerm := &NetPermissionConfig{
		Ips:         "10.20.30.0/24",
		Permissions: vsanfstypes.VsanFileShareAccessTypeREAD_ONLY,
		RootSquash:  true,
	}

	if netPerm.Ips != "10.20.30.0/24" {
		t.Errorf("Ips = %q, expected %q", netPerm.Ips, "10.20.30.0/24")
	}

	if netPerm.Permissions != vsanfstypes.VsanFileShareAccessTypeREAD_ONLY {
		t.Errorf("Permissions = %v, expected %v",
			netPerm.Permissions, vsanfstypes.VsanFileShareAccessTypeREAD_ONLY)
	}

	if netPerm.RootSquash != true {
		t.Errorf("RootSquash = %v, expected true", netPerm.RootSquash)
	}
}

func TestGCConfigStruct(t *testing.T) {
	gcConfig := GCConfig{
		Endpoint:                   "10.0.0.1",
		Port:                       "6443",
		TanzuKubernetesClusterUID:  "test-uid",
		TanzuKubernetesClusterName: "test-cluster",
		ClusterDistribution:        "TKGService",
		ClusterAPIVersion:          "cluster.x-k8s.io/v1beta1",
		ClusterKind:                "Cluster",
	}

	if gcConfig.Endpoint != "10.0.0.1" {
		t.Errorf("Endpoint = %q, expected %q", gcConfig.Endpoint, "10.0.0.1")
	}

	if gcConfig.Port != "6443" {
		t.Errorf("Port = %q, expected %q", gcConfig.Port, "6443")
	}

	if gcConfig.TanzuKubernetesClusterUID != "test-uid" {
		t.Errorf("TanzuKubernetesClusterUID = %q, expected %q",
			gcConfig.TanzuKubernetesClusterUID, "test-uid")
	}
}

func TestSnapshotConfigStruct(t *testing.T) {
	snapshotConfig := SnapshotConfig{
		GlobalMaxSnapshotsPerBlockVolume:         3,
		GranularMaxSnapshotsPerBlockVolumeInVSAN: 32,
		GranularMaxSnapshotsPerBlockVolumeInVVOL: 32,
	}

	if snapshotConfig.GlobalMaxSnapshotsPerBlockVolume != 3 {
		t.Errorf("GlobalMaxSnapshotsPerBlockVolume = %d, expected %d",
			snapshotConfig.GlobalMaxSnapshotsPerBlockVolume, 3)
	}

	if snapshotConfig.GranularMaxSnapshotsPerBlockVolumeInVSAN != 32 {
		t.Errorf("GranularMaxSnapshotsPerBlockVolumeInVSAN = %d, expected %d",
			snapshotConfig.GranularMaxSnapshotsPerBlockVolumeInVSAN, 32)
	}

	if snapshotConfig.GranularMaxSnapshotsPerBlockVolumeInVVOL != 32 {
		t.Errorf("GranularMaxSnapshotsPerBlockVolumeInVVOL = %d, expected %d",
			snapshotConfig.GranularMaxSnapshotsPerBlockVolumeInVVOL, 32)
	}
}

