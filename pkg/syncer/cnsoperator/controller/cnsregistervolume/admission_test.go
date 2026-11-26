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
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/event"

	cnsregistervolumev1alpha1 "sigs.k8s.io/vsphere-csi-driver/v3/pkg/apis/cnsoperator/cnsregistervolume/v1alpha1"
	volumes "sigs.k8s.io/vsphere-csi-driver/v3/pkg/common/cns-lib/volume"
	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/common/conditions"
)

// drainWakes returns the CR names woken so far, without blocking.
func drainWakes(events chan event.GenericEvent) []string {
	var names []string
	for {
		select {
		case ev := <-events:
			names = append(names, ev.Object.GetName())
		default:
			return names
		}
	}
}

// TestTerminalOutcomeFreesCapacityBeforeConsumption is the first half of the
// fix: a slot stops counting against the cap as soon as CNS is done with the
// task, not when the owning CR's next reconcile gets around to it. That gap is
// what kept every slot busy while vCenter sat idle.
func TestTerminalOutcomeFreesCapacityBeforeConsumption(t *testing.T) {
	tracker := newCnsTaskTracker(newFakeListView(), nil, make(chan event.GenericEvent, 10), 1)
	require.True(t, tracker.tryAcquireSlot(crKey("cr-1")))
	tracker.mu.Lock()
	tracker.slots[crKey("cr-1")] = "task-1"
	tracker.mu.Unlock()
	require.False(t, tracker.tryAcquireSlot(crKey("cr-2")), "the cap is full while task-1 runs")

	seedOutcome(tracker, "task-1", &cnsTaskOutcome{state: volumes.CreateVolumeTaskSucceeded})

	assert.Equal(t, 0, tracker.inFlight(), "a finished task is no longer load on vCenter")
	assert.True(t, tracker.tryAcquireSlot(crKey("cr-2")), "the next CR gets the capacity at once")
	tracker.mu.Lock()
	assert.Equal(t, "task-1", tracker.slots[crKey("cr-1")],
		"cr-1 keeps its task reference until it consumes the result")
	tracker.mu.Unlock()
}

// TestUnknownOutcomeKeepsCapacity: Unknown means the task may still be running,
// so its slot must keep counting.
func TestUnknownOutcomeKeepsCapacity(t *testing.T) {
	lv := newFakeListView()
	tracker := newCnsTaskTracker(lv, nil, make(chan event.GenericEvent, 10), 1)
	require.NoError(t, tracker.watch(context.Background(), crKey("cr-1"), taskRef("task-1"), "vol-1"))
	lv.deliver(t, "task-1", volumes.TaskResult{Err: fmt.Errorf("%w: connection reset", volumes.ErrTaskMonitoring)})
	waitFor(t, func() bool { _, ok := tracker.outcome("task-1"); return ok }, "outcome")

	out, _ := tracker.outcome("task-1")
	require.Equal(t, volumes.CreateVolumeTaskUnknown, out.state)
	assert.Equal(t, 1, tracker.inFlight())
	assert.False(t, tracker.tryAcquireSlot(crKey("cr-2")))
}

// TestListViewTaskFailureIsFailed: when vCenter reports the task in the error
// state the listview sends ErrTaskFailed with no TaskInfo. That is a real
// failure, unlike the ErrTaskMonitoring the listview sends for a lost session,
// and is recorded as Failed rather than re-read as Unknown.
func TestListViewTaskFailureIsFailed(t *testing.T) {
	lv := newFakeListView()
	events := make(chan event.GenericEvent, 10)
	tracker := newCnsTaskTracker(lv, nil, events, 1)
	require.NoError(t, tracker.watch(context.Background(), crKey("cr-1"), taskRef("task-1"), "vol-1"))
	lv.deliver(t, "task-1", volumes.TaskResult{
		Err: fmt.Errorf("%w: The operation is not allowed in the current state.", volumes.ErrTaskFailed)})
	waitFor(t, func() bool { _, ok := tracker.outcome("task-1"); return ok }, "outcome")

	out, _ := tracker.outcome("task-1")
	assert.Equal(t, volumes.CreateVolumeTaskFailed, out.state)
	assert.ErrorIs(t, out.err, volumes.ErrTaskFailed)
	assert.Equal(t, 0, tracker.inFlight(), "a failed task frees its slot")
}

// TestWaitersAreAdmittedInOrder: a freed slot goes to the CR that has waited
// longest, and only that CR is woken for it.
func TestWaitersAreAdmittedInOrder(t *testing.T) {
	events := make(chan event.GenericEvent, 10)
	tracker := newCnsTaskTracker(newFakeListView(), nil, events, 1)
	require.True(t, tracker.tryAcquireSlot(crKey("holder")))
	for _, name := range []string{"cr-a", "cr-b", "cr-c"} {
		require.False(t, tracker.tryAcquireSlot(crKey(name)))
		time.Sleep(time.Millisecond)
	}
	require.Equal(t, 3, tracker.waiting())

	tracker.releaseSlot(crKey("holder"))
	assert.Equal(t, []string{"cr-a"}, drainWakes(events), "only the head of the line is woken")
	assert.False(t, tracker.tryAcquireSlot(crKey("cr-c")), "cr-c may not take cr-a's slot")
	assert.False(t, tracker.tryAcquireSlot(crKey("cr-b")), "cr-b may not take cr-a's slot")
	assert.True(t, tracker.tryAcquireSlot(crKey("cr-a")))
	assert.Equal(t, 2, tracker.waiting())

	tracker.releaseSlot(crKey("cr-a"))
	assert.Equal(t, []string{"cr-b"}, drainWakes(events))
}

// TestNewcomerDoesNotJumpTheLine: a CR seen for the first time queues behind
// the CRs already waiting, even if a slot is momentarily free.
func TestNewcomerDoesNotJumpTheLine(t *testing.T) {
	tracker := newCnsTaskTracker(newFakeListView(), nil, make(chan event.GenericEvent, 10), 1)
	require.True(t, tracker.tryAcquireSlot(crKey("holder")))
	require.False(t, tracker.tryAcquireSlot(crKey("cr-a")))
	tracker.releaseSlot(crKey("holder"))

	assert.False(t, tracker.tryAcquireSlot(crKey("cr-new")))
	assert.True(t, tracker.tryAcquireSlot(crKey("cr-a")))
}

// TestRetryAfterFailureHasNoPriority: a CR whose task failed rejoins the line
// at the back, behind CRs that were already waiting.
func TestRetryAfterFailureHasNoPriority(t *testing.T) {
	events := make(chan event.GenericEvent, 10)
	tracker := newCnsTaskTracker(newFakeListView(), nil, events, 1)
	require.True(t, tracker.tryAcquireSlot(crKey("cr-a")))
	require.False(t, tracker.tryAcquireSlot(crKey("cr-b")))

	// cr-a's task fails; it consumes the result and asks again.
	tracker.forget(crKey("cr-a"), "")
	assert.Equal(t, []string{"cr-b"}, drainWakes(events))
	assert.False(t, tracker.tryAcquireSlot(crKey("cr-a")), "the retry waits its turn")
	assert.True(t, tracker.tryAcquireSlot(crKey("cr-b")))
}

// TestUnclaimedWakeLosesItsTurn: a woken CR that does not come back for its
// slot must not hold up the line behind it.
func TestUnclaimedWakeLosesItsTurn(t *testing.T) {
	events := make(chan event.GenericEvent, 10)
	tracker := newCnsTaskTracker(newFakeListView(), nil, events, 1)
	tracker.claimWindow = 20 * time.Millisecond
	require.True(t, tracker.tryAcquireSlot(crKey("holder")))
	require.False(t, tracker.tryAcquireSlot(crKey("cr-a")))
	time.Sleep(time.Millisecond)
	require.False(t, tracker.tryAcquireSlot(crKey("cr-b")))
	tracker.releaseSlot(crKey("holder"))
	require.Equal(t, []string{"cr-a"}, drainWakes(events))
	require.False(t, tracker.tryAcquireSlot(crKey("cr-b")))

	time.Sleep(30 * time.Millisecond)
	assert.True(t, tracker.tryAcquireSlot(crKey("cr-b")), "cr-a did not claim within its window")
}

// TestReleaseCRLeavesTheLine: a deleted CR stops waiting.
func TestReleaseCRLeavesTheLine(t *testing.T) {
	tracker := newCnsTaskTracker(newFakeListView(), nil, make(chan event.GenericEvent, 10), 1)
	require.True(t, tracker.tryAcquireSlot(crKey("holder")))
	require.False(t, tracker.tryAcquireSlot(crKey("cr-a")))
	tracker.releaseCR(crKey("cr-a"))
	assert.Equal(t, 0, tracker.waiting())
}

// TestMustWaitReservesNothing: the reconcile fast path may only ever refuse.
func TestMustWaitReservesNothing(t *testing.T) {
	tracker := newCnsTaskTracker(newFakeListView(), nil, make(chan event.GenericEvent, 10), 1)
	assert.False(t, tracker.mustWait(crKey("cr-a")))
	assert.Equal(t, 0, tracker.inFlight(), "an admissible CR is not given a slot by the check")
	require.True(t, tracker.tryAcquireSlot(crKey("holder")))
	assert.True(t, tracker.mustWait(crKey("cr-a")))
	assert.Equal(t, 1, tracker.waiting(), "a refused CR is put in line")
	assert.False(t, tracker.mustWait(crKey("holder")), "a slot holder never waits")
}

// TestCapIsNeverExceededUnderContention hammers admission and release from
// many goroutines (run with -race).
func TestCapIsNeverExceededUnderContention(t *testing.T) {
	const capacity = 5
	tracker := newCnsTaskTracker(newFakeListView(), nil, make(chan event.GenericEvent, 1000), capacity)
	var holders, peak atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cr := crKey(fmt.Sprintf("cr-%d", i))
			for !tracker.tryAcquireSlot(cr) {
				time.Sleep(time.Millisecond)
			}
			n := holders.Add(1)
			for {
				p := peak.Load()
				if n <= p || peak.CompareAndSwap(p, n) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			holders.Add(-1)
			tracker.forget(cr, "")
		}(i)
	}
	wg.Wait()
	assert.LessOrEqual(t, peak.Load(), int32(capacity))
	assert.Equal(t, 0, tracker.inFlight())
	assert.Equal(t, 0, tracker.waiting())
}

// TestWaitingCRUsesSafetyRequeue: a CR refused a slot is woken by the next
// release, so its own requeue is only a backstop and must not be a tight poll.
func TestWaitingCRUsesSafetyRequeue(t *testing.T) {
	instance := newCondTestInstance(cnsregistervolumev1alpha1.CnsRegisterVolumeStatus{})
	r, _, _ := newAsyncTestReconciler(t, instance, 1)
	require.True(t, r.taskTracker.tryAcquireSlot(apitypes.NamespacedName{Name: "other", Namespace: "test-ns"}))

	_, result, err := r.submitOrPollCreateVolume(context.Background(), instance, nil, asyncTestSpec(), time.Second)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 30*time.Second, result.RequeueAfter)
	assert.Equal(t, 1, r.taskTracker.waiting())
}

// TestConsumingResultTakesNoSlot: once a finished task's capacity has gone to
// the next CR, the owner consuming its result must not take it back.
func TestConsumingResultTakesNoSlot(t *testing.T) {
	instance := newCondTestInstance(cnsregistervolumev1alpha1.CnsRegisterVolumeStatus{CnsTaskID: "task-done"})
	r, _, _ := newAsyncTestReconciler(t, instance, 1)
	r.taskTracker.mu.Lock()
	r.taskTracker.slots[instanceKey(instance)] = "task-done"
	r.taskTracker.mu.Unlock()
	seedOutcome(r.taskTracker, "task-done", &cnsTaskOutcome{
		state: volumes.CreateVolumeTaskFailed, err: errors.New("CnsFault: InvalidState")})
	other := apitypes.NamespacedName{Name: "other", Namespace: "test-ns"}
	require.True(t, r.taskTracker.tryAcquireSlot(other))

	_, result, err := r.submitOrPollCreateVolume(context.Background(), instance, nil, asyncTestSpec(), time.Second)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, r.taskTracker.inFlight(), "only the other CR holds capacity")
	r.taskTracker.mu.Lock()
	_, held := r.taskTracker.slots[instanceKey(instance)]
	r.taskTracker.mu.Unlock()
	assert.False(t, held)
}

// TestFailedTaskRecordsWarning: every failed CreateVolume leaves a Warning on
// the CR. In the 1000-volume run none did, which hid 276 host-side failures.
func TestFailedTaskRecordsWarning(t *testing.T) {
	instance := newCondTestInstance(cnsregistervolumev1alpha1.CnsRegisterVolumeStatus{CnsTaskID: "task-1"})
	r, lv, _ := newAsyncTestReconciler(t, instance, 1)
	recorder := record.NewFakeRecorder(8)
	r.recorder = recorder
	require.NoError(t, r.taskTracker.watch(context.Background(), instanceKey(instance), taskRef("task-1"), "vol"))
	lv.deliver(t, "task-1", volumes.TaskResult{
		Err: fmt.Errorf("%w: The operation is not allowed in the current state.", volumes.ErrTaskFailed)})
	waitFor(t, func() bool { _, ok := r.taskTracker.outcome("task-1"); return ok }, "outcome")

	_, result, err := r.submitOrPollCreateVolume(context.Background(), instance, nil, asyncTestSpec(), time.Second)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, instance.Status.CnsTaskID, "the failed task is cleared so the CR resubmits")
	select {
	case ev := <-recorder.Events:
		assert.True(t, strings.HasPrefix(ev, "Warning CnsRegisterVolumeFailed"), ev)
	default:
		t.Fatal("a failed CNS task recorded no event")
	}
}

// TestRepeatedTaskFailuresStayWithinBudget bounds what the per-CR backoff
// costs a CR that keeps hitting a transient host fault (the duplicate-ddb.uuid
// InvalidState seen in the field, where no disk failed more than 4 times).
// The requeue after each failure is the backoff in force for that pass, which
// doubles per failure from 1s; four failures cost 1+2+4+8 = 15s.
func TestRepeatedTaskFailuresStayWithinBudget(t *testing.T) {
	instance := newCondTestInstance(cnsregistervolumev1alpha1.CnsRegisterVolumeStatus{})
	r, _, _ := newAsyncTestReconciler(t, instance, 1)
	key := instanceKey(instance)
	backOffDuration[key] = time.Second
	var waited time.Duration
	for i := 1; i <= 4; i++ {
		taskID := fmt.Sprintf("task-%d", i)
		require.NoError(t, setInstanceCnsTaskID(context.Background(), r, instance, taskID))
		r.taskTracker.mu.Lock()
		r.taskTracker.slots[key] = taskID
		r.taskTracker.mu.Unlock()
		seedOutcome(r.taskTracker, taskID, &cnsTaskOutcome{
			state: volumes.CreateVolumeTaskFailed, err: errors.New("CnsFault: InvalidState")})
		timeout := backOffDuration[key]
		_, result, err := r.submitOrPollCreateVolume(context.Background(), instance, nil, asyncTestSpec(), timeout)
		require.NoError(t, err)
		require.NotNil(t, result)
		waited += result.RequeueAfter
	}
	assert.LessOrEqual(t, waited, 30*time.Second)
	assert.Equal(t, cnsregistervolumev1alpha1.ReasonCnsTaskFailed,
		conditions.GetReason(instance, cnsregistervolumev1alpha1.ConditionCnsVolumeRegistered))
}

// TestWaitingCRSkipsPreCNSWork: a validated CR that cannot get a slot returns
// before the vCenter lookups and create-spec work. The reconciler here has no
// vCenter configuration at all, so reaching that work would fail the pass.
func TestWaitingCRSkipsPreCNSWork(t *testing.T) {
	instance := newCondTestInstance(cnsregistervolumev1alpha1.CnsRegisterVolumeStatus{
		Conditions: []metav1.Condition{{
			Type:   cnsregistervolumev1alpha1.ConditionSpecValidated,
			Status: metav1.ConditionTrue,
			Reason: cnsregistervolumev1alpha1.ReasonValidated,
		}},
	})
	r, _, _ := newAsyncTestReconciler(t, instance, 1)
	// The fake client may assign a generation; the condition must match it.
	instance.Status.Conditions[0].ObservedGeneration = instance.Generation
	require.NoError(t, r.client.Status().Update(context.Background(), instance))
	r.taskTracker.seed(nil)
	require.True(t, r.taskTracker.tryAcquireSlot(apitypes.NamespacedName{Name: "other", Namespace: "test-ns"}))

	result, err := r.Reconcile(context.Background(), reconcileRequestFor(instance))
	require.NoError(t, err)
	assert.Equal(t, cnsSlotRetryInterval, result.RequeueAfter)
	assert.Equal(t, 1, r.taskTracker.waiting())
	current := &cnsregistervolumev1alpha1.CnsRegisterVolume{}
	require.NoError(t, r.client.Get(context.Background(), instanceKey(instance), current))
	assert.Equal(t, cnsregistervolumev1alpha1.ReasonWaitingForTaskSlot,
		conditions.GetReason(current, cnsregistervolumev1alpha1.ConditionCnsVolumeRegistered))
}
