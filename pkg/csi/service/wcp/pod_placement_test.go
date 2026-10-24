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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/csi/service/common"
	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/internalapis/podvmv2"
)

const testPlacementVCUUID = "42305b1e-8f1a-4a3b-9c2d-1234567890ab"

func placementHostMapping(node *v1.Node, hosts ...string) *podvmv2.HostMapping {
	controller := true
	mapping := &podvmv2.HostMapping{
		ObjectMeta: metav1.ObjectMeta{
			Name: node.Name, Generation: 1,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "v1", Kind: "Node", Name: node.Name, UID: node.UID, Controller: &controller,
			}},
		},
		Spec: podvmv2.HostMappingSpec{
			VCenterUUID: testPlacementVCUUID, ComputeClusterMOID: "domain-c1",
		},
		Status: podvmv2.HostMappingStatus{Conditions: []metav1.Condition{{
			Type: podvmv2.HostMappingConditionReady, Status: metav1.ConditionTrue,
			ObservedGeneration: 1, Reason: "Synced", LastTransitionTime: metav1.Now(),
		}}},
	}
	for _, host := range hosts {
		mapping.Spec.Hosts = append(mapping.Spec.Hosts, podvmv2.HostMappingHost{MOID: host})
	}
	return mapping
}

func TestPodVMPlacementBindingModes(t *testing.T) {
	scenarios := []string{
		"ready", "immediate", "missing", "single candidate missing", "not prepared", "no prepared section",
		"wrong pod", "wrong pvc", "wrong node", "wrong node uid", "wrong host", "host not owned",
		"inexact host", "wrong cluster", "wrong vcenter", "source conflict", "ambiguous", "bound",
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
				mapping := placementHostMapping(selected, "host-1", "host-2")
				// The committed host lives in spec: status is written only after
				// power-on and is absent while CSI provisions.
				placement := &podvmv2.PodVMPlacement{
					ObjectMeta: metav1.ObjectMeta{
						Name: podvmv2.PlacementName(pod.UID), Namespace: pod.Namespace, Generation: 1},
					Spec: podvmv2.PodVMPlacementSpec{
						PodName: pod.Name, PodUID: pod.UID, State: podvmv2.PodVMPlacementStatePrepared,
						Prepared: &podvmv2.PodVMPlacementPrepared{
							NodeName: node, NodeUID: selected.UID, VCID: testPlacementVCUUID,
							Cluster: "domain-c1", Host: "host-1",
							AllowedHosts: []string{"host-1", "host-2"}, ExactHost: true,
							NativeCommitments: &podvmv2.PodVMNativeCommitments{
								Volumes: []podvmv2.PodVMPlacementVolume{{
									PVCName: pvc.Name, PVCUID: pvc.UID, StorageClass: sc.Name, Dynamic: true}}},
						},
					},
				}
				candidates := []vimtypes.ManagedObjectReference{
					{Type: "HostSystem", Value: "host-1"}, {Type: "HostSystem", Value: "host-2"}}
				switch scenario {
				case "single candidate missing":
					candidates = candidates[:1]
				case "not prepared":
					placement.Spec.State = "Pending"
				case "no prepared section":
					placement.Spec.Prepared = nil
				case "wrong pod":
					placement.Spec.PodUID = "old-pod"
				case "wrong pvc":
					placement.Spec.Prepared.NativeCommitments.Volumes[0].PVCUID = "old-pvc"
				case "wrong node":
					placement.Spec.Prepared.NodeName = "other-node"
				case "wrong node uid":
					placement.Spec.Prepared.NodeUID = "replaced-node-uid"
				case "wrong host":
					placement.Spec.Prepared.Host = "host-9"
				case "host not owned":
					mapping = placementHostMapping(selected, "host-2")
				case "inexact host":
					placement.Spec.Prepared.ExactHost = false
				case "wrong cluster":
					placement.Spec.Prepared.Cluster = "domain-c99"
				case "wrong vcenter":
					placement.Spec.Prepared.VCID = "vc-a"
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
				reads, mappingReads := 0, 0
				reader := func(_ context.Context, ns, name string) (*podvmv2.PodVMPlacement, error) {
					reads++
					require.Equal(t, pod.Namespace, ns)
					require.Equal(t, "podvm-"+string(pod.UID), name)
					if scenario == "missing" || scenario == "single candidate missing" {
						return nil, fmt.Errorf("placement has not been created")
					}
					return placement, nil
				}
				mappingReader := func(_ context.Context, name string) (*podvmv2.HostMapping, error) {
					mappingReads++
					require.Equal(t, node, name)
					return mapping, nil
				}
				result, err := constrainPodVMPlacement(ctx, client, reader, mappingReader,
					pvc.Namespace, pvc.Name, candidates, ownersForPlacementTest(node, tc.virtual))
				if scenario == "ready" || scenario == "bound" ||
					(!tc.virtual && scenario != "source conflict" && scenario != "immediate") {
					require.NoError(t, err)
					require.Equal(t, candidates[:1], result)
				} else if scenario == "immediate" {
					require.NoError(t, err)
					require.Equal(t, candidates, result)
				} else {
					require.Error(t, err)
				}
				if scenario == "immediate" || scenario == "bound" || !tc.virtual {
					require.Zero(t, reads, "only host-local WFFC on a VirtualNode reads the placement CR")
					require.Zero(t, mappingReads)
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

// TestPlacementToleratesAbsentVolumeList covers the section still being
// optional. Once the platform makes it required for host-local placements,
// absence must become a hard failure instead.
func TestPlacementToleratesAbsentVolumeList(t *testing.T) {
	ctx := context.Background()
	binding := storagev1.VolumeBindingWaitForFirstConsumer
	sc := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "local"}, VolumeBindingMode: &binding}
	pvc := &v1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
		Name: "disk", Namespace: "tenant", UID: "pvc-uid",
		Annotations: map[string]string{"volume.kubernetes.io/selected-node": "compute-a"}},
		Spec: v1.PersistentVolumeClaimSpec{StorageClassName: &sc.Name}}
	pod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "tenant", UID: "pod-uid"},
		Spec: v1.PodSpec{Volumes: []v1.Volume{{Name: "disk", VolumeSource: v1.VolumeSource{
			PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{ClaimName: "disk"}}}}}}
	selected := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "compute-a", UID: "compute-a-uid",
		Annotations: map[string]string{podvmv2.NodeTypeAnnotation: podvmv2.VirtualNodeType}}}
	mapping := placementHostMapping(selected, "host-1")
	placement := &podvmv2.PodVMPlacement{
		ObjectMeta: metav1.ObjectMeta{Name: podvmv2.PlacementName(pod.UID), Namespace: pod.Namespace},
		Spec: podvmv2.PodVMPlacementSpec{
			PodName: pod.Name, PodUID: pod.UID, State: podvmv2.PodVMPlacementStatePrepared,
			Prepared: &podvmv2.PodVMPlacementPrepared{
				NodeName: selected.Name, NodeUID: selected.UID, VCID: testPlacementVCUUID,
				Cluster: "domain-c1", Host: "host-1", ExactHost: true,
			},
		},
	}
	candidates := []vimtypes.ManagedObjectReference{{Type: "HostSystem", Value: "host-1"}}
	result, err := constrainPodVMPlacement(ctx, fake.NewSimpleClientset(sc, pvc, pod, selected),
		func(context.Context, string, string) (*podvmv2.PodVMPlacement, error) { return placement, nil },
		func(context.Context, string) (*podvmv2.HostMapping, error) { return mapping, nil },
		pvc.Namespace, pvc.Name, candidates, map[string]string{"host-1": selected.Name})
	require.NoError(t, err)
	require.Equal(t, candidates, result)
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
			// No Pod, placement CR or HostMapping is needed for a dedicated Node.
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
			mappingReader := func(context.Context, string) (*podvmv2.HostMapping, error) {
				t.Fatal("dedicated Node provisioning must not read a HostMapping")
				return nil, nil
			}
			result, err := constrainPodVMPlacement(context.Background(), client, reader, mappingReader,
				pvc.Namespace, pvc.Name, candidates, owners)
			if scenario == "all legacy" || scenario == "mixed" {
				require.NoError(t, err)
				require.Equal(t, candidates[:1], result)
			} else {
				require.Error(t, err)
			}
		})
	}
}
