/*
Copyright 2020 The Kubernetes Authors.

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

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CnsRegisterVolumeSpec defines the desired state of CnsRegisterVolume
// +k8s:openapi-gen=true
type CnsRegisterVolumeSpec struct {
	// Name of the PVC
	PvcName string `json:"pvcName"`

	// VolumeID indicates an existing vsphere volume to be imported into Project
	// Pacific cluster.
	// If the AccessMode is "ReadWriteMany" or "ReadOnlyMany", then this VolumeID
	// can be either an existing FileShare (or) CNS file volume backed FileShare.
	// If the AccessMode is "ReadWriteOnce", then this VolumeID can be either an
	// existing FCD (or) a CNS backed FCD.
	// VolumeID and DiskUrlPath cannot be specified together.
	VolumeID string `json:"volumeID,omitempty"`

	// AccessMode contains the actual access mode the volume backing the
	// CnsRegisterVolume has.
	// AccessMode must be specified if VolumeID is specified.
	AccessMode v1.PersistentVolumeAccessMode `json:"accessMode,omitempty"`

	// DiskUrlPath is URL path to an existing block volume to be imported into
	// Project Pacific cluster.
	// VolumeID and DiskUrlPath cannot be specified together.
	// DiskUrlPath is explicitly used for block volumes. AccessMode need not be
	// specified and will be defaulted to "ReadWriteOnce".
	// This field must be in the following format:
	// Format:
	// https://<vc_ip>/folder/<vm_vmdk_path>?dcPath=<datacenterName>&dsName=<datastoreName>
	// Ex: https://10.192.255.221/folder/34a9c05d-5f03-e254-e692-02004479cb91/
	//        vm2_1.vmdk?dcPath=Datacenter-1&dsName=vsanDatastore
	// This is for a 34a9c05d-5f03-e254-e692-02004479cb91/vm2_1.vmdk
	// file under datacenter "Datacenter-1" and datastore "vsanDatastore".
	DiskURLPath string `json:"diskURLPath,omitempty"`

	// VolumeMode can either be Block (for raw block volume) or
	// Filesystem. Default values is Filesystem.
	VolumeMode v1.PersistentVolumeMode `json:"volumeMode,omitempty"`

	// BackingType enumerates types of backing for batch attach operations.
	// The values correspond to the cns.VolumeBackingType enum.
	// Valid values: FlatVer1BackingInfo, FlatVer2BackingInfo, SparseVer1BackingInfo,
	// SparseVer2BackingInfo, RawDiskMappingVer1BackingInfo, SeSparseBackingInfo,
	// LocalPMemBackingInfo, or empty string.
	BackingType string `json:"backingType,omitempty"`

	// StorageClassName is the name of the Kubernetes StorageClass whose associated
	// vSphere storage policy should be assigned to the volume being registered.
	// This is optional. If set, the storage policy is applied unconditionally,
	// regardless of whether the volume already has a storage policy associated
	// with it in CNS.
	StorageClassName string `json:"storageClassName,omitempty"`
}

// CnsRegisterVolumePhase is the lifecycle phase of a CnsRegisterVolume.
//
// Phase is additive, observational state: it reports where in the registration
// flow the CR currently is. Registered remains the completion signal that
// callers (e.g. VM Operator, and the VKSRegisterVolume controller's Supervisor
// watch) key off; Phase must not be used to infer completion.
type CnsRegisterVolumePhase string

const (
	// CnsRegisterVolumePhasePending is the initial phase: the CR has been
	// accepted and its spec is being validated, before any CNS call is made.
	CnsRegisterVolumePhasePending CnsRegisterVolumePhase = "Pending"
	// CnsRegisterVolumePhaseCnsRegistering means the CNS CreateVolume call for
	// this CR is in progress.
	CnsRegisterVolumePhaseCnsRegistering CnsRegisterVolumePhase = "CnsRegistering"
	// CnsRegisterVolumePhaseConfiguringK8s means the CNS volume exists and the
	// Kubernetes objects for it (PV, PVC, topology and volumeMode fixes) are
	// being created or reconciled.
	CnsRegisterVolumePhaseConfiguringK8s CnsRegisterVolumePhase = "ConfiguringK8s"
	// CnsRegisterVolumePhaseWaitingForBind means the PV and PVC exist and the
	// controller is waiting for the PVC to reach Bound.
	CnsRegisterVolumePhaseWaitingForBind CnsRegisterVolumePhase = "WaitingForBind"
	// CnsRegisterVolumePhaseFinalizing means the PVC is Bound and the remaining
	// bookkeeping (CnsVolumeInfo, storage quota accounting) is being applied.
	CnsRegisterVolumePhaseFinalizing CnsRegisterVolumePhase = "Finalizing"
	// CnsRegisterVolumePhaseRegistered is the terminal success phase. It is set
	// at the same moment as Registered=true.
	CnsRegisterVolumePhaseRegistered CnsRegisterVolumePhase = "Registered"
	// CnsRegisterVolumePhaseFailed reports a failure the controller does not
	// retry on its own, such as a volume that is not accessible to the cluster
	// or has no storage policy. The CNS volume has been cleaned up. It is set
	// only on paths that already stopped requeuing before this field existed,
	// so it reports existing behaviour rather than introducing new behaviour.
	CnsRegisterVolumePhaseFailed CnsRegisterVolumePhase = "Failed"
)

// CnsRegisterVolumeStatus defines the observed state of CnsRegisterVolume
// +k8s:openapi-gen=true
type CnsRegisterVolumeStatus struct {
	// Indicates the volume is successfully registered.
	// This field must only be set by the entity completing the register
	// operation, i.e. the CNS Operator.
	Registered bool `json:"registered"`

	// The last error encountered during import operation, if any.
	// This field must only be set by the entity completing the import
	// operation, i.e. the CNS Operator.
	Error string `json:"error,omitempty"`

	// Phase is the current lifecycle phase of the registration. It is
	// informational: Registered, not Phase, is the completion signal.
	// Absent on CRs last written by a controller that predates this field.
	// +optional
	Phase CnsRegisterVolumePhase `json:"phase,omitempty"`

	// CnsTaskID is the vCenter managed object ID of the in-flight CNS
	// CreateVolume task for this CR, while one is running.
	//
	// It exists because the controller submits the task and returns rather than
	// holding a worker until CNS responds, so the task reference has to survive
	// across reconciles. There is no other durable copy: this controller writes
	// no CnsVolumeOperationRequest, and the volume manager skips its in-memory
	// task map for static provisioning specs.
	//
	// Set immediately after a successful submit and cleared once the task has
	// reached a terminal state. A cleared or failed task ID is never reused.
	// +optional
	CnsTaskID string `json:"cnsTaskID,omitempty"`

	// LastUpdateTime is when this status was last published, i.e. the time of
	// the most recent write that actually changed the status. A reconcile that
	// leaves the status unchanged does not move it, so the age of this field is
	// how long the CR has sat in its current state -- which is the only way to
	// tell a registration that is progressing from one that has stalled.
	//
	// This is a publish time, not an event time: the state it describes was
	// reached at some point before the write that recorded it.
	//
	// Absent on CRs last written by a controller that predates this field.
	// Consumers must treat an absent value as unknown and fall back to
	// metadata.creationTimestamp, never as the zero time, which would make every
	// such CR look arbitrarily old.
	// +optional
	LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CnsRegisterVolume is the Schema for the cnsregistervolumes API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="PVCName",type=string,JSONPath=`.spec.pvcName`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Registered",type=boolean,JSONPath=`.status.registered`
// +kubebuilder:printcolumn:name="LastUpdate",type=date,JSONPath=`.status.lastUpdateTime`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type CnsRegisterVolume struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CnsRegisterVolumeSpec   `json:"spec,omitempty"`
	Status CnsRegisterVolumeStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CnsRegisterVolumeList contains a list of CnsRegisterVolume
type CnsRegisterVolumeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CnsRegisterVolume `json:"items"`
}
