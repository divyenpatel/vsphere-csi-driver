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
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/csi/service/common"
)

func TestPhysicalHostCandidates(t *testing.T) {
	inventory := map[string]map[string]string{
		"host-1": {v1.LabelTopologyZone: "zone-1", v1.LabelHostname: "virtual-node-domain-c1"},
		"host-2": {v1.LabelTopologyZone: "zone-1", v1.LabelHostname: "virtual-node-domain-c1"},
		"host-3": {v1.LabelTopologyZone: "zone-2", v1.LabelHostname: "esx-node-3"},
	}
	req := &csi.TopologyRequirement{Requisite: []*csi.Topology{
		{Segments: map[string]string{v1.LabelTopologyZone: "zone-1"}},
		{Segments: map[string]string{v1.LabelTopologyZone: "zone-2"}},
	}}
	for _, tc := range []struct {
		name, raw string
		want      []string
		fail      bool
	}{
		{name: "physical pair", raw: `[{"topology.kubernetes.io/zone":"zone-1","topology.csi.vmware.com/host":"host-1"}]`, want: []string{"host-1"}},
		{name: "same zone alternative remains eligible", raw: `[{"topology.kubernetes.io/zone":"zone-1","topology.csi.vmware.com/host":"host-1"},{"topology.kubernetes.io/zone":"zone-1"}]`, want: []string{"host-1", "host-2"}},
		{name: "cross zone pairs", raw: `[{"topology.kubernetes.io/zone":"zone-1","topology.csi.vmware.com/host":"host-1"},{"topology.kubernetes.io/zone":"zone-2","topology.csi.vmware.com/host":"host-3"}]`, want: []string{"host-1", "host-3"}},
		{name: "virtual node expands", raw: `[{"topology.kubernetes.io/zone":"zone-1","kubernetes.io/hostname":"virtual-node-domain-c1"}]`, want: []string{"host-1", "host-2"}},
		{name: "zone only expands", want: []string{"host-1", "host-2", "host-3"}},
		{name: "wrong zone cannot match", raw: `[{"topology.kubernetes.io/zone":"zone-2","topology.csi.vmware.com/host":"host-1"}]`, fail: true},
		{name: "unknown host cannot widen", raw: `[{"topology.kubernetes.io/zone":"zone-1","topology.csi.vmware.com/host":"host-9"}]`, fail: true},
		{name: "missing zone", raw: `[{"topology.csi.vmware.com/host":"host-1"}]`, fail: true},
		{name: "empty host", raw: `[{"topology.kubernetes.io/zone":"zone-1","topology.csi.vmware.com/host":""}]`, fail: true},
		{name: "unsupported key", raw: `[{"topology.kubernetes.io/zone":"zone-1","rack":"r1"}]`, fail: true},
		{name: "empty alternatives", raw: `[]`, fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			refs, topologies, err := hostLocalCandidates(req, tc.raw, inventory)
			if tc.fail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			var got []string
			for _, ref := range refs {
				got = append(got, ref.Value)
				require.Equal(t, "HostSystem", ref.Type)
			}
			require.Equal(t, tc.want, got)
			for host, topology := range topologies {
				require.Equal(t, inventory[host], topology)
				topology[v1.LabelHostname] = "mutated"
				require.NotEqual(t, "mutated", inventory[host][v1.LabelHostname])
			}
		})
	}
	// A physical alternative is not permission to escape CSI's requisite zones.
	limited := &csi.TopologyRequirement{Requisite: req.Requisite[:1]}
	_, _, err := hostLocalCandidates(limited, `[{"topology.kubernetes.io/zone":"zone-2","topology.csi.vmware.com/host":"host-3"}]`, inventory)
	require.Error(t, err)
	require.True(t, validateCreateBlockReqParam(common.AttributeHostLocalTopology, "[]"))
	require.False(t, validateCreateFileReqParam(common.AttributeHostLocalTopology, "[]"))
}
