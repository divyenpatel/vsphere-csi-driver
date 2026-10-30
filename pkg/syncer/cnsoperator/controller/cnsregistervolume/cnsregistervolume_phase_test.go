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

package cnsregistervolume

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	cnsregistervolumev1alpha1 "sigs.k8s.io/vsphere-csi-driver/v3/pkg/apis/cnsoperator/cnsregistervolume/v1alpha1"
	cnsvsphere "sigs.k8s.io/vsphere-csi-driver/v3/pkg/common/cns-lib/vsphere"
	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/csi/service/common/commonco"
)

// phaseTestScheme returns a scheme that knows the CnsRegisterVolume types.
func phaseTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	scheme.AddKnownTypes(schema.GroupVersion{Group: "cnsoperator.vmware.com", Version: "v1alpha1"},
		&cnsregistervolumev1alpha1.CnsRegisterVolume{}, &cnsregistervolumev1alpha1.CnsRegisterVolumeList{})
	return scheme
}

// newPhaseTestInstance returns a CnsRegisterVolume with the given starting status.
func newPhaseTestInstance(
	status cnsregistervolumev1alpha1.CnsRegisterVolumeStatus,
) *cnsregistervolumev1alpha1.CnsRegisterVolume {
	return &cnsregistervolumev1alpha1.CnsRegisterVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "test-volume", Namespace: "test-ns", UID: "test-uid"},
		Spec:       cnsregistervolumev1alpha1.CnsRegisterVolumeSpec{VolumeID: "test-volume-id"},
		Status:     status,
	}
}

// getPhaseTestInstance re-reads the instance from the client.
func getPhaseTestInstance(t *testing.T, c ctrlclient.Client,
	instance *cnsregistervolumev1alpha1.CnsRegisterVolume) *cnsregistervolumev1alpha1.CnsRegisterVolume {
	t.Helper()
	got := &cnsregistervolumev1alpha1.CnsRegisterVolume{}
	err := c.Get(context.Background(),
		types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, got)
	assert.NoError(t, err)
	return got
}

// capturedPatch records the status patches a test issues, so assertions can be made
// about which keys were sent rather than only about the resulting object. The
// distinction matters: a merge patch that omits a key leaves the stored value alone,
// which is how an unchanged lastUpdateTime is expressed on the wire.
type capturedPatch struct {
	patches []map[string]interface{}
}

func (c *capturedPatch) count() int { return len(c.patches) }

func (c *capturedPatch) last() map[string]interface{} {
	if len(c.patches) == 0 {
		return nil
	}
	return c.patches[len(c.patches)-1]
}

// newCapturingClient returns a client that records the status subresource patches
// it is given before applying them. failWith, when non-nil, is returned instead of
// applying the patch.
func newCapturingClient(instance *cnsregistervolumev1alpha1.CnsRegisterVolume,
	failWith error) (ctrlclient.Client, *capturedPatch) {
	rec := &capturedPatch{}
	c := fake.NewClientBuilder().
		WithScheme(phaseTestScheme()).
		WithObjects(instance).
		WithStatusSubresource(instance).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourcePatch: func(ctx context.Context, client ctrlclient.Client, subResourceName string,
				obj ctrlclient.Object, patch ctrlclient.Patch, opts ...ctrlclient.SubResourcePatchOption) error {
				data, err := patch.Data(obj)
				if err != nil {
					return err
				}
				var decoded map[string]interface{}
				if err := json.Unmarshal(data, &decoded); err != nil {
					return err
				}
				if status, ok := decoded["status"].(map[string]interface{}); ok {
					rec.patches = append(rec.patches, status)
				}
				if failWith != nil {
					return failWith
				}
				return client.Status().Patch(ctx, obj, patch, opts...)
			},
		}).
		Build()
	return c, rec
}

// TestPatchStatusStampsLastUpdateTimeOnPhaseChange verifies that a phase change is
// published together with a fresh publish time.
func TestPatchStatusStampsLastUpdateTimeOnPhaseChange(t *testing.T) {
	ctx := context.Background()
	instance := newPhaseTestInstance(cnsregistervolumev1alpha1.CnsRegisterVolumeStatus{})
	c, rec := newCapturingClient(instance, nil)

	oldInstance := instance.DeepCopy()
	newInstance := instance.DeepCopy()
	newInstance.Status.Phase = cnsregistervolumev1alpha1.CnsRegisterVolumePhaseCnsRegistering

	assert.NoError(t, patchCnsRegisterVolumeStatus(ctx, c, oldInstance, newInstance))

	assert.Contains(t, rec.last(), "phase")
	assert.Contains(t, rec.last(), "lastUpdateTime",
		"a phase change is a status change and must be stamped with a publish time")

	got := getPhaseTestInstance(t, c, instance)
	assert.Equal(t, cnsregistervolumev1alpha1.CnsRegisterVolumePhaseCnsRegistering, got.Status.Phase)
	assert.NotNil(t, got.Status.LastUpdateTime)
}

// TestPatchStatusDoesNotStampWhenNothingChanged is the rule that makes the field
// meaningful: re-publishing identical content must leave lastUpdateTime alone, so
// that its age measures how long the CR has been stuck rather than how recently it
// was reconciled.
func TestPatchStatusDoesNotStampWhenNothingChanged(t *testing.T) {
	ctx := context.Background()
	earlier := metav1.NewTime(metav1.Now().Add(-time.Hour))
	instance := newPhaseTestInstance(cnsregistervolumev1alpha1.CnsRegisterVolumeStatus{
		Registered:     false,
		Error:          "some transient error",
		Phase:          cnsregistervolumev1alpha1.CnsRegisterVolumePhaseConfiguringK8s,
		LastUpdateTime: &earlier,
	})
	c, rec := newCapturingClient(instance, nil)

	oldInstance := instance.DeepCopy()
	// Same registered, same error text, same phase: a retry that reports the same
	// condition it reported last time.
	newInstance := instance.DeepCopy()

	assert.NoError(t, patchCnsRegisterVolumeStatus(ctx, c, oldInstance, newInstance))

	assert.NotContains(t, rec.last(), "lastUpdateTime",
		"an unchanged status must not move the publish time")

	got := getPhaseTestInstance(t, c, instance)
	assert.NotNil(t, got.Status.LastUpdateTime)
	assert.Equal(t, earlier.Unix(), got.Status.LastUpdateTime.Unix(),
		"the stored publish time must be preserved untouched")
}

// TestPatchStatusStampsOnErrorAndRegisteredChange verifies the other two fields that
// count as published content.
func TestPatchStatusStampsOnErrorAndRegisteredChange(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*cnsregistervolumev1alpha1.CnsRegisterVolume)
	}{
		{"error changes", func(i *cnsregistervolumev1alpha1.CnsRegisterVolume) {
			i.Status.Error = "a new and different error"
		}},
		{"registered changes", func(i *cnsregistervolumev1alpha1.CnsRegisterVolume) {
			i.Status.Registered = true
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			instance := newPhaseTestInstance(cnsregistervolumev1alpha1.CnsRegisterVolumeStatus{
				Error: "the original error",
				Phase: cnsregistervolumev1alpha1.CnsRegisterVolumePhaseConfiguringK8s,
			})
			c, rec := newCapturingClient(instance, nil)

			oldInstance := instance.DeepCopy()
			newInstance := instance.DeepCopy()
			tc.mutate(newInstance)

			assert.NoError(t, patchCnsRegisterVolumeStatus(ctx, c, oldInstance, newInstance))
			assert.Contains(t, rec.last(), "lastUpdateTime")
			assert.NotNil(t, getPhaseTestInstance(t, c, instance).Status.LastUpdateTime)
		})
	}
}

// TestPatchStatusOmitsPhaseWhenNeverSet verifies backwards compatibility on the wire:
// a CR that has never had a phase must not acquire an empty one, so the status an old
// consumer reads is byte-for-byte what it reads today.
func TestPatchStatusOmitsPhaseWhenNeverSet(t *testing.T) {
	ctx := context.Background()
	instance := newPhaseTestInstance(cnsregistervolumev1alpha1.CnsRegisterVolumeStatus{})
	c, rec := newCapturingClient(instance, nil)

	oldInstance := instance.DeepCopy()
	newInstance := instance.DeepCopy()
	newInstance.Status.Registered = true

	assert.NoError(t, patchCnsRegisterVolumeStatus(ctx, c, oldInstance, newInstance))

	assert.NotContains(t, rec.last(), "phase",
		"a CR with no phase must not have an empty phase written into its status")
	assert.Contains(t, rec.last(), "registered")
}

// TestPatchStatusPreservesStoredPhaseOnErrorOnlyWrite verifies that setInstanceError,
// which knows nothing about phases, cannot wipe a phase published earlier.
func TestPatchStatusPreservesStoredPhaseOnErrorOnlyWrite(t *testing.T) {
	ctx := context.Background()
	instance := newPhaseTestInstance(cnsregistervolumev1alpha1.CnsRegisterVolumeStatus{
		Phase: cnsregistervolumev1alpha1.CnsRegisterVolumePhaseWaitingForBind,
	})
	c, _ := newCapturingClient(instance, nil)

	oldInstance := instance.DeepCopy()
	newInstance := instance.DeepCopy()
	newInstance.Status.Error = "bind is taking a while"

	assert.NoError(t, patchCnsRegisterVolumeStatus(ctx, c, oldInstance, newInstance))

	got := getPhaseTestInstance(t, c, instance)
	assert.Equal(t, cnsregistervolumev1alpha1.CnsRegisterVolumePhaseWaitingForBind, got.Status.Phase,
		"an error-only write must leave the phase where it was")
	assert.Equal(t, "bind is taking a while", got.Status.Error)
}

// TestSetInstancePhasePublishes verifies the happy path of the phase helper.
func TestSetInstancePhasePublishes(t *testing.T) {
	ctx := context.Background()
	instance := newPhaseTestInstance(cnsregistervolumev1alpha1.CnsRegisterVolumeStatus{})
	c, rec := newCapturingClient(instance, nil)
	r := &ReconcileCnsRegisterVolume{client: c, scheme: phaseTestScheme()}

	setInstancePhase(ctx, r, instance, cnsregistervolumev1alpha1.CnsRegisterVolumePhasePending)

	assert.Equal(t, 1, rec.count())
	assert.Equal(t, cnsregistervolumev1alpha1.CnsRegisterVolumePhasePending, instance.Status.Phase)
	assert.NotNil(t, instance.Status.LastUpdateTime)

	got := getPhaseTestInstance(t, c, instance)
	assert.Equal(t, cnsregistervolumev1alpha1.CnsRegisterVolumePhasePending, got.Status.Phase)
}

// TestSetInstancePhaseIsNoOpWhenUnchanged verifies that re-entering the same phase
// issues no API write at all. Reconcile restarts from the top on every retry, so
// without this a CR that keeps failing would rewrite its phase, and its publish
// time, on every attempt and would always look like it was making progress.
func TestSetInstancePhaseIsNoOpWhenUnchanged(t *testing.T) {
	ctx := context.Background()
	earlier := metav1.NewTime(metav1.Now().Add(-time.Hour))
	instance := newPhaseTestInstance(cnsregistervolumev1alpha1.CnsRegisterVolumeStatus{
		Phase:          cnsregistervolumev1alpha1.CnsRegisterVolumePhaseCnsRegistering,
		LastUpdateTime: &earlier,
	})
	c, rec := newCapturingClient(instance, nil)
	r := &ReconcileCnsRegisterVolume{client: c, scheme: phaseTestScheme()}

	setInstancePhase(ctx, r, instance, cnsregistervolumev1alpha1.CnsRegisterVolumePhaseCnsRegistering)

	assert.Equal(t, 0, rec.count(), "re-entering the same phase must not write to the API server")
	assert.Equal(t, earlier.Unix(), instance.Status.LastUpdateTime.Unix(),
		"a no-op phase transition must not move the publish time")
}

// TestSetInstancePhaseLeavesRegisteredAndErrorAlone verifies the helper is
// observational only: it must never disturb the fields callers key off.
func TestSetInstancePhaseLeavesRegisteredAndErrorAlone(t *testing.T) {
	ctx := context.Background()
	instance := newPhaseTestInstance(cnsregistervolumev1alpha1.CnsRegisterVolumeStatus{
		Registered: false,
		Error:      "an error a caller is currently reading",
	})
	c, _ := newCapturingClient(instance, nil)
	r := &ReconcileCnsRegisterVolume{client: c, scheme: phaseTestScheme()}

	setInstancePhase(ctx, r, instance, cnsregistervolumev1alpha1.CnsRegisterVolumePhaseConfiguringK8s)

	got := getPhaseTestInstance(t, c, instance)
	assert.False(t, got.Status.Registered)
	assert.Equal(t, "an error a caller is currently reading", got.Status.Error)
	assert.Equal(t, cnsregistervolumev1alpha1.CnsRegisterVolumePhaseConfiguringK8s, got.Status.Phase)
}

// TestSetInstancePhaseRollsBackOnPatchFailure verifies that a failed publish leaves
// the in-memory status matching the server rather than drifting ahead of it. If it
// drifted, the next transition would compare against a phase the server never
// received and could skip the write entirely.
func TestSetInstancePhaseRollsBackOnPatchFailure(t *testing.T) {
	ctx := context.Background()
	earlier := metav1.NewTime(metav1.Now().Add(-time.Hour))
	instance := newPhaseTestInstance(cnsregistervolumev1alpha1.CnsRegisterVolumeStatus{
		Phase:          cnsregistervolumev1alpha1.CnsRegisterVolumePhasePending,
		LastUpdateTime: &earlier,
	})
	c, _ := newCapturingClient(instance, errors.New("apiserver is unavailable"))
	r := &ReconcileCnsRegisterVolume{client: c, scheme: phaseTestScheme()}

	// Must not panic and must not return anything for the caller to handle.
	setInstancePhase(ctx, r, instance, cnsregistervolumev1alpha1.CnsRegisterVolumePhaseConfiguringK8s)

	assert.Equal(t, cnsregistervolumev1alpha1.CnsRegisterVolumePhasePending, instance.Status.Phase,
		"a failed publish must roll the in-memory phase back")
	assert.Equal(t, earlier.Unix(), instance.Status.LastUpdateTime.Unix(),
		"a failed publish must roll the in-memory publish time back")
}

// TestStatusDeepCopyIsIndependent guards the aliasing hazard introduced by making
// LastUpdateTime a pointer. setInstanceError and setInstancePhase both DeepCopy the
// instance and then mutate the original, and compare the two to decide whether to
// stamp a new time; if DeepCopy shared the pointer, that comparison would be made
// against a value that had already been overwritten.
func TestStatusDeepCopyIsIndependent(t *testing.T) {
	now := metav1.Now()
	orig := &cnsregistervolumev1alpha1.CnsRegisterVolumeStatus{
		Registered:     true,
		Phase:          cnsregistervolumev1alpha1.CnsRegisterVolumePhaseRegistered,
		LastUpdateTime: &now,
	}

	cp := orig.DeepCopy()
	assert.NotSame(t, orig.LastUpdateTime, cp.LastUpdateTime,
		"DeepCopy must not share the LastUpdateTime pointer")
	assert.Equal(t, orig.LastUpdateTime.Unix(), cp.LastUpdateTime.Unix())

	later := metav1.NewTime(now.Add(time.Hour))
	cp.LastUpdateTime = &later
	assert.Equal(t, now.Unix(), orig.LastUpdateTime.Unix(),
		"mutating the copy must not affect the original")

	// A nil pointer must survive the round trip as nil, not as the zero time.
	var nilStatus cnsregistervolumev1alpha1.CnsRegisterVolumeStatus
	assert.Nil(t, nilStatus.DeepCopy().LastUpdateTime)
}

// TestCnsRegisterVolumeDeepCopyIsIndependent verifies the same for the whole object,
// which previously copied Status by assignment.
func TestCnsRegisterVolumeDeepCopyIsIndependent(t *testing.T) {
	now := metav1.Now()
	orig := newPhaseTestInstance(cnsregistervolumev1alpha1.CnsRegisterVolumeStatus{
		Phase:          cnsregistervolumev1alpha1.CnsRegisterVolumePhaseWaitingForBind,
		LastUpdateTime: &now,
	})

	cp := orig.DeepCopy()
	assert.NotSame(t, orig.Status.LastUpdateTime, cp.Status.LastUpdateTime)

	later := metav1.NewTime(now.Add(time.Hour))
	cp.Status.LastUpdateTime = &later
	assert.Equal(t, now.Unix(), orig.Status.LastUpdateTime.Unix())
}

// TestStatusJSONOmitsNewFieldsWhenUnset is the back-compatibility guarantee for
// existing consumers: a status with no phase and no publish time must serialize to
// exactly what it serialized to before these fields existed.
func TestStatusJSONOmitsNewFieldsWhenUnset(t *testing.T) {
	data, err := json.Marshal(cnsregistervolumev1alpha1.CnsRegisterVolumeStatus{Registered: true})
	assert.NoError(t, err)
	assert.JSONEq(t, `{"registered":true}`, string(data),
		"the new fields must be absent, not null, when unset")

	// registered keeps its required (non-omitempty) encoding.
	data, err = json.Marshal(cnsregistervolumev1alpha1.CnsRegisterVolumeStatus{})
	assert.NoError(t, err)
	assert.JSONEq(t, `{"registered":false}`, string(data))
}

// TestStatusJSONRoundTrip verifies the new fields survive serialization, and that an
// absent lastUpdateTime decodes as nil rather than as the zero time. Consumers are
// told to treat absent as unknown, which they can only do if it decodes as nil.
func TestStatusJSONRoundTrip(t *testing.T) {
	var decoded cnsregistervolumev1alpha1.CnsRegisterVolumeStatus
	assert.NoError(t, json.Unmarshal([]byte(`{"registered":false,"phase":"WaitingForBind"}`), &decoded))
	assert.Equal(t, cnsregistervolumev1alpha1.CnsRegisterVolumePhaseWaitingForBind, decoded.Phase)
	assert.Nil(t, decoded.LastUpdateTime, "an absent publish time must decode as nil, not as the zero time")

	now := metav1.Now()
	data, err := json.Marshal(cnsregistervolumev1alpha1.CnsRegisterVolumeStatus{
		Registered:     true,
		Phase:          cnsregistervolumev1alpha1.CnsRegisterVolumePhaseRegistered,
		LastUpdateTime: &now,
	})
	assert.NoError(t, err)
	var back cnsregistervolumev1alpha1.CnsRegisterVolumeStatus
	assert.NoError(t, json.Unmarshal(data, &back))
	assert.Equal(t, cnsregistervolumev1alpha1.CnsRegisterVolumePhaseRegistered, back.Phase)
	assert.NotNil(t, back.LastUpdateTime)
	assert.Equal(t, now.Unix(), back.LastUpdateTime.Unix())
}

// TestPhaseConstants pins the wire values, since they are published API and a rename
// would silently change what operators and dashboards see.
func TestPhaseConstants(t *testing.T) {
	for want, got := range map[string]cnsregistervolumev1alpha1.CnsRegisterVolumePhase{
		"Pending":        cnsregistervolumev1alpha1.CnsRegisterVolumePhasePending,
		"CnsRegistering": cnsregistervolumev1alpha1.CnsRegisterVolumePhaseCnsRegistering,
		"ConfiguringK8s": cnsregistervolumev1alpha1.CnsRegisterVolumePhaseConfiguringK8s,
		"WaitingForBind": cnsregistervolumev1alpha1.CnsRegisterVolumePhaseWaitingForBind,
		"Finalizing":     cnsregistervolumev1alpha1.CnsRegisterVolumePhaseFinalizing,
		"Registered":     cnsregistervolumev1alpha1.CnsRegisterVolumePhaseRegistered,
		"Failed":         cnsregistervolumev1alpha1.CnsRegisterVolumePhaseFailed,
	} {
		assert.Equal(t, want, string(got))
	}
}

// TestReconcilePublishesPendingPhaseBeforeValidation verifies that the phase is
// published before the spec is validated. A CR that is rejected early must still
// carry a phase, otherwise the states "not looked at yet" and "looked at and
// rejected" are indistinguishable, which is the gap this field exists to close.
func TestReconcilePublishesPendingPhaseBeforeValidation(t *testing.T) {
	backOffDuration = make(map[types.NamespacedName]time.Duration)
	ctx := context.Background()

	// VolumeID without AccessMode fails validateCnsRegisterVolumeSpec, which runs
	// immediately after the phase is published and before vCenter is contacted.
	crv := &cnsregistervolumev1alpha1.CnsRegisterVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "test-volume", Namespace: "test-ns"},
		Spec: cnsregistervolumev1alpha1.CnsRegisterVolumeSpec{
			PvcName:  "test-pvc",
			VolumeID: "dummy-volume-id",
		},
	}
	scheme := phaseTestScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(crv).WithStatusSubresource(crv).Build()

	commonco.ContainerOrchestratorUtility = &mockCOCommon{}
	r := &ReconcileCnsRegisterVolume{
		client:   fakeClient,
		scheme:   scheme,
		recorder: record.NewFakeRecorder(10),
	}

	_, err := r.Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "test-volume", Namespace: "test-ns"}})
	assert.NoError(t, err)

	got := getPhaseTestInstance(t, fakeClient, crv)
	assert.Equal(t, cnsregistervolumev1alpha1.CnsRegisterVolumePhasePending, got.Status.Phase,
		"a CR rejected during spec validation must still report Pending")
	assert.NotNil(t, got.Status.LastUpdateTime, "the published phase must carry a publish time")
	assert.False(t, got.Status.Registered, "registered must be untouched by the phase write")
	assert.Contains(t, got.Status.Error, "AccessMode",
		"the existing error reporting must be unaffected")
}

// TestReconcilePublishesFailedPhaseOnPermanentFailure verifies that the paths which
// already stopped requeuing before this change now say so on the CR. Phase=Failed is
// only ever written where the controller had already given up, so it reports
// existing behaviour rather than introducing new behaviour.
func TestReconcilePublishesFailedPhaseOnPermanentFailure(t *testing.T) {
	backOffDuration = make(map[types.NamespacedName]time.Duration)
	ctx := context.Background()
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	r, _, cleanupCalled := newHostLocalReconcileFixture(patches, "HostLocalStorage")
	defer wireAsyncCnsTaskSuccess(r)()
	defer func() { fssEnabledOverride = nil }()

	checkDatastorePolicyCompatibilityFn = func(_ context.Context, _ *cnsvsphere.VirtualCenter,
		_ *cnsvsphere.DatastoreInfo, _ string) (bool, error) {
		return false, nil
	}
	defer func() { checkDatastorePolicyCompatibilityFn = checkDatastorePolicyCompatibility }()

	result, err := reconcileUntilSettled(ctx, r, reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "test-volume", Namespace: "test-ns"}})

	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result, "this path must still not requeue")
	assert.True(t, *cleanupCalled, "the CNS volume must still be cleaned up")

	got := &cnsregistervolumev1alpha1.CnsRegisterVolume{}
	assert.NoError(t, r.client.Get(ctx,
		types.NamespacedName{Name: "test-volume", Namespace: "test-ns"}, got))
	assert.Equal(t, cnsregistervolumev1alpha1.CnsRegisterVolumePhaseFailed, got.Status.Phase,
		"a permanent failure must be reported as Failed")
	assert.False(t, got.Status.Registered,
		"Failed must never be published together with registered=true")
}
