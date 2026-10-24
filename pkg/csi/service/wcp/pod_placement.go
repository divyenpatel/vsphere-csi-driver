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
	"slices"
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

type hostMappingReader func(context.Context, string) (*podvmv2.HostMapping, error)

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

func readHostMapping(ctx context.Context, name string) (*podvmv2.HostMapping, error) {
	client, err := placementDynamicClient()
	if err != nil {
		return nil, err
	}
	return podvmv2.GetHostMapping(ctx, client, name)
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
func constrainPodVMPlacement(ctx context.Context, client kubernetes.Interface, read placementReader,
	readMapping hostMappingReader, namespace, pvcName string, candidates []vimtypes.ManagedObjectReference,
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
		return nil, fmt.Errorf("selected Node %q has unsupported node type %q",
			selectedNode, node.Annotations[podvmv2.NodeTypeAnnotation])
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
	return constrainToCommittedHost(ctx, read, readMapping, node, consumer, pvc, pvcName, candidates, owners)
}

// constrainToCommittedHost validates the scheduler's published decision for a
// VirtualNode. The Node identifies a cluster, so only the scheduler knows the
// committed host before VM creation. Nothing here may widen the candidate set.
func constrainToCommittedHost(ctx context.Context, read placementReader, readMapping hostMappingReader,
	node *v1.Node, consumer *v1.Pod, pvc *v1.PersistentVolumeClaim, pvcName string,
	candidates []vimtypes.ManagedObjectReference,
	owners map[string]string) ([]vimtypes.ManagedObjectReference, error) {
	selectedNode := node.Name
	placement, err := read(ctx, consumer.Namespace, podvmv2.PlacementName(consumer.UID))
	if err != nil {
		return nil, err
	}
	// The committed host lives in spec, not status: the scheduler owns spec and
	// publishes before VolumeBinding.PreBind, while status is written by the
	// node routine after power-on. Provisioning must never wait on status.
	if placement == nil || placement.DeletionTimestamp != nil || placement.Namespace != consumer.Namespace ||
		placement.Spec.PodName != consumer.Name || placement.Spec.PodUID != consumer.UID ||
		placement.Spec.State != podvmv2.PodVMPlacementStatePrepared || placement.Spec.Prepared == nil {
		return nil, fmt.Errorf("Pod placement is incomplete or not yet prepared for PVC %q", pvcName)
	}
	prepared := placement.Spec.Prepared
	if prepared.NodeName != selectedNode || node.UID == "" || prepared.NodeUID != node.UID {
		return nil, fmt.Errorf("placement targets Node %q/%q, not the selected Node %q/%q",
			prepared.NodeName, prepared.NodeUID, selectedNode, node.UID)
	}
	if pvc.UID == "" {
		return nil, fmt.Errorf("PVC %q has no UID", pvcName)
	}
	// A host-local volume lives on one host's exclusively mounted datastore. If
	// the PodVM may power on at another member of allowedHosts, the volume is
	// unattachable and the PV pin is violated before first use.
	if !prepared.ExactHost {
		return nil, fmt.Errorf("placement for PVC %q does not pin an exact host; "+
			"host-local provisioning cannot tolerate power-on on another allowed host", pvcName)
	}
	// Cross-check the scheduler against the HostMapping producer. These are two
	// independent writers, so the committed host must satisfy both.
	mapping, err := readMapping(ctx, selectedNode)
	if err != nil {
		return nil, err
	}
	if mapping == nil || mapping.DeletionTimestamp != nil {
		return nil, fmt.Errorf("no usable HostMapping for selected Node %q", selectedNode)
	}
	if prepared.Cluster == "" || prepared.Cluster != mapping.Spec.ComputeClusterMOID {
		return nil, fmt.Errorf("placement cluster %q does not match the cluster %q backing Node %q",
			prepared.Cluster, mapping.Spec.ComputeClusterMOID, selectedNode)
	}
	// TODO: The platform currently publishes an opaque vcID such as "vc-a"
	// while HostMapping carries an instance UUID. The scheduling owner has been
	// asked to align them; until then this comparison fails closed rather than
	// trusting host as a bare MoID.
	if prepared.VCID == "" || !strings.EqualFold(prepared.VCID, mapping.Spec.VCenterUUID) {
		return nil, fmt.Errorf("placement vCenter %q does not match the vCenter %q owning Node %q",
			prepared.VCID, mapping.Spec.VCenterUUID, selectedNode)
	}
	host := prepared.Host
	if host == "" {
		return nil, fmt.Errorf("placement for PVC %q has no committed host", pvcName)
	}
	if !slices.Contains(mapping.HostMoIDs(), host) {
		return nil, fmt.Errorf("placement host %q is not published as owned by Node %q", host, selectedNode)
	}
	// owners reflects the validated ownership snapshot, including conflicts
	// withheld there. Requiring both keeps a disagreement fail-closed.
	if owners[host] != selectedNode {
		return nil, fmt.Errorf("placement host %q is not owned by selected Node %q", host, selectedNode)
	}
	// An absent volume list is tolerated only because the section is still
	// optional; once the platform makes it mandatory for host-local placements,
	// absence becomes a hard failure. See DeclaresVolumes.
	if prepared.DeclaresVolumes() && !prepared.CoversPVC(pvcName, pvc.UID) {
		return nil, fmt.Errorf("placement does not account for PVC %q with UID %q", pvcName, pvc.UID)
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

func applyPodVMPlacement(ctx context.Context, namespace, pvcName string,
	candidates []vimtypes.ManagedObjectReference) ([]vimtypes.ManagedObjectReference, error) {
	client, err := newK8sClient(ctx)
	if err != nil {
		return nil, err
	}
	return constrainPodVMPlacement(ctx, client, readPodPlacement, readHostMapping, namespace, pvcName, candidates,
		commonco.ContainerOrchestratorUtility.GetNodeIDtoNameMap(ctx))
}
