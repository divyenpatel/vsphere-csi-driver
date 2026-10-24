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

package cnsregistervolume

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/csi/service/common"
)

func TestRegistrationHostMetadata(t *testing.T) {
	for _, node := range []string{"esx-node", "virtual-node-domain-c1"} {
		topology, err := registeredHostTopology([]map[string]string{{v1.LabelTopologyZone: "zone-1", v1.LabelHostname: node}}, "host-45")
		require.NoError(t, err)
		require.Equal(t, []map[string]string{{v1.LabelTopologyZone: "zone-1"}}, topology)
		pv := &v1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "pv"}, Spec: v1.PersistentVolumeSpec{
			NodeAffinity: buildNodeAffinityFromSegments(topology), PersistentVolumeSource: v1.PersistentVolumeSource{CSI: &v1.CSIPersistentVolumeSource{VolumeHandle: "volume"}},
		}}
		setRegisteredPVHost(pv, "host-45")
		require.Equal(t, "host-45", pv.Spec.CSI.VolumeAttributes[common.AttributeESXiHostMoID])
		require.Equal(t, "host-45", pv.Annotations[common.AnnPodVMHostPinned])
		require.Equal(t, buildNodeAffinityFromSegments([]map[string]string{{v1.LabelTopologyZone: "zone-1"}}), pv.Spec.NodeAffinity)
		pvc := &v1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "replacement", Namespace: "tenant", Annotations: map[string]string{"keep": "value"}}}
		client := fake.NewSimpleClientset(pv, pvc)
		updated, err := publishRegisteredPVCHost(context.Background(), client, pvc, "host-45", topology)
		require.NoError(t, err)
		require.Equal(t, "host-45", updated.Annotations[common.AttributeESXiHostMoID])
		require.NotEmpty(t, updated.Annotations[common.AnnVolumeAccessibleTopology])
		require.Equal(t, "value", updated.Annotations["keep"])
		guest, err := common.GuestAccessibleTopology(updated)
		require.NoError(t, err)
		require.Equal(t, "host-45", guest[0][common.GuestClusterTopologyLabelHost])
		client.ClearActions()
		_, err = publishRegisteredPVCHost(context.Background(), client, updated, "host-45", topology)
		require.NoError(t, err)
		require.Empty(t, client.Actions())
		_, err = publishRegisteredPVCHost(context.Background(), client, updated, "host-46", topology)
		require.Error(t, err)
	}
}

func TestRegistrationReusedPVPreservesImmutableFields(t *testing.T) {
	original := &v1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "retained"}, Spec: v1.PersistentVolumeSpec{
		NodeAffinity:           buildNodeAffinityFromSegments([]map[string]string{{v1.LabelTopologyZone: "zone-1", v1.LabelHostname: "old-esx-node"}}),
		PersistentVolumeSource: v1.PersistentVolumeSource{CSI: &v1.CSIPersistentVolumeSource{VolumeHandle: "volume"}},
	}}
	client := fake.NewSimpleClientset(original)
	updated, err := reconcileRegisteredPVHost(context.Background(), client, original, "host-45")
	require.NoError(t, err)
	require.Equal(t, original.Spec, updated.Spec)
	require.Equal(t, "host-45", updated.Annotations[common.AnnPodVMHostPinned])
	_, err = reconcileRegisteredPVHost(context.Background(), client, updated, "host-46")
	require.Error(t, err)
	client.ClearActions()
	_, err = reconcileRegisteredPVHost(context.Background(), client, updated, "host-45")
	require.NoError(t, err)
	require.Empty(t, client.Actions())
}

func TestRegistrationSharedVolumeUnpinned(t *testing.T) {
	pv := &v1.PersistentVolume{}
	setRegisteredPVHost(pv, "")
	require.Empty(t, pv.Annotations)
	require.Nil(t, pv.Spec.CSI)
	_, err := registeredHostTopology(nil, "host-45")
	require.Error(t, err)
}
