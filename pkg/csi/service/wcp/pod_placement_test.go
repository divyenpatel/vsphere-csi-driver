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

func TestPodVMPlacementBindingModes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		virtual bool
	}{
		{"esx-node-1", false}, {"arbitrary-cluster-name", true}, {"virtual-node-legacy-name", false},
	} {
		node := tc.name
		for _, scenario := range []string{"ready", "immediate", "missing", "single candidate missing", "stale", "not ready", "wrong pod", "wrong pvc", "wrong node", "wrong host", "source conflict", "ambiguous", "bound"} {
			t.Run(node+"/"+scenario, func(t *testing.T) {
				ctx := context.Background()
				binding := storagev1.VolumeBindingWaitForFirstConsumer
				if scenario == "immediate" {
					binding = storagev1.VolumeBindingImmediate
				}
				sc := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "local"}, VolumeBindingMode: &binding}
				pvc := &v1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "disk", Namespace: "tenant", UID: "pvc-uid",
					Annotations: map[string]string{"volume.kubernetes.io/selected-node": node}}, Spec: v1.PersistentVolumeClaimSpec{StorageClassName: &sc.Name}}
				// The Pod has no NodeName or VM UUID: volume creation must precede VM creation.
				pod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "tenant", UID: "pod-uid"}, Spec: v1.PodSpec{
					Volumes: []v1.Volume{{Name: "disk", VolumeSource: v1.VolumeSource{PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{ClaimName: pvc.Name}}}},
				}}
				placement := &podvmv2.PodPlacement{
					ObjectMeta: metav1.ObjectMeta{Name: string(pod.UID), Namespace: pod.Namespace, Generation: 1},
					Spec:       podvmv2.PodPlacementSpec{PodName: pod.Name, PodUID: pod.UID, NodeName: node, PVCUIDs: map[string]types.UID{pvc.Name: pvc.UID}},
					Status:     podvmv2.PodPlacementStatus{ObservedGeneration: 1, HostMoID: "host-1", Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}}},
				}
				candidates := []vimtypes.ManagedObjectReference{{Type: "HostSystem", Value: "host-1"}, {Type: "HostSystem", Value: "host-2"}}
				switch scenario {
				case "single candidate missing":
					candidates = candidates[:1]
				case "stale":
					placement.Status.ObservedGeneration = 0
				case "not ready":
					placement.Status.Conditions = nil
				case "wrong pod":
					placement.Spec.PodUID = "old-pod"
				case "wrong pvc":
					placement.Spec.PVCUIDs[pvc.Name] = "old-pvc"
				case "wrong node":
					placement.Spec.NodeName = "other-node"
				case "wrong host":
					placement.Status.HostMoID = "host-9"
				case "source conflict":
					candidates = candidates[1:]
				case "bound":
					pvc.Spec.VolumeName = "retained-pv"
				}
				selected := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: node}}
				if tc.virtual {
					selected.Annotations = map[string]string{podvmv2.NodeTypeAnnotation: podvmv2.VirtualNodeType}
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
					_, err := client.CoreV1().PersistentVolumes().Create(ctx, &v1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: pvc.Spec.VolumeName,
						Annotations: map[string]string{common.AnnPodVMHostPinned: "host-1"}}}, metav1.CreateOptions{})
					require.NoError(t, err)
				}
				reads := 0
				reader := func(_ context.Context, ns, name string) (*podvmv2.PodPlacement, error) {
					reads++
					require.Equal(t, pod.Namespace, ns)
					require.Equal(t, string(pod.UID), name)
					if scenario == "missing" || scenario == "single candidate missing" {
						return nil, fmt.Errorf("placement has not been created")
					}
					return placement, nil
				}
				result, err := constrainPodVMPlacement(ctx, client, reader, pvc.Namespace, pvc.Name, candidates,
					ownersForPlacementTest(node, tc.virtual))
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
					require.Zero(t, reads)
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

func TestDedicatedNodePlacementDuringUpgrade(t *testing.T) {
	for _, scenario := range []string{"all legacy", "mixed", "unknown node", "joined virtual node", "ambiguous owner", "source conflict", "missing node", "unknown type", "deleting node"} {
		t.Run(scenario, func(t *testing.T) {
			binding := storagev1.VolumeBindingWaitForFirstConsumer
			sc := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "local"}, VolumeBindingMode: &binding}
			pvc := &v1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "disk", Namespace: "tenant",
				Annotations: map[string]string{"volume.kubernetes.io/selected-node": "esx-node-1"}},
				Spec: v1.PersistentVolumeClaimSpec{StorageClassName: &sc.Name}}
			// No Pod or placement CR is needed to resolve a dedicated Node's host.
			node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "esx-node-1"}}
			if scenario == "unknown type" {
				node.Annotations = map[string]string{podvmv2.NodeTypeAnnotation: "entity-group"}
			}
			if scenario == "deleting node" {
				now := metav1.Now()
				node.DeletionTimestamp = &now
			}
			client := fake.NewSimpleClientset(sc, pvc, node)
			if scenario == "missing node" {
				require.NoError(t, client.CoreV1().Nodes().Delete(context.Background(), node.Name, metav1.DeleteOptions{}))
			}
			owners := map[string]string{"host-1": "esx-node-1", "host-2": "esx-node-2"}
			candidates := []vimtypes.ManagedObjectReference{{Type: "HostSystem", Value: "host-1"}, {Type: "HostSystem", Value: "host-2"}}
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
			reader := func(context.Context, string, string) (*podvmv2.PodPlacement, error) {
				t.Fatal("dedicated Node provisioning must not read a placement CR")
				return nil, nil
			}
			result, err := constrainPodVMPlacement(context.Background(), client, reader, pvc.Namespace, pvc.Name, candidates, owners)
			if scenario == "all legacy" || scenario == "mixed" {
				require.NoError(t, err)
				require.Equal(t, candidates[:1], result)
			} else {
				require.Error(t, err)
			}
		})
	}
}
