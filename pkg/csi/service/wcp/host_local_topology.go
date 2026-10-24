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
	"encoding/json"
	"fmt"
	"maps"
	"sort"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	v1 "k8s.io/api/core/v1"
	cnsvsphere "sigs.k8s.io/vsphere-csi-driver/v3/pkg/common/cns-lib/vsphere"
	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/csi/service/common"
	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/csi/service/common/commonco"
)

// hostLocalCandidates intersects each physical alternative with namespace-active
// inventory and normal CSI zone requirements. It never flattens host/zone pairs.
func hostLocalCandidates(requirement *csi.TopologyRequirement, raw string,
	inventory map[string]map[string]string) ([]vimtypes.ManagedObjectReference, map[string]map[string]string, error) {
	var alternatives []map[string]string
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &alternatives); err != nil {
			return nil, nil, fmt.Errorf("invalid physical topology: %w", err)
		}
	} else {
		topologies := requirement.GetPreferred()
		if len(topologies) == 0 {
			topologies = requirement.GetRequisite()
		}
		for _, topology := range topologies {
			alternatives = append(alternatives, topology.GetSegments())
		}
	}
	if len(alternatives) == 0 {
		return nil, nil, fmt.Errorf("host-local provisioning requires topology alternatives")
	}
	allowed := requirement.GetRequisite()
	if len(allowed) == 0 {
		allowed = requirement.GetPreferred()
	}
	selected := make(map[string]map[string]string)
	for _, segment := range alternatives {
		zone := segment[v1.LabelTopologyZone]
		if zone == "" {
			return nil, nil, fmt.Errorf("host-local topology requires a zone")
		}
		host, hasHost := segment[common.GuestClusterTopologyLabelHost]
		node, hasNode := segment[v1.LabelHostname]
		if (hasHost && host == "") || (hasNode && node == "") || (hasHost && hasNode) {
			return nil, nil, fmt.Errorf("invalid host constraint in topology %v", segment)
		}
		for key := range segment {
			if key != v1.LabelTopologyZone && key != common.GuestClusterTopologyLabelHost && key != v1.LabelHostname {
				return nil, nil, fmt.Errorf("unsupported physical topology key %q", key)
			}
		}
		for candidate, topology := range inventory {
			if topology[v1.LabelTopologyZone] != zone || (hasHost && host != candidate) || (hasNode && node != topology[v1.LabelHostname]) {
				continue
			}
			for _, normal := range allowed {
				// Normal topology can still carry a legacy Supervisor Node constraint.
				if normal.GetSegments()[v1.LabelTopologyZone] == zone &&
					(normal.GetSegments()[v1.LabelHostname] == "" || normal.GetSegments()[v1.LabelHostname] == topology[v1.LabelHostname]) {
					selected[candidate] = maps.Clone(topology)
					break
				}
			}
		}
	}
	if len(selected) == 0 {
		return nil, nil, fmt.Errorf("no namespace-active host satisfies the requested physical topology")
	}
	hosts := make([]string, 0, len(selected))
	for host := range selected {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	refs := make([]vimtypes.ManagedObjectReference, 0, len(hosts))
	for _, host := range hosts {
		refs = append(refs, vimtypes.ManagedObjectReference{Type: "HostSystem", Value: host})
	}
	return refs, selected, nil
}

// hostLocalInventory verifies host membership against vCenter, using active
// clusters for each namespace zone when the multi-cluster capability is enabled.
// CNS still evaluates storage policy compatibility and placement eligibility.
func (c *controller) hostLocalInventory(ctx context.Context, vc *cnsvsphere.VirtualCenter,
	namespace string, requirement *csi.TopologyRequirement) (map[string]map[string]string, error) {
	if namespace == "" {
		return nil, fmt.Errorf("PVC namespace metadata is required for host-local provisioning")
	}
	zones, err := GetZonesFromAccessibilityRequirements(ctx, requirement)
	if err != nil {
		return nil, err
	}
	if c.topologyMgr == nil {
		return nil, fmt.Errorf("topology manager is not initialized")
	}
	azClusters := c.topologyMgr.GetAZClustersMap(ctx)
	owners := commonco.ContainerOrchestratorUtility.GetNodeIDtoNameMap(ctx)
	result := make(map[string]map[string]string)
	for _, zone := range zones {
		clusters := azClusters[zone]
		if IsMultipleClustersPerVsphereZoneFSSEnabled {
			clusters, err = commonco.ContainerOrchestratorUtility.GetActiveClustersForNamespaceInRequestedZones(ctx, namespace, []string{zone})
			if err != nil {
				return nil, err
			}
		}
		for _, cluster := range clusters {
			clusterObj := &cnsvsphere.ClusterComputeResource{ClusterComputeResource: object.NewClusterComputeResource(vc.Client.Client,
				vimtypes.ManagedObjectReference{Type: "ClusterComputeResource", Value: cluster})}
			hosts, err := clusterObj.GetHosts(ctx)
			if err != nil {
				return nil, err
			}
			for _, host := range hosts {
				id := host.Reference().Value
				node := owners[id]
				if node == "" {
					continue
				}
				if previous := result[id]; previous != nil && previous[v1.LabelTopologyZone] != zone {
					return nil, fmt.Errorf("host %q belongs to conflicting zones", id)
				}
				result[id] = map[string]string{v1.LabelTopologyZone: zone, v1.LabelHostname: node}
			}
		}
	}
	return result, nil
}
