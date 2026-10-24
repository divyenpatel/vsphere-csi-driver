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

package cnsregistervolume

import (
	"context"
	"encoding/json"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/csi/service/common"
)

type registrationHostMetadata struct {
	Host string
}

func registeredHostTopology(topology []map[string]string, host string) ([]map[string]string, error) {
	if host == "" || len(topology) != 1 || topology[0][v1.LabelTopologyZone] == "" || topology[0][v1.LabelHostname] == "" {
		return nil, fmt.Errorf("host-local registration has incomplete backing-host topology")
	}
	// HostLocalStorage uses a durable pin rather than hostname affinity even
	// before PodVM v2 is available, so later Node-model changes remain valid.
	return []map[string]string{{v1.LabelTopologyZone: topology[0][v1.LabelTopologyZone]}}, nil
}

// setRegisteredPVHost is used only while constructing a new PV, including the
// volume-mode recreation path. CSI volume attributes are immutable after creation.
func setRegisteredPVHost(pv *v1.PersistentVolume, host string) {
	if host == "" {
		return
	}
	if pv.Spec.CSI.VolumeAttributes == nil {
		pv.Spec.CSI.VolumeAttributes = make(map[string]string)
	}
	pv.Spec.CSI.VolumeAttributes[common.AttributeESXiHostMoID] = host
	metav1.SetMetaDataAnnotation(&pv.ObjectMeta, common.AnnPodVMHostPinned, host)
}

func reconcileRegisteredPVHost(ctx context.Context, client kubernetes.Interface, pv *v1.PersistentVolume,
	host string) (*v1.PersistentVolume, error) {
	if actual := pv.Spec.CSI.VolumeAttributes[common.AttributeESXiHostMoID]; actual != "" && actual != host {
		return nil, fmt.Errorf("PV backing host conflicts with registered volume")
	}
	if pinned := pv.Annotations[common.AnnPodVMHostPinned]; pinned != "" && pinned != host {
		return nil, fmt.Errorf("PV host pin conflicts with registered volume")
	}
	if pv.Annotations[common.AnnPodVMHostPinned] == host {
		return pv, nil
	}
	// Reconcile only mutable metadata on reused PVs. Do not modify immutable
	// volume attributes or populated affinity; old hostname-affinity PVs stay old.
	patch, err := json.Marshal(map[string]interface{}{"metadata": map[string]interface{}{
		"resourceVersion": pv.ResourceVersion, "uid": pv.UID,
		"annotations": map[string]string{common.AnnPodVMHostPinned: host},
	}})
	if err != nil {
		return nil, err
	}
	return client.CoreV1().PersistentVolumes().Patch(ctx, pv.Name, types.MergePatchType, patch, metav1.PatchOptions{})
}

func publishRegisteredPVCHost(ctx context.Context, client kubernetes.Interface, pvc *v1.PersistentVolumeClaim,
	host string, topology []map[string]string) (*v1.PersistentVolumeClaim, error) {
	if actual := pvc.Annotations[common.AttributeESXiHostMoID]; actual != "" && actual != host {
		return nil, fmt.Errorf("PVC backing host conflicts with registered volume")
	}
	annotations := map[string]string{common.AttributeESXiHostMoID: host}
	if pvc.Annotations[common.AnnVolumeAccessibleTopology] == "" {
		data, err := json.Marshal(topology)
		if err != nil {
			return nil, err
		}
		annotations[common.AnnVolumeAccessibleTopology] = string(data)
	} else if pvc.Annotations[common.AttributeESXiHostMoID] == host {
		return pvc, nil
	}
	patch, err := json.Marshal(map[string]interface{}{"metadata": map[string]interface{}{
		"resourceVersion": pvc.ResourceVersion, "uid": pvc.UID, "annotations": annotations,
	}})
	if err != nil {
		return nil, err
	}
	return client.CoreV1().PersistentVolumeClaims(pvc.Namespace).Patch(ctx, pvc.Name, types.MergePatchType, patch, metav1.PatchOptions{})
}
