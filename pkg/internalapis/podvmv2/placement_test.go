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
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodVMPlacementAppliesTo(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "tenant", UID: "pod-uid"}}
	controller, notController := true, false
	owner := func(mutate func(*metav1.OwnerReference)) *PodVMPlacement {
		ref := metav1.OwnerReference{APIVersion: "v1", Kind: "Pod", Name: "app", UID: "pod-uid", Controller: &controller}
		if mutate != nil {
			mutate(&ref)
		}
		return &PodVMPlacement{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "tenant",
			OwnerReferences: []metav1.OwnerReference{ref}}}
	}
	for _, tc := range []struct {
		name      string
		placement *PodVMPlacement
		pod       *corev1.Pod
		want      bool
	}{
		{name: "owned by this Pod", placement: owner(nil), pod: pod, want: true},
		{name: "earlier Pod with the same name",
			placement: owner(func(r *metav1.OwnerReference) { r.UID = "old-uid" }), pod: pod},
		{name: "not the controller",
			placement: owner(func(r *metav1.OwnerReference) { r.Controller = &notController }), pod: pod},
		{name: "owner is not a Pod",
			placement: owner(func(r *metav1.OwnerReference) { r.Kind = "ReplicaSet"; r.APIVersion = "apps/v1" }),
			pod:       pod},
		{name: "owner name differs", placement: owner(func(r *metav1.OwnerReference) { r.Name = "other" }), pod: pod},
		{name: "no owner", placement: &PodVMPlacement{}, pod: pod},
		{name: "Pod without UID", placement: owner(nil),
			pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "tenant"}}},
		{name: "nil placement", pod: pod},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.placement.AppliesTo(tc.pod))
		})
	}
}

func TestPodVMPlacementUnsupportedCSIFeatures(t *testing.T) {
	placement := &PodVMPlacement{Spec: PodVMPlacementSpec{RequiredFeatures: []PodVMPlacementFeature{
		{Name: "MultipleNetworkInterfaces", Consumers: []string{"VirtualNode"}},
		{Name: "PerVolumeStorage", Consumers: []string{PodVMPlacementConsumerCSI}},
		{Name: "Shared", Consumers: []string{"VirtualNode", PodVMPlacementConsumerCSI}},
		// A feature with no consumers blocks no component from acting; it only forbids writes.
		{Name: "WriteOnlyGuard"},
	}}}
	require.Equal(t, []string{"PerVolumeStorage", "Shared"}, placement.UnsupportedCSIFeatures())
	require.Empty(t, (&PodVMPlacement{}).UnsupportedCSIFeatures(), "an absent list means v1alpha1 only")
}
