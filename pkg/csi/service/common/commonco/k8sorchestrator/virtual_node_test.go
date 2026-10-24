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

package k8sorchestrator

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/simulator"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/common/cns-lib/vsphere"
	commontypes "sigs.k8s.io/vsphere-csi-driver/v3/pkg/common/types"
	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/csi/service/common"
	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/internalapis/podvmv2"
)

func inventoryNode(name, host, cluster string) *corev1.Node {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name,
		Labels:      map[string]string{corev1.LabelTopologyZone: "zone-1"},
		Annotations: map[string]string{common.HostMoidAnnotationKey: host},
	}}
	if cluster != "" {
		node.Annotations[podvmv2.NodeTypeAnnotation] = podvmv2.VirtualNodeType
		node.Labels[podvmv2.VCenterUUIDLabel] = "vc-uuid"
		node.Labels[podvmv2.ClusterMoIDLabel] = cluster
	}
	return node
}

func TestVirtualNodeInventoryMixedHostUpgrade(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	hosts := []string{"host-1", "host-2", "host-3"}
	var queryErr error
	h := &hostInventory{client: client, clusterHosts: func(_ context.Context, uuid, cluster string) ([]string, error) {
		require.Equal(t, "vc-uuid", uuid)
		require.Equal(t, "domain-c1", cluster)
		return hosts, queryErr
	}}
	c := &K8sOrchestrator{hostInventory: h}
	create := func(node *corev1.Node) {
		t.Helper()
		_, err := client.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
		require.NoError(t, err)
	}
	check := func(want map[string]string) { t.Helper(); require.Equal(t, want, c.GetNodeIDtoNameMap(ctx)) }
	// No VirtualNode exists before host upgrade, even when the capability is enabled.
	for i := 1; i <= 3; i++ {
		create(inventoryNode(fmt.Sprintf("esx-%d", i), fmt.Sprintf("host-%d", i), ""))
	}
	legacy := map[string]string{"host-1": "esx-1", "host-2": "esx-2", "host-3": "esx-3"}
	check(legacy)
	// Names carry no identity. Ignore the VirtualNode's masqueraded host annotation.
	create(inventoryNode("compute-a", "host-masquerade", "domain-c1"))
	check(legacy)
	require.NoError(t, client.CoreV1().Nodes().Delete(ctx, "esx-1", metav1.DeleteOptions{}))
	check(map[string]string{"host-1": "compute-a", "host-2": "esx-2", "host-3": "esx-3"})
	for i := 2; i <= 3; i++ {
		require.NoError(t, client.CoreV1().Nodes().Delete(ctx, fmt.Sprintf("esx-%d", i), metav1.DeleteOptions{}))
	}
	check(map[string]string{"host-1": "compute-a", "host-2": "compute-a", "host-3": "compute-a"})
	require.Empty(t, c.GetNodeNameToHostMoIDMap(ctx), "singleton accessor must not choose a cluster member")
	mutated := c.GetNodeIDtoNameMap(ctx)
	mutated["host-1"] = "wrong"
	hosts = []string{"host-2"} // vCenter cluster membership is refreshed each read.
	check(map[string]string{"host-2": "compute-a"})
	queryErr = fmt.Errorf("vCenter unavailable")
	check(map[string]string{}) // Never reuse previous host membership on query failure.
	queryErr = nil
	check(map[string]string{"host-2": "compute-a"})
	require.NoError(t, client.CoreV1().Nodes().Delete(ctx, "compute-a", metav1.DeleteOptions{}))
	check(map[string]string{})
}

func TestVirtualNodeInventoryIncomplete(t *testing.T) {
	for _, scenario := range []string{"missing zone", "missing uuid", "missing cluster", "deleted", "unknown type", "node list fails", "duplicate cluster", "conflicting hosts", "vc fails"} {
		t.Run(scenario, func(t *testing.T) {
			node := inventoryNode("compute-a", "masquerade", "domain-c1")
			var extra []runtime.Object
			switch scenario {
			case "missing zone":
				delete(node.Labels, corev1.LabelTopologyZone)
			case "missing uuid":
				delete(node.Labels, podvmv2.VCenterUUIDLabel)
			case "missing cluster":
				delete(node.Labels, podvmv2.ClusterMoIDLabel)
			case "deleted":
				now := metav1.Now()
				node.DeletionTimestamp = &now
			case "unknown type":
				node.Annotations[podvmv2.NodeTypeAnnotation] = "entity-group"
			case "duplicate cluster":
				extra = append(extra, inventoryNode("compute-b", "", "domain-c1"))
			case "conflicting hosts":
				extra = append(extra, inventoryNode("compute-b", "", "domain-c2"))
			}
			client := fake.NewSimpleClientset(append(extra, node)...)
			if scenario == "node list fails" {
				client.PrependReactor("list", "nodes", func(clienttesting.Action) (bool, runtime.Object, error) { return true, nil, fmt.Errorf("unavailable") })
			}
			h := &hostInventory{client: client, clusterHosts: func(context.Context, string, string) ([]string, error) {
				if scenario == "vc fails" {
					return nil, fmt.Errorf("unavailable")
				}
				return []string{"host-1"}, nil
			}}
			require.Empty(t, h.snapshot(context.Background()))
		})
	}
}

func TestDedicatedNodesReserveHostsDuringHandoff(t *testing.T) {
	for _, scenario := range []string{"dedicated", "deleting", "unknown type", "conflicting dedicated"} {
		t.Run(scenario, func(t *testing.T) {
			node := inventoryNode("virtual-node-legacy-name", "host-1", "")
			objects := []runtime.Object{node, inventoryNode("compute-a", "", "domain-c1")}
			switch scenario {
			case "deleting":
				now := metav1.Now()
				node.DeletionTimestamp = &now
			case "unknown type":
				node.Annotations[podvmv2.NodeTypeAnnotation] = "entity-group"
			case "conflicting dedicated":
				objects = append(objects, inventoryNode("duplicate", "host-1", ""))
			}
			h := &hostInventory{client: fake.NewSimpleClientset(objects...), clusterHosts: func(context.Context, string, string) ([]string, error) { return []string{"host-1", "host-2"}, nil }}
			want := map[string]string{"host-2": "compute-a"}
			if scenario == "dedicated" {
				want["host-1"] = node.Name
			}
			require.Equal(t, want, h.snapshot(context.Background()))
		})
	}
}

func TestClusterHostsFromVCenter(t *testing.T) {
	ctx := context.Background()
	model := simulator.VPX()
	model.ClusterHost = 3
	require.NoError(t, model.Create())
	defer model.Remove()
	model.Service.TLS = new(tls.Config)
	model.Service.RegisterEndpoints = true
	server := model.Service.NewServer()
	defer server.Close()
	confPath := filepath.Join(t.TempDir(), "csi.conf")
	require.NoError(t, os.WriteFile(confPath, []byte("[Global]\ncluster-id = \"test\"\n[VirtualCenter \"127.0.0.1\"]\nuser = \"user@vsphere.local\"\npassword = \"pass\"\ndatacenters = \"DC0\"\ninsecure-flag = \"true\"\n"), 0600))
	t.Setenv("VSPHERE_CSI_CONFIG", confPath)
	t.Setenv("CLUSTER_FLAVOR", "VANILLA")
	port, err := strconv.Atoi(server.URL.Port())
	require.NoError(t, err)
	vc := &vsphere.VirtualCenter{ClientMutex: &sync.Mutex{}, Config: &vsphere.VirtualCenterConfig{
		Host: commontypes.NewFQDN(server.URL.Hostname()), Port: port, Username: "user", Password: "pass", Insecure: true,
	}}
	require.NoError(t, vc.Connect(ctx))
	defer func() { _ = vc.Disconnect(context.Background()) }()
	finder := find.NewFinder(vc.Client.Client, false)
	dc, err := finder.DefaultDatacenter(ctx)
	require.NoError(t, err)
	clusters, err := finder.SetDatacenter(dc).ClusterComputeResourceList(ctx, "*")
	require.NoError(t, err)
	require.Len(t, clusters, 1)
	cluster := clusters[0].Reference().Value
	uuid := vc.Client.ServiceContent.About.InstanceUuid
	want, err := clusters[0].Hosts(ctx)
	require.NoError(t, err)
	var ids []string
	for _, host := range want {
		ids = append(ids, host.Reference().Value)
	}
	got, err := clusterHostsFromVC(ctx, []*vsphere.VirtualCenter{vc}, uuid, cluster)
	require.NoError(t, err)
	require.Len(t, got, 3)
	require.ElementsMatch(t, ids, got)
	for _, tc := range []struct {
		name, uuid, cluster string
		vcs                 []*vsphere.VirtualCenter
	}{
		{"wrong vcenter", "another-uuid", cluster, []*vsphere.VirtualCenter{vc}},
		{"missing cluster", uuid, "domain-does-not-exist", []*vsphere.VirtualCenter{vc}},
		{"empty uuid", "", cluster, []*vsphere.VirtualCenter{vc}},
		{"not initialized", uuid, cluster, nil},
		{"multiple vcenters", uuid, cluster, []*vsphere.VirtualCenter{vc, vc}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hosts, err := clusterHostsFromVC(ctx, tc.vcs, tc.uuid, tc.cluster)
			require.Error(t, err)
			require.Empty(t, hosts)
		})
	}
}
