/*
Copyright 2019 The Kubernetes Authors.

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

package vanilla

import (
	"context"
	"fmt"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"sigs.k8s.io/vsphere-csi-driver/pkg/apis/migration"
	"sigs.k8s.io/vsphere-csi-driver/pkg/csi/service/common"
	"sigs.k8s.io/vsphere-csi-driver/pkg/csi/service/logger"
)

// validateVanillaDeleteVolumeRequest is the helper function to validate
// DeleteVolumeRequest for Vanilla CSI driver.
// Function returns error if validation fails otherwise returns nil.
func validateVanillaDeleteVolumeRequest(ctx context.Context, req *csi.DeleteVolumeRequest) error {
	return common.ValidateDeleteVolumeRequest(ctx, req)

}

// validateControllerPublishVolumeRequest is the helper function to validate
// ControllerPublishVolumeRequest. Function returns error if validation fails otherwise returns nil.
func validateVanillaControllerPublishVolumeRequest(ctx context.Context, req *csi.ControllerPublishVolumeRequest) error {
	return common.ValidateControllerPublishVolumeRequest(ctx, req)
}

// validateControllerUnpublishVolumeRequest is the helper function to validate
// ControllerUnpublishVolumeRequest. Function returns error if validation fails otherwise returns nil.
func validateVanillaControllerUnpublishVolumeRequest(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) error {
	return common.ValidateControllerUnpublishVolumeRequest(ctx, req)
}

// validateVanillaControllerExpandVolumeRequest is the helper function to validate
// ExpandVolumeRequest for Vanilla CSI driver.
// Function returns error if validation fails otherwise returns nil.
func validateVanillaControllerExpandVolumeRequest(ctx context.Context, req *csi.ControllerExpandVolumeRequest) error {
	return common.ValidateControllerExpandVolumeRequest(ctx, req)
}

func registerVolume(ctx context.Context, volumePath string, c *controller) (string, error) {
	log := logger.GetLogger(ctx)
	var registeredVolumeID string
	volumeMigrationSpec, err := volumeMigrationService.GetVolumeInfo(ctx, volumePath)
	if err == migration.ErrVolumeNotRegistered {
		log.Infof("Registering in-tree volume: %q", volumePath)
		volumeMigration, err := volumeMigrationService.RegisterVolume(ctx, volumePath, c.manager)
		if err != nil {
			msg := fmt.Sprintf("failed to register in-tree volume: %+q err %+v", volumePath, err)
			log.Error(msg)
			return "", status.Errorf(codes.Internal, msg)
		}
		volumeMigrationService.SaveVolumeInfo(ctx, volumeMigration)
		registeredVolumeID = volumeMigration.Spec.VolumeID
	} else {
		registeredVolumeID = volumeMigrationSpec.VolumeID
	}
	return registeredVolumeID, nil
}
