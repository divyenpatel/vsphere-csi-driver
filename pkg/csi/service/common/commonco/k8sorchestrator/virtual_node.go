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
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/common/cns-lib/vsphere"
	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/csi/service/common"
	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/csi/service/logger"
	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/internalapis/podvmv2"
)

// hostInventory reads core Nodes and vCenter cluster membership. It does not
// depend on a separate inventory CR or diagnostic host metadata on a Node.
// Live lists avoid treating an unsynchronized informer as evidence of ownership;
// no previous mapping is reused after a lookup fails or a host leaves a cluster.
type hostInventory struct {
	client       kubernetes.Interface
	clusterHosts func(context.Context, string, string) ([]string, error)
}

func queryClusterHosts(ctx context.Context, vcUUID, clusterMoID string) ([]string, error) {
	return clusterHostsFromVC(ctx, vsphere.GetVirtualCenterManager(ctx).GetAllVirtualCenters(), vcUUID, clusterMoID)
}

func clusterHostsFromVC(ctx context.Context, vcs []*vsphere.VirtualCenter, vcUUID, clusterMoID string) ([]string, error) {
	// Supervisor host topology currently uses bare MoIDs. Do not allow a second
	// vCenter's coincidentally equal host/cluster MoID to enter this namespace.
	if len(vcs) != 1 || vcUUID == "" || clusterMoID == "" {
		return nil, fmt.Errorf("host ownership requires one configured vCenter and a complete cluster identity")
	}
	vc := vcs[0]
	if err := vc.Connect(ctx); err != nil {
		return nil, err
	}
	if vc.Client.ServiceContent.About.InstanceUuid != vcUUID {
		return nil, fmt.Errorf("VirtualNode vCenter UUID %q does not match the configured vCenter", vcUUID)
	}
	hosts, err := vc.GetHostsByCluster(ctx, clusterMoID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(hosts))
	for _, host := range hosts {
		ids = append(ids, host.Reference().Value)
	}
	return ids, nil
}

func (h *hostInventory) snapshot(ctx context.Context) map[string]string {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	log := logger.GetLogger(ctx)
	owners := make(map[string]string)
	nodes, err := h.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Errorf("Cannot list Nodes for physical host ownership: %v", err)
		return owners
	}
	conflicts := make(map[string]bool)
	reserved := make(map[string]bool)
	add := func(host, node string) {
		if host == "" {
			return
		}
		if previous, exists := owners[host]; exists && previous != node {
			conflicts[host] = true
		}
		owners[host] = node
	}
	type clusterIdentity struct{ vcUUID, moID string }
	clusters := make(map[clusterIdentity][]*corev1.Node)
	for i := range nodes.Items {
		node := &nodes.Items[i]
		if podvmv2.IsVirtualNode(node) {
			// Even deleting/duplicate Nodes reserve their cluster identity, so a
			// replacement cannot claim the same hosts before handoff completes.
			identity := clusterIdentity{node.Labels[podvmv2.VCenterUUIDLabel], node.Labels[podvmv2.ClusterMoIDLabel]}
			if identity.vcUUID != "" && identity.moID != "" {
				clusters[identity] = append(clusters[identity], node)
			}
			continue // Ignore any legacy masqueraded single-host annotation.
		}
		host := node.Annotations[common.HostMoidAnnotationKey]
		if host == "" {
			continue
		}
		reserved[host] = true
		if podvmv2.IsDedicatedHostNode(node) && node.DeletionTimestamp == nil {
			add(host, node.Name)
		} else {
			// Neither an unsupported node type nor a terminating Node can be
			// interpreted as evidence that this host has joined a VirtualNode.
			conflicts[host] = true
		}
	}
	for identity, candidates := range clusters {
		if len(candidates) != 1 {
			continue
		}
		node := candidates[0]
		if node.DeletionTimestamp != nil || node.Labels[corev1.LabelTopologyZone] == "" {
			continue
		}
		hosts, err := h.clusterHosts(ctx, identity.vcUUID, identity.moID)
		if err != nil {
			log.Errorf("Cannot resolve hosts for VirtualNode %q: %v", node.Name, err)
			continue
		}
		for _, host := range hosts {
			// During mixed upgrades a surviving dedicated Node owns its host.
			// TODO: Confirm the platform guarantees that deleting that Node is
			// the handoff to the cluster-backed Node. vCenter membership alone
			// does not describe the scheduler's ownership transition.
			if !reserved[host] {
				add(host, node.Name)
			}
		}
	}
	for host := range conflicts {
		delete(owners, host)
	}
	return owners
}
