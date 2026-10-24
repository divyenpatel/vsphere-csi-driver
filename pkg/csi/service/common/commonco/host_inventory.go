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

package commonco

import (
	"context"
	"sort"
)

// GetNodeNameToHostMoIDsMap returns all member hosts per Kubernetes Node.
// This reverses a copied host-to-owner snapshot; callers must not assume one
// physical host per Node. Host identity is scoped to the current vCenter.
func GetNodeNameToHostMoIDsMap(ctx context.Context, co COCommonInterface) map[string][]string {
	result := make(map[string][]string)
	for host, node := range co.GetNodeIDtoNameMap(ctx) {
		result[node] = append(result[node], host)
	}
	for node := range result {
		sort.Strings(result[node])
	}
	return result
}
