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

	vimtypes "github.com/vmware/govmomi/vim25/types"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/csi/service/common"
	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/csi/service/common/commonco"
	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/internalapis/podvmv2"
)

type placementReader func(context.Context, string, string) (*podvmv2.PodPlacement, error)

func readPodPlacement(ctx context.Context, namespace, name string) (*podvmv2.PodPlacement, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}
	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	obj, err := client.Resource(podvmv2.PodPlacementGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	var result podvmv2.PodPlacement
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// constrainPodVMPlacement runs only for an enabled host-local PodVM v2 request.
// Binding mode comes from the Supervisor StorageClass, never candidate count.
// Guest requests use an Immediate Supervisor StorageClass and do not need this CR.
// Dedicated ESXi Nodes already identify one physical host and do not need a CR either.
func constrainPodVMPlacement(ctx context.Context, client kubernetes.Interface, read placementReader,
	namespace, pvcName string, candidates []vimtypes.ManagedObjectReference,
	owners map[string]string) ([]vimtypes.ManagedObjectReference, error) {
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
		return nil, fmt.Errorf("selected Node %q has unsupported node type %q", selectedNode, node.Annotations[podvmv2.NodeTypeAnnotation])
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
	placement, err := read(ctx, namespace, string(consumer.UID))
	if err != nil {
		return nil, err
	}
	if placement == nil || placement.DeletionTimestamp != nil || placement.Namespace != namespace ||
		placement.Spec.PodName != consumer.Name || placement.Spec.PodUID != consumer.UID ||
		placement.Spec.NodeName != selectedNode || pvc.UID == "" || placement.Spec.PVCUIDs[pvcName] != pvc.UID ||
		placement.Generation == 0 || placement.Status.ObservedGeneration != placement.Generation ||
		!meta.IsStatusConditionTrue(placement.Status.Conditions, "Ready") {
		return nil, fmt.Errorf("Pod placement is incomplete or stale for PVC %q", pvcName)
	}
	host := placement.Status.HostMoID
	if host == "" || owners[host] != selectedNode {
		return nil, fmt.Errorf("placement host is not owned by selected Node %q", selectedNode)
	}
	return constrainToPlacementHost(candidates, host)
}

func constrainToPlacementHost(candidates []vimtypes.ManagedObjectReference, host string) ([]vimtypes.ManagedObjectReference, error) {
	for _, candidate := range candidates {
		if candidate.Value == host {
			return []vimtypes.ManagedObjectReference{candidate}, nil
		}
	}
	return nil, fmt.Errorf("placement host %q conflicts with volume topology or source constraints", host)
}

func applyPodVMPlacement(ctx context.Context, namespace, pvcName string,
	candidates []vimtypes.ManagedObjectReference) ([]vimtypes.ManagedObjectReference, error) {
	client, err := newK8sClient(ctx)
	if err != nil {
		return nil, err
	}
	return constrainPodVMPlacement(ctx, client, readPodPlacement, namespace, pvcName, candidates,
		commonco.ContainerOrchestratorUtility.GetNodeIDtoNameMap(ctx))
}
