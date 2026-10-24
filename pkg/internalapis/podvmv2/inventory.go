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

package podvmv2

const (
	// TODO: Replace these provisional label keys with the VirtualNode producer's
	// finalized vCenter instance UUID and ClusterComputeResource MoID metadata.
	// The two values form one identity; cluster MoIDs are scoped to a vCenter.
	VCenterUUIDLabel = "pod.vmware.com/vcenter-instance-uuid"
	ClusterMoIDLabel = "pod.vmware.com/cluster-moid"
)
