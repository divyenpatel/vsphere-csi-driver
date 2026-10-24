/*
Copyright 2026 The Kubernetes Authors.

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
	"slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// PodVMPlacementGVR identifies the scheduler's placement decision for one Pod.
// It mirrors private.iaas.vmware.com/v1alpha1 PodVMPlacement from the
// Supervisor controller-managers APIs (apis/private/podvm/v1alpha1).
// TODO: The API group is still marked Proposed in the platform design. Replace
// this local mirror with the published types once the CRD merges.
var PodVMPlacementGVR = schema.GroupVersionResource{
	Group:    "private.iaas.vmware.com",
	Version:  "v1alpha1",
	Resource: "podvmplacements",
}

// PodVMPlacementConsumerCSI is the requiredFeatures consumer value naming CSI.
const PodVMPlacementConsumerCSI = "CSI"

// PodVMPlacement records where the scheduler placed a Pod's PodVM. The scheduler
// extender creates it from its Filter step, before VolumeBinding PreBind sets the
// PVC's selected Node, so CSI can read the host before the Pod is bound or its VM exists.
//
// It has the same name and namespace as its Pod and a controller owner reference
// to that Pod. Pod names can be reused and garbage collection is asynchronous, so
// a PodVMPlacement applies to a Pod only when that owner reference carries the
// Pod's UID; a mismatch means the placement for this Pod is not written yet.
//
// CSI reads it on exactly one path: host-local WFFC on a VirtualNode, where the
// Node identifies a cluster rather than a host. Only the fields CSI reads are
// modelled: spec.storage, spec.configSpec and status are deliberately absent, and
// the unstructured conversion ignores them.
type PodVMPlacement struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PodVMPlacementSpec `json:"spec"`
}

// PodVMPlacementSpec is the placement decision. The scheduler owns it, and it is
// rewritten when placement is retried.
type PodVMPlacementSpec struct {
	// NodeName is the Kubernetes Node selected for the Pod, recorded before the
	// Pod is bound to it.
	NodeName string `json:"nodeName,omitempty"`
	// Target is the vCenter, cluster and ESXi host selected for the PodVM.
	Target PodVMPlacementTarget `json:"target"`
	// RequiredFeatures lists features whose data this object carries beyond
	// v1alpha1. CSI supports none, so a feature that names CSI as a consumer
	// means CSI must not act on this object.
	RequiredFeatures []PodVMPlacementFeature `json:"requiredFeatures,omitempty"`
}

// PodVMPlacementTarget scopes the cluster and host MoIDs to one vCenter.
type PodVMPlacementTarget struct {
	// VCenterInstanceUUID is ServiceContent.about.instanceUuid of the vCenter
	// that manages ClusterMoID and HostMoID. NodeMapping publishes bare host
	// MoIDs with no vCenter identity, so this is the only evidence that HostMoID
	// is interpreted in the vCenter CSI resolves against.
	VCenterInstanceUUID string `json:"vCenterInstanceUUID,omitempty"`
	// ClusterMoID is not validated by CSI: NodeMapping has no cluster MoID to
	// compare it with, and host ownership already confines HostMoID to the Node.
	ClusterMoID string `json:"clusterMoID,omitempty"`
	// HostMoID is the selected ESXi host. The virtual node rewrites it after a
	// DRS vMotion, which the platform does not perform for a PodVM that owns a
	// host-local volume, so for host-local provisioning it is the exact host.
	HostMoID string `json:"hostMoID,omitempty"`
}

// PodVMPlacementFeature names a feature whose data a PodVMPlacement carries.
type PodVMPlacementFeature struct {
	Name      string   `json:"name,omitempty"`
	Consumers []string `json:"consumers,omitempty"`
}

// AppliesTo reports whether this placement was written for this Pod incarnation:
// its controller owner reference must name the Pod and carry the Pod's UID.
func (p *PodVMPlacement) AppliesTo(pod *corev1.Pod) bool {
	if p == nil || pod == nil || pod.UID == "" {
		return false
	}
	owner := metav1.GetControllerOfNoCopy(p)
	return owner != nil && owner.Kind == "Pod" && owner.APIVersion == "v1" &&
		owner.Name == pod.Name && owner.UID == pod.UID
}

// UnsupportedCSIFeatures returns the required features that list CSI as a
// consumer. CSI supports none of the features added after v1alpha1, so any
// such entry means CSI must not act on this placement until it is upgraded.
// Features for other consumers do not block CSI.
func (p *PodVMPlacement) UnsupportedCSIFeatures() []string {
	var unsupported []string
	for _, feature := range p.Spec.RequiredFeatures {
		if slices.Contains(feature.Consumers, PodVMPlacementConsumerCSI) {
			unsupported = append(unsupported, feature.Name)
		}
	}
	return unsupported
}
