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

package wcp

import (
	"context"
	"fmt"
	"strings"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/csi/service/common"
	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/csi/service/common/commonco"
	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/internalapis/podvmv2"
)

type placementReader func(context.Context, string, string) (*podvmv2.PodVMPlacement, error)

// ownershipReader resolves current host ownership (host MoID -> owning Node name). It is a
// function so the Node and NodeMapping lists behind it are issued only on the paths that need
// them: Immediate, bound and shared requests never pay for, or fail on, an ownership snapshot.
type ownershipReader func(context.Context) (map[string]string, error)

func readPodPlacement(ctx context.Context, namespace, name string) (*podvmv2.PodVMPlacement, error) {
	client, err := placementDynamicClient()
	if err != nil {
		return nil, err
	}
	obj, err := client.Resource(podvmv2.PodVMPlacementGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	var result podvmv2.PodVMPlacement
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func placementDynamicClient() (dynamic.Interface, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}
	return dynamic.NewForConfig(cfg)
}

// constrainPodVMPlacement runs only for an enabled host-local PodVM v2 request.
// Binding mode comes from the Supervisor StorageClass, never candidate count.
// Guest requests use an Immediate Supervisor StorageClass and do not need this CR.
// Dedicated ESXi Nodes already identify one physical host and do not need a CR either.
//
// vcUUID is the instance UUID of the vCenter this Supervisor is backed by. NodeMapping publishes
// bare host MoIDs without a vCenter, so the placement's vcID is the only evidence that its host
// MoID is interpreted in the same vCenter as CSI's inventory.
func constrainPodVMPlacement(ctx context.Context, client kubernetes.Interface, read placementReader,
	resolveOwners ownershipReader, vcUUID, namespace, pvcName string,
	candidates []vimtypes.ManagedObjectReference) ([]vimtypes.ManagedObjectReference, error) {
	if namespace == "" || pvcName == "" {
		return nil, fmt.Errorf("PVC identity is required for PodVM v2 host-local provisioning")
	}
	pvc, err := client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, pvcName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if pvc.Spec.VolumeName != "" {
		pv, err := client.CoreV1().PersistentVolumes().Get(ctx, pvc.Spec.VolumeName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		host := pv.Annotations[common.AnnPodVMHostPinned]
		if host == "" {
			return nil, fmt.Errorf("bound host-local PV %q has no host pin", pv.Name)
		}
		return constrainToPlacementHost(candidates, host)
	}
	if pvc.Spec.StorageClassName == nil {
		return nil, fmt.Errorf("PVC has no StorageClass")
	}
	sc, err := client.StorageV1().StorageClasses().Get(ctx, *pvc.Spec.StorageClassName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if sc.VolumeBindingMode == nil || *sc.VolumeBindingMode == storagev1.VolumeBindingImmediate {
		return candidates, nil
	}
	if *sc.VolumeBindingMode != storagev1.VolumeBindingWaitForFirstConsumer {
		return nil, fmt.Errorf("unsupported binding mode")
	}
	selectedNode := pvc.Annotations["volume.kubernetes.io/selected-node"]
	if selectedNode == "" {
		return nil, fmt.Errorf("WFFC PVC has no selected Node yet")
	}
	node, err := client.CoreV1().Nodes().Get(ctx, selectedNode, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if node.DeletionTimestamp != nil {
		return nil, fmt.Errorf("selected Node %q is being deleted", selectedNode)
	}
	if !podvmv2.IsVirtualNode(node) && !podvmv2.IsDedicatedHostNode(node) {
		return nil, fmt.Errorf("selected Node %q has unsupported node provider %q",
			selectedNode, podvmv2.NodeTypeOf(node))
	}
	owners, err := resolveOwners(ctx)
	if err != nil {
		// An incomplete view cannot tell "no owner" from "not resolved": retry, never guess.
		return nil, fmt.Errorf("cannot resolve physical host ownership: %w", err)
	}
	if podvmv2.IsDedicatedHostNode(node) {
		// Enabling the capability does not upgrade the selected host. Resolve the
		// dedicated Node from current ownership, never from candidate count: a
		// source constraint or stale topology may have narrowed to a different host.
		var host string
		for id, owner := range owners {
			if owner != selectedNode {
				continue
			}
			if host != "" {
				return nil, fmt.Errorf("dedicated Node %q maps to multiple physical hosts", selectedNode)
			}
			host = id
		}
		if host == "" {
			return nil, fmt.Errorf("selected dedicated Node %q has no current physical host", selectedNode)
		}
		return constrainToPlacementHost(candidates, host)
	}
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var consumer *v1.Pod
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.DeletionTimestamp != nil || pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			continue
		}
		for _, volume := range pod.Spec.Volumes {
			if volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.ClaimName == pvcName {
				if consumer != nil && consumer.UID != pod.UID {
					return nil, fmt.Errorf("ambiguous WFFC consumers for PVC %q", pvcName)
				}
				consumer = pod
			}
		}
	}
	if consumer == nil || consumer.UID == "" {
		return nil, fmt.Errorf("WFFC consumer is not available yet")
	}
	if consumer.Spec.NodeName != "" && consumer.Spec.NodeName != selectedNode {
		return nil, fmt.Errorf("consumer Node conflicts with selected Node")
	}
	return constrainToCommittedHost(ctx, read, vcUUID, node, consumer, pvcName, candidates, owners)
}

// constrainToCommittedHost validates the scheduler's published decision for a
// VirtualNode. The Node identifies a cluster, so only the scheduler knows the
// committed host before VM creation. Nothing here may widen the candidate set.
func constrainToCommittedHost(ctx context.Context, read placementReader, vcUUID string,
	node *v1.Node, consumer *v1.Pod, pvcName string, candidates []vimtypes.ManagedObjectReference,
	owners map[string]string) ([]vimtypes.ManagedObjectReference, error) {
	selectedNode := node.Name
	// The placement has the Pod's name and namespace. The scheduler extender creates
	// it from its Filter step, before VolumeBinding PreBind sets selected-node, so a missing object
	// is a retry, not a reason to choose a host. Status is never read: the virtual
	// node writes it after the PodVM exists, and provisioning must not wait on it.
	placement, err := read(ctx, consumer.Namespace, consumer.Name)
	if err != nil {
		return nil, err
	}
	if placement == nil || placement.DeletionTimestamp != nil || placement.Namespace != consumer.Namespace {
		return nil, fmt.Errorf("placement for Pod %s/%s is not available", consumer.Namespace, consumer.Name)
	}
	// Pod names are reused and garbage collection is asynchronous, so a placement
	// left by an earlier Pod of the same name can still exist. Treat it as not
	// written yet for this Pod.
	if !placement.AppliesTo(consumer) {
		return nil, fmt.Errorf("placement %s/%s is not owned by Pod UID %q; waiting for the scheduler",
			placement.Namespace, placement.Name, consumer.UID)
	}
	// A feature that names CSI as a consumer carries data CSI would silently miss
	// by reading through v1alpha1. Never act on it.
	if unsupported := placement.UnsupportedCSIFeatures(); len(unsupported) > 0 {
		return nil, fmt.Errorf("placement %s/%s requires features this CSI release does not support: %v",
			placement.Namespace, placement.Name, unsupported)
	}
	if placement.Spec.NodeName != selectedNode {
		return nil, fmt.Errorf("placement targets Node %q, not the selected Node %q",
			placement.Spec.NodeName, selectedNode)
	}
	target := placement.Spec.Target
	// NodeMapping carries no vCenter identity, so this is the only per-placement
	// guard that the host MoID is interpreted in the vCenter CSI resolves against.
	if target.VCenterInstanceUUID == "" || vcUUID == "" || !strings.EqualFold(target.VCenterInstanceUUID, vcUUID) {
		return nil, fmt.Errorf("placement vCenter %q is not the configured vCenter %q",
			target.VCenterInstanceUUID, vcUUID)
	}
	host := target.HostMoID
	if host == "" {
		return nil, fmt.Errorf("placement for PVC %q has no host", pvcName)
	}
	// Cross-check the scheduler against the NodeMapping producer. A VirtualNode
	// owns a host in the ownership snapshot only when its validated, Ready
	// NodeMapping lists that host and no other Node contests it, so the two
	// independent writers must agree. target.clusterMoID is not compared:
	// NodeMapping has no cluster MoID, and this check already confines the host to
	// the selected VirtualNode.
	if owners[host] != selectedNode {
		return nil, fmt.Errorf("placement host %q is not published as owned by selected Node %q", host, selectedNode)
	}
	return constrainToPlacementHost(candidates, host)
}

func constrainToPlacementHost(candidates []vimtypes.ManagedObjectReference,
	host string) ([]vimtypes.ManagedObjectReference, error) {
	for _, candidate := range candidates {
		if candidate.Value == host {
			return []vimtypes.ManagedObjectReference{candidate}, nil
		}
	}
	return nil, fmt.Errorf("placement host %q conflicts with volume topology or source constraints", host)
}

func applyPodVMPlacement(ctx context.Context, vcUUID, namespace, pvcName string,
	candidates []vimtypes.ManagedObjectReference) ([]vimtypes.ManagedObjectReference, error) {
	client, err := newK8sClient(ctx)
	if err != nil {
		return nil, err
	}
	return constrainPodVMPlacement(ctx, client, readPodPlacement,
		commonco.ContainerOrchestratorUtility.GetNodeIDtoNameMap, vcUUID, namespace, pvcName, candidates)
}
