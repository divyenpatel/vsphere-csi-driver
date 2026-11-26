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

package volume

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"

	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/csi/service/logger"
)

// listViewTransport answers ModifyListView. A task named in stall blocks until
// its context ends or release is closed; everything else succeeds at once. It
// counts every round trip so tests can assert what reached vCenter.
type listViewTransport struct {
	stall    string
	stalled  chan struct{}
	release  chan struct{}
	fault    error
	modifies atomic.Int32
	others   atomic.Int32
}

func (f *listViewTransport) RoundTrip(ctx context.Context, req, res soap.HasFault) error {
	body, ok := req.(*methods.ModifyListViewBody)
	if !ok {
		f.others.Add(1)
		return fmt.Errorf("unexpected request %T", req)
	}
	f.modifies.Add(1)
	if f.fault != nil {
		return f.fault
	}
	refs := append(append([]types.ManagedObjectReference{}, body.Req.Add...), body.Req.Remove...)
	for _, ref := range refs {
		if ref.Value == f.stall {
			close(f.stalled)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-f.release:
			}
		}
	}
	res.(*methods.ModifyListViewBody).Res = &types.ModifyListViewResponse{}
	return nil
}

func newConcurrencyTestView(transport *listViewTransport) (*ListViewImpl, context.Context) {
	client := &vim25.Client{RoundTripper: transport, ServiceContent: types.ServiceContent{
		SessionManager:    &types.ManagedObjectReference{Type: "SessionManager", Value: "session"},
		PropertyCollector: types.ManagedObjectReference{Type: "PropertyCollector", Value: "collector"},
	}}
	sharedCtx, sharedCancel := context.WithCancel(context.Background())
	l := &ListViewImpl{isReady: true, taskMap: NewTaskMap(),
		virtualCenter: newTestVirtualCenter(client), client: client,
		listView:              view.NewListView(client, types.ManagedObjectReference{Type: "ListView", Value: "view"}),
		waitForUpdatesContext: sharedCtx, waitForUpdatesCancelFunc: sharedCancel}
	return l, sharedCtx
}

func taskRef(name string) types.ManagedObjectReference {
	return types.ManagedObjectReference{Type: "Task", Value: name}
}

// TestStalledAddDoesNotBlockOtherRegistrations is the property the burst
// needed: one registration waiting on vCenter must not hold up every other
// registration in the process, as the fully serialized lock did.
func TestStalledAddDoesNotBlockOtherRegistrations(t *testing.T) {
	transport := &listViewTransport{stall: "task-slow", stalled: make(chan struct{}), release: make(chan struct{})}
	l, sharedCtx := newConcurrencyTestView(transport)
	ctx := logger.NewContextWithLogger(context.Background())

	slow := make(chan error, 1)
	go func() { slow <- l.AddTask(ctx, taskRef("task-slow"), make(chan TaskResult, 1)) }()
	<-transport.stalled

	done := make(chan error, 1)
	go func() { done <- l.AddTask(ctx, taskRef("task-fast"), make(chan TaskResult, 1)) }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("a registration waited behind another registration's vCenter call")
	}
	removed := make(chan error, 1)
	go func() { removed <- l.RemoveTask(ctx, taskRef("task-fast")) }()
	select {
	case err := <-removed:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("a removal waited behind another registration's vCenter call")
	}

	close(transport.release)
	require.NoError(t, <-slow)
	_, ok := l.taskMap.Get(taskRef("task-slow"))
	require.True(t, ok)
	_, ok = l.taskMap.Get(taskRef("task-fast"))
	require.False(t, ok)
	require.True(t, l.IsListViewReady())
	require.NoError(t, sharedCtx.Err())
}

// TestAddTaskMakesOneRoundTrip pins the removal of the per-call session check:
// a registration is a single ModifyListView and nothing else.
func TestAddTaskMakesOneRoundTrip(t *testing.T) {
	transport := &listViewTransport{}
	l, _ := newConcurrencyTestView(transport)
	ctx := logger.NewContextWithLogger(context.Background())
	require.NoError(t, l.AddTask(ctx, taskRef("task-1"), make(chan TaskResult, 1)))
	require.NoError(t, l.RemoveTask(ctx, taskRef("task-1")))
	require.EqualValues(t, 2, transport.modifies.Load())
	require.EqualValues(t, 0, transport.others.Load(), "no session check round trip")
}

// TestExpiredSessionAddInvalidatesView covers what the removed session check
// used to catch: an expired session fails the Add itself, the caller gets the
// same ErrListViewTaskAddition, and the view is marked for rebuild.
func TestExpiredSessionAddInvalidatesView(t *testing.T) {
	fault := &soap.Fault{Code: "ServerFaultCode", String: "The session is not authenticated."}
	fault.Detail.Fault = types.NotAuthenticated{}
	transport := &listViewTransport{fault: soap.WrapSoapFault(fault)}
	l, sharedCtx := newConcurrencyTestView(transport)
	ref := taskRef("task-1")
	err := l.AddTask(logger.NewContextWithLogger(context.Background()), ref, make(chan TaskResult, 1))
	require.ErrorIs(t, err, ErrListViewTaskAddition)
	require.False(t, l.IsListViewReady())
	require.ErrorIs(t, sharedCtx.Err(), context.Canceled, "listener must be told to rebuild the view")
	_, ok := l.taskMap.Get(ref)
	require.False(t, ok, "a failed registration must not stay in the task map")
}

// TestExpiredSessionRemoveInvalidatesView is the removal counterpart.
func TestExpiredSessionRemoveInvalidatesView(t *testing.T) {
	fault := &soap.Fault{Code: "ServerFaultCode", String: "The session is not authenticated."}
	fault.Detail.Fault = types.NotAuthenticated{}
	transport := &listViewTransport{fault: soap.WrapSoapFault(fault)}
	l, sharedCtx := newConcurrencyTestView(transport)
	require.Error(t, l.RemoveTask(logger.NewContextWithLogger(context.Background()), taskRef("task-1")))
	require.False(t, l.IsListViewReady())
	require.ErrorIs(t, sharedCtx.Err(), context.Canceled)
}

// TestViewLockSharedHoldersAreBounded checks that registrations run in
// parallel only up to viewLockSharedSlots.
func TestViewLockSharedHoldersAreBounded(t *testing.T) {
	var lock viewLock
	for i := 0; i < viewLockSharedSlots; i++ {
		require.NoError(t, lock.RLock(context.Background()))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, lock.RLock(ctx), context.DeadlineExceeded)
	lock.RUnlock()
	require.NoError(t, lock.RLock(context.Background()))
	for i := 0; i < viewLockSharedSlots; i++ {
		lock.RUnlock()
	}
}

// TestViewLockExclusiveExcludesShared keeps the guarantee view replacement
// relies on: it waits for in-flight registrations, and no registration starts
// while it is pending or held.
func TestViewLockExclusiveExcludesShared(t *testing.T) {
	var lock viewLock
	require.NoError(t, lock.RLock(context.Background()))

	acquired := make(chan struct{})
	go func() {
		require.NoError(t, lock.Lock(context.Background()))
		close(acquired)
	}()
	select {
	case <-acquired:
		t.Fatal("exclusive hold taken while a registration was in flight")
	case <-time.After(50 * time.Millisecond):
	}

	// A pending exclusive holder is not overtaken by new registrations.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, lock.RLock(ctx), context.DeadlineExceeded)

	lock.RUnlock()
	select {
	case <-acquired:
	case <-time.After(5 * time.Second):
		t.Fatal("exclusive hold not granted after registrations drained")
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel2()
	require.ErrorIs(t, lock.RLock(ctx2), context.DeadlineExceeded, "no registration while the view is replaced")
	lock.Unlock()
	require.NoError(t, lock.RLock(context.Background()))
	lock.RUnlock()
}

// TestConcurrentRegistrationsKeepTaskMapConsistent drives many concurrent
// add/remove pairs through the shared path under -race.
func TestConcurrentRegistrationsKeepTaskMapConsistent(t *testing.T) {
	transport := &listViewTransport{}
	l, sharedCtx := newConcurrencyTestView(transport)
	ctx := logger.NewContextWithLogger(context.Background())
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ref := taskRef(fmt.Sprintf("task-%d", i))
			require.NoError(t, l.AddTask(ctx, ref, make(chan TaskResult, 1)))
			if i%2 == 0 {
				require.NoError(t, l.RemoveTask(ctx, ref))
			}
		}(i)
	}
	wg.Wait()
	require.Equal(t, 100, l.taskMap.Count())
	require.EqualValues(t, 300, transport.modifies.Load())
	require.NoError(t, sharedCtx.Err())
}
