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

package podvmv2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// PodVMPlacementGVR identifies the scheduler's committed Pod-to-host decision.
// TODO: Replace with the scheduling owner's published API once it ships.
var PodVMPlacementGVR = schema.GroupVersionResource{
	Group:    "scheduling.podvm.vmware.com",
	Version:  "v1alpha1",
	Resource: "podvmplacements",
}

// PodVMPlacementStatePrepared means the scheduler has published its selected
// placement. The Pod may still be unbound and the PodVM may not exist yet.
const PodVMPlacementStatePrepared = "Prepared"

// PlacementName derives the stable object name from the Pod UID.
func PlacementName(podUID types.UID) string {
	return "podvm-" + string(podUID)
}

// PodVMPlacement carries the scheduler's committed physical placement for one
// Pod incarnation. CSI reads it on exactly one path: host-local WFFC on a
// VirtualNode, where the Node identifies a cluster rather than a single host.
// Dedicated ESXi Nodes, shared zonal volumes and Immediate requests resolve
// without it, so a placement producer outage cannot stall all Supervisor WFFC.
//
// The scheduler owns spec; the node routine owns status. Status is written
// after power-on and is absent when CSI provisions WFFC storage, so it is
// deliberately not modelled here: provisioning must never wait on it.
// TODO: Confirm authorization, scheduling-attempt fencing and cleanup.
type PodVMPlacement struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PodVMPlacementSpec `json:"spec"`
}

// PodVMPlacementSpec identifies the Pod and its published placement.
type PodVMPlacementSpec struct {
	PodName  string                  `json:"podName,omitempty"`
	PodUID   types.UID               `json:"podUID,omitempty"`
	State    string                  `json:"state,omitempty"`
	Prepared *PodVMPlacementPrepared `json:"prepared,omitempty"`
}

// PodVMPlacementPrepared is the committed decision. Host, cluster and VM MoIDs
// are interpreted within the vCenter named by VCID.
type PodVMPlacementPrepared struct {
	NodeName string    `json:"nodeName,omitempty"`
	NodeUID  types.UID `json:"nodeUID,omitempty"`

	// VCID identifies the owning vCenter.
	// TODO: The platform currently shows an opaque handle such as "vc-a", while
	// HostMapping.spec.vCenterUUID is a strict instance UUID. CSI must join the
	// two resources to confirm the committed host is one the selected
	// VirtualNode owns, so the scheduling owner has been asked to publish this
	// as the same vCenter instance UUID. Until then this check fails closed.
	VCID string `json:"vcID,omitempty"`

	Cluster string `json:"cluster,omitempty"`
	Host    string `json:"host,omitempty"`

	// AllowedHosts is the set permitted by the selected Node scope. ExactHost
	// reports whether creation and power-on must use Host. Host-local volumes
	// cannot tolerate ExactHost=false: the volume lives on one host's
	// exclusively mounted datastore, so power-on elsewhere in AllowedHosts
	// leaves it unattachable and violates the PV pin before first use.
	AllowedHosts []string `json:"allowedHosts,omitempty"`
	ExactHost    bool     `json:"exactHost,omitempty"`

	NativeCommitments *PodVMNativeCommitments `json:"nativeCommitments,omitempty"`
}

// PodVMNativeCommitments records the volumes this placement accounted for.
type PodVMNativeCommitments struct {
	Volumes []PodVMPlacementVolume `json:"volumes,omitempty"`
}

// PodVMPlacementVolume identifies one PVC covered by the placement.
// RequestedBytes is advisory: expansion legitimately diverges from it.
// DeviceKey links the request to a placeholder disk and is not interpreted.
type PodVMPlacementVolume struct {
	PVCName        string    `json:"pvcName,omitempty"`
	PVCUID         types.UID `json:"pvcUID,omitempty"`
	StorageClass   string    `json:"storageClass,omitempty"`
	RequestedBytes int64     `json:"requestedBytes,omitempty"`
	Dynamic        bool      `json:"dynamic,omitempty"`
	DeviceKey      int32     `json:"deviceKey,omitempty"`
}

// CoversPVC reports whether the placement accounted for this exact PVC
// incarnation. The UID match prevents a delete and recreate of a same-named
// PVC from binding against a placement computed for its predecessor.
func (p *PodVMPlacementPrepared) CoversPVC(name string, uid types.UID) bool {
	if p == nil || p.NativeCommitments == nil {
		return false
	}
	for _, volume := range p.NativeCommitments.Volumes {
		if volume.PVCName == name && uid != "" && volume.PVCUID == uid {
			return true
		}
	}
	return false
}

// DeclaresVolumes reports whether the placement published a volume list at all.
// TODO: The section is currently optional, so its absence is ambiguous between
// "the scheduler did not constrain placement on this PVC" and "the producer
// omitted the section". CSI can therefore only validate it opportunistically.
// The platform has been asked to make it required whenever a host-local volume
// influenced placement, after which absence becomes a hard failure.
func (p *PodVMPlacementPrepared) DeclaresVolumes() bool {
	return p != nil && p.NativeCommitments != nil && len(p.NativeCommitments.Volumes) > 0
}
