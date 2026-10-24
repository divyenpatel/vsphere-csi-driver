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
	"testing"

	"github.com/stretchr/testify/require"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/csi/service/common"
	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/internalapis/podvmv2"
)

const testPlacementVCUUID = "42305b1e-8f1a-4a3b-9c2d-1234567890ab"

// ownersFrom returns an ownership reader serving a fixed snapshot and counting reads.
func ownersFrom(owners map[string]string, err error, reads *int) ownershipReader {
	return func(context.Context) (map[string]string, error) {
		*reads++
		return owners, err
	}
}

// testPlacement builds the PodVMPlacement the scheduler's PreBind plugin writes for pod.
func testPlacement(pod *v1.Pod, node, host string) *podvmv2.PodVMPlacement {
	controller := true
	return &podvmv2.PodVMPlacement{
		ObjectMeta: metav1.ObjectMeta{
			Name: pod.Name, Namespace: pod.Namespace, Generation: 1,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "v1", Kind: "Pod", Name: pod.Name, UID: pod.UID, Controller: &controller,
			}},
		},
		Spec: podvmv2.PodVMPlacementSpec{
			NodeName: node,
			Target: podvmv2.PodVMPlacementTarget{
				VCenterInstanceUUID: testPlacementVCUUID, ClusterMoID: "domain-c1", HostMoID: host,
			},
		},
	}
}

func TestPodVMPlacementBindingModes(t *testing.T) {
	scenarios := []string{
		"ready", "immediate", "missing", "single candidate missing", "deleting", "stale owner", "no owner",
		"not controller", "wrong node", "wrong host", "no host", "host not owned", "host held by dedicated node",
		"cluster not compared", "wrong vcenter", "vcenter unknown", "vcenter case", "ownership unresolved",
		"csi feature", "other consumer feature", "source conflict", "ambiguous", "bound",
	}
	for _, tc := range []struct {
		name    string
		virtual bool
	}{
		{"esx-node-1", false}, {"arbitrary-cluster-name", true}, {"virtual-node-legacy-name", false},
	} {
		node := tc.name
		for _, scenario := range scenarios {
			t.Run(node+"/"+scenario, func(t *testing.T) {
				ctx := context.Background()
				binding := storagev1.VolumeBindingWaitForFirstConsumer
				if scenario == "immediate" {
					binding = storagev1.VolumeBindingImmediate
				}
				sc := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "local"}, VolumeBindingMode: &binding}
				pvc := &v1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
					Name: "disk", Namespace: "tenant", UID: "pvc-uid",
					Annotations: map[string]string{"volume.kubernetes.io/selected-node": node}},
					Spec: v1.PersistentVolumeClaimSpec{StorageClassName: &sc.Name}}
				// The Pod has no NodeName or VM UUID: volume creation must precede VM creation.
				pod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "tenant", UID: "pod-uid"},
					Spec: v1.PodSpec{Volumes: []v1.Volume{{Name: "disk", VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{ClaimName: "disk"}}}}}}
				selected := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: node, UID: types.UID(node + "-uid")}}
				if tc.virtual {
					selected.Annotations = map[string]string{podvmv2.NodeTypeAnnotation: podvmv2.VirtualNodeType}
				}
				owners := ownersForPlacementTest(node, tc.virtual)
				var ownersErr error
				vcUUID := testPlacementVCUUID
				placement := testPlacement(pod, node, "host-1")
				candidates := []vimtypes.ManagedObjectReference{
					{Type: "HostSystem", Value: "host-1"}, {Type: "HostSystem", Value: "host-2"}}
				switch scenario {
				case "single candidate missing":
					candidates = candidates[:1]
				case "deleting":
					now := metav1.Now()
					placement.DeletionTimestamp = &now
				case "stale owner":
					// Left by an earlier Pod with the same name; garbage collection has not run yet.
					placement.OwnerReferences[0].UID = "old-pod-uid"
				case "no owner":
					placement.OwnerReferences = nil
				case "not controller":
					placement.OwnerReferences[0].Controller = nil
				case "wrong node":
					placement.Spec.NodeName = "other-node"
				case "wrong host":
					placement.Spec.Target.HostMoID = "host-9"
				case "no host":
					placement.Spec.Target.HostMoID = ""
				case "host not owned":
					// Not listed by the VirtualNode's NodeMapping, so the snapshot has no owner.
					delete(owners, "host-1")
				case "host held by dedicated node":
					// The snapshot keeps a dedicated Node as owner when a NodeMapping overlaps it.
					owners["host-1"] = "esx-node-9"
				case "cluster not compared":
					placement.Spec.Target.ClusterMoID = "domain-c99"
				case "wrong vcenter":
					placement.Spec.Target.VCenterInstanceUUID = "11111111-2222-3333-4444-555555555555"
				case "vcenter unknown":
					vcUUID = ""
				case "vcenter case":
					placement.Spec.Target.VCenterInstanceUUID = "42305B1E-8F1A-4A3B-9C2D-1234567890AB"
				case "ownership unresolved":
					ownersErr = fmt.Errorf("cannot list NodeMapping resources")
				case "csi feature":
					placement.Spec.RequiredFeatures = []podvmv2.PodVMPlacementFeature{
						{Name: "PerVolumeStorage", Consumers: []string{podvmv2.PodVMPlacementConsumerCSI}}}
				case "other consumer feature":
					// A feature only the virtual node consumes does not block CSI.
					placement.Spec.RequiredFeatures = []podvmv2.PodVMPlacementFeature{
						{Name: "MultipleNetworkInterfaces", Consumers: []string{"VirtualNode"}}}
				case "source conflict":
					candidates = candidates[1:]
				case "bound":
					pvc.Spec.VolumeName = "retained-pv"
				}
				client := fake.NewSimpleClientset(sc, pvc, pod, selected)
				if scenario == "ambiguous" {
					second := pod.DeepCopy()
					second.Name = "other"
					second.UID = "other-uid"
					_, err := client.CoreV1().Pods(pod.Namespace).Create(ctx, second, metav1.CreateOptions{})
					require.NoError(t, err)
				}
				if scenario == "bound" {
					_, err := client.CoreV1().PersistentVolumes().Create(ctx, &v1.PersistentVolume{
						ObjectMeta: metav1.ObjectMeta{Name: pvc.Spec.VolumeName,
							Annotations: map[string]string{common.AnnPodVMHostPinned: "host-1"}}},
						metav1.CreateOptions{})
					require.NoError(t, err)
				}
				reads, ownerReads := 0, 0
				reader := func(_ context.Context, ns, name string) (*podvmv2.PodVMPlacement, error) {
					reads++
					// The placement has the same name and namespace as its Pod.
					require.Equal(t, pod.Namespace, ns)
					require.Equal(t, pod.Name, name)
					if scenario == "missing" || scenario == "single candidate missing" {
						return nil, fmt.Errorf("placement has not been created")
					}
					return placement, nil
				}
				result, err := constrainPodVMPlacement(ctx, client, reader, ownersFrom(owners, ownersErr, &ownerReads),
					vcUUID, pvc.Namespace, pvc.Name, candidates)
				// The ownership mutations also take host-1 away from a dedicated Node, which then has no host.
				dedicatedFails := scenario == "source conflict" || scenario == "ownership unresolved" ||
					scenario == "host not owned" || scenario == "host held by dedicated node"
				succeeds := scenario == "ready" || scenario == "bound" || scenario == "cluster not compared" ||
					scenario == "vcenter case" || scenario == "other consumer feature"
				switch {
				case scenario == "immediate":
					require.NoError(t, err)
					require.Equal(t, candidates, result)
				case succeeds || (!tc.virtual && !dedicatedFails):
					require.NoError(t, err)
					require.Equal(t, candidates[:1], result)
				default:
					require.Error(t, err)
				}
				if scenario == "immediate" || scenario == "bound" || !tc.virtual {
					require.Zero(t, reads, "only host-local WFFC on a VirtualNode reads the placement CR")
				}
				if scenario == "immediate" || scenario == "bound" {
					require.Zero(t, ownerReads, "Immediate and bound requests must not resolve host ownership")
				}
			})
		}
	}
}

func ownersForPlacementTest(node string, virtual bool) map[string]string {
	if virtual {
		return map[string]string{"host-1": node, "host-2": node}
	}
	return map[string]string{"host-1": node, "host-2": "esx-node-2"}
}

// TestPodVMPlacementDecodesReviewedSchema decodes an object shaped like the platform's
// private.iaas.vmware.com/v1alpha1 example. Fields CSI does not model, such as
// spec.configSpec, spec.storage and status, must not break the conversion.
func TestPodVMPlacementDecodesReviewedSchema(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "private.iaas.vmware.com/v1alpha1",
		"kind":       "PodVMPlacement",
		"metadata": map[string]interface{}{
			"name": "app", "namespace": "tenant",
			"ownerReferences": []interface{}{map[string]interface{}{
				"apiVersion": "v1", "kind": "Pod", "name": "app", "uid": "pod-uid", "controller": true,
			}},
		},
		"spec": map[string]interface{}{
			"nodeName": "virtual-node-domain-c52",
			"target": map[string]interface{}{
				"vCenterInstanceUUID": testPlacementVCUUID, "clusterMoID": "domain-c52", "hostMoID": "host-21",
			},
			"storage":    map[string]interface{}{"storagePolicyID": "aa6d5a82-1c88-45da-85d3-3d74b91a5bad"},
			"configSpec": map[string]interface{}{"_typeName": "VirtualMachineConfigSpec", "version": "vmx-23"},
			"requiredFeatures": []interface{}{map[string]interface{}{
				"name": "MultipleNetworkInterfaces", "consumers": []interface{}{"VirtualNode"},
			}},
		},
		"status": map[string]interface{}{
			"vm": map[string]interface{}{"instanceUUID": "5012abcd-0000-0000-0000-000000000000", "moID": "vm-1042"},
		},
	}}
	var placement podvmv2.PodVMPlacement
	require.NoError(t, runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &placement))
	require.Equal(t, "virtual-node-domain-c52", placement.Spec.NodeName)
	require.Equal(t, podvmv2.PodVMPlacementTarget{
		VCenterInstanceUUID: testPlacementVCUUID, ClusterMoID: "domain-c52", HostMoID: "host-21",
	}, placement.Spec.Target)
	pod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "tenant", UID: "pod-uid"}}
	require.True(t, placement.AppliesTo(pod))
	require.Empty(t, placement.UnsupportedCSIFeatures())
	pod.UID = "recreated-pod-uid"
	require.False(t, placement.AppliesTo(pod), "a recreated Pod with the same name must not use the old placement")
}

func TestDedicatedNodePlacementDuringUpgrade(t *testing.T) {
	for _, scenario := range []string{"all legacy", "mixed", "unknown node", "joined virtual node",
		"ambiguous owner", "source conflict", "missing node", "unknown type", "deleting node"} {
		t.Run(scenario, func(t *testing.T) {
			binding := storagev1.VolumeBindingWaitForFirstConsumer
			sc := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "local"}, VolumeBindingMode: &binding}
			pvc := &v1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "disk", Namespace: "tenant",
				Annotations: map[string]string{"volume.kubernetes.io/selected-node": "esx-node-1"}},
				Spec: v1.PersistentVolumeClaimSpec{StorageClassName: &sc.Name}}
			// No Pod or placement CR is needed for a dedicated Node.
			node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "esx-node-1", UID: "esx-node-1-uid"}}
			if scenario == "unknown type" {
				node.Annotations = map[string]string{podvmv2.NodeTypeAnnotation: "entity-group"}
			}
			if scenario == "deleting node" {
				now := metav1.Now()
				node.DeletionTimestamp = &now
			}
			client := fake.NewSimpleClientset(sc, pvc, node)
			if scenario == "missing node" {
				require.NoError(t, client.CoreV1().Nodes().Delete(context.Background(), node.Name,
					metav1.DeleteOptions{}))
			}
			owners := map[string]string{"host-1": "esx-node-1", "host-2": "esx-node-2"}
			candidates := []vimtypes.ManagedObjectReference{
				{Type: "HostSystem", Value: "host-1"}, {Type: "HostSystem", Value: "host-2"}}
			switch scenario {
			case "mixed":
				owners["host-2"] = "virtual-node-domain-c1"
			case "unknown node":
				delete(owners, "host-1")
			case "joined virtual node":
				owners["host-1"] = "virtual-node-domain-c1"
			case "ambiguous owner":
				owners["host-2"] = "esx-node-1"
			case "source conflict":
				candidates = candidates[1:]
			}
			reader := func(context.Context, string, string) (*podvmv2.PodVMPlacement, error) {
				t.Fatal("dedicated Node provisioning must not read a placement CR")
				return nil, nil
			}
			// The dedicated path never consults vcID, so an empty configured UUID is irrelevant here.
			result, err := constrainPodVMPlacement(context.Background(), client, reader,
				ownersFrom(owners, nil, new(int)), "", pvc.Namespace, pvc.Name, candidates)
			if scenario == "all legacy" || scenario == "mixed" {
				require.NoError(t, err)
				require.Equal(t, candidates[:1], result)
			} else {
				require.Error(t, err)
			}
		})
	}
}
