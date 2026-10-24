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
	"fmt"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/csi/service/common"
)

// Preserve complete requested segments while narrowing to source-accessible
// zones. A physical source host restricts local copy placement; the target
// policy and CreateVolume result still determine final volume accessibility.
func narrowRestoreTopologySegments(raw string, zones map[string]bool, sourceHost string) (string, error) {
	var requested []map[string]string
	if err := json.Unmarshal([]byte(raw), &requested); err != nil {
		return "", err
	}
	var result []map[string]string
	for _, segment := range requested {
		if !zones[segment[v1.LabelTopologyZone]] {
			continue
		}
		if sourceHost != "" {
			if host, present := segment[common.GuestClusterTopologyLabelHost]; present && host != sourceHost {
				continue
			}
			// Preserve a direct Supervisor Node constraint for WCP to intersect with
			// the snapshot's source host. Do not replace it or invent a Node name.
			if segment[v1.LabelHostname] == "" {
				segment[common.GuestClusterTopologyLabelHost] = sourceHost
			}
		}
		result = append(result, segment)
	}
	if len(result) == 0 {
		return "", fmt.Errorf("requested topology has no alternative compatible with the source volume")
	}
	encoded, err := json.Marshal(result)
	return string(encoded), err
}
