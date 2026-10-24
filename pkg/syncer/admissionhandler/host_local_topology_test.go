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

package admissionhandler

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/csi/service/common"
)

func TestNarrowRestoreTopologyPreservesAlternatives(t *testing.T) {
	raw := `[{"topology.kubernetes.io/zone":"zone-1","topology.csi.vmware.com/host":"host-1"},{"topology.kubernetes.io/zone":"zone-1"},{"topology.kubernetes.io/zone":"zone-2","topology.csi.vmware.com/host":"host-2"}]`
	narrowed, err := narrowRestoreTopologySegments(raw, map[string]bool{"zone-1": true}, "")
	require.NoError(t, err)
	var segments []map[string]string
	require.NoError(t, json.Unmarshal([]byte(narrowed), &segments))
	require.Equal(t, []map[string]string{
		{v1.LabelTopologyZone: "zone-1", common.GuestClusterTopologyLabelHost: "host-1"},
		{v1.LabelTopologyZone: "zone-1"},
	}, segments)
	narrowed, err = narrowRestoreTopologySegments(raw, map[string]bool{"zone-1": true}, "host-3")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(narrowed), &segments))
	require.Equal(t, []map[string]string{{v1.LabelTopologyZone: "zone-1", common.GuestClusterTopologyLabelHost: "host-3"}}, segments)
	_, err = narrowRestoreTopologySegments(`[{"topology.kubernetes.io/zone":"zone-1","topology.csi.vmware.com/host":"host-1"}]`, map[string]bool{"zone-1": true}, "host-2")
	require.Error(t, err)
	direct := `[{"topology.kubernetes.io/zone":"zone-1","kubernetes.io/hostname":"virtual-node-domain-c1"}]`
	narrowed, err = narrowRestoreTopologySegments(direct, map[string]bool{"zone-1": true}, "host-1")
	require.NoError(t, err)
	require.JSONEq(t, direct, narrowed)
}
