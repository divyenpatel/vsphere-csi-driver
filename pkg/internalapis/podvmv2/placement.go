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

// TODO: Replace with the scheduling owner's published placement API and lifecycle.
var PodPlacementGVR = schema.GroupVersionResource{Group: "virtualnode.vmware.com", Version: "v1alpha1", Resource: "podvmplacements"}

// PodPlacement is a provisional namespaced resource named after the Pod UID.
// CSI requires it for direct host-local WFFC consumers on VirtualNodes. A
// dedicated ESXi Node already identifies the physical host without this CR.
// Ready means the physical host decision is committed for this Pod incarnation
// and all listed PVC incarnations, before volume provisioning or VM creation.
// The producer must preserve that commitment while provisioning is in progress.
// TODO: Confirm naming, authorization, scheduling-attempt fencing and cleanup.
type PodPlacement struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PodPlacementSpec   `json:"spec"`
	Status            PodPlacementStatus `json:"status,omitempty"`
}

type PodPlacementSpec struct {
	PodName  string               `json:"podName"`
	PodUID   types.UID            `json:"podUID"`
	NodeName string               `json:"nodeName"`
	PVCUIDs  map[string]types.UID `json:"pvcUIDs"`
}

type PodPlacementStatus struct {
	ObservedGeneration int64              `json:"observedGeneration"`
	HostMoID           string             `json:"hostMoID"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}
