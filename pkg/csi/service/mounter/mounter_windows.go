//go:build windows
// +build windows

/*
Copyright 2021 The Kubernetes Authors.
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

package mounter

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/davecgh/go-spew/spew"
	windowsdiskAPI "sigs.k8s.io/vsphere-csi-driver/v2/pkg/csi/service/osutils/windows/disk"
	windowsfilesystemAPI "sigs.k8s.io/vsphere-csi-driver/v2/pkg/csi/service/osutils/windows/filesystem"
	windowssystemAPI "sigs.k8s.io/vsphere-csi-driver/v2/pkg/csi/service/osutils/windows/system"
	windowsvolumeAPI "sigs.k8s.io/vsphere-csi-driver/v2/pkg/csi/service/osutils/windows/volume"

	"sigs.k8s.io/vsphere-csi-driver/v2/pkg/csi/service/logger"

	"k8s.io/mount-utils"
	utilexec "k8s.io/utils/exec"
)

// black assignment is used to check if it can be cast
var _ CSIProxyMounter = &csiWindowsMounter{}

type csiWindowsMounter struct {
	Ctx   context.Context
	FSAPI *windowsfilesystemAPI.FilesystemAPI
	//FsClient     *fsclient.Client
	DiskAPI *windowsdiskAPI.DiskAPI
	//DiskClient   *diskclient.Client
	VolumeAPI *windowsvolumeAPI.VolumeAPI
	//VolumeClient *volumeclient.Client
	SystemAPI *windowssystemAPI.APIImplementor
	//SystemClient *systemClient.Client
}

// CSIProxyMounter extends the mount.Interface interface with CSI Proxy methods.
// In future this functions are supposed to be implemented in mount.Interface for windows
type CSIProxyMounter interface {
	mount.Interface
	// ExistsPath - Checks if a path exists. This call does not perform follow link.
	ExistsPath(path string) (bool, error)
	// FormatAndMount - accepts the source disk number, target path to mount, the fstype to format with and options to be used.
	FormatAndMount(source, target, fstype string, options []string) error
	// Rmdir - delete the given directory
	Rmdir(path string) error
	// MakeDir - Creates a directory.
	MakeDir(pathname string) error
	// Rescan would trigger an update storage cache via the CSI proxy.
	Rescan() error
	// GetDeviceNameFromMount returns the volume ID for a mount path.
	GetDeviceNameFromMount(mountPath string) (string, error)
	// Get the size in bytes for Volume
	GetVolumeSizeInBytes(devicePath string) (int64, error)
	// ResizeVolume resizes the volume to the maximum available size.
	ResizeVolume(devicePath string, sizeInBytes int64) error
	// Gets windows specific disk number from diskId
	GetDiskNumber(diskID string) (string, error)
	// Get the size of the disk in bytes
	GetDiskTotalBytes(devicePath string) (int64, error)
	// StatFS returns info about volume
	StatFS(path string) (available, capacity, used, inodesFree, inodes, inodesUsed int64, err error)
	// GetBIOSSerialNumber - Get bios serial number
	GetBIOSSerialNumber() (string, error)
}

// NewSafeMounter returns mounter with exec
func NewSafeMounter(ctx context.Context) (*mount.SafeFormatAndMount, error) {
	csiWindowsMounter, err := newCSIProxyMounter(ctx)
	log := logger.GetLogger(ctx)
	if err == nil {
		log.Infof("using csiWindowsMounter")
		return &mount.SafeFormatAndMount{
			Interface: csiWindowsMounter,
			Exec:      utilexec.New(),
		}, nil
	}
	return nil, err
}

// newCSIProxyMounter - creates a new CSI Proxy mounter struct which encompassed all the
// clients to the CSI proxy - filesystem, disk and volume clients.
func newCSIProxyMounter(ctx context.Context) (*csiWindowsMounter, error) {
	fsAPI := windowsfilesystemAPI.FilesystemAPI{}
	//fsClient, err := fsclient.NewClient()
	//if err != nil {
	//	return nil, err
	//}
	diskAPI := windowsdiskAPI.DiskAPI{}
	//diskClient, err := diskclient.NewClient()
	//if err != nil {
	//	return nil, err
	//}

	volumeAPI := windowsvolumeAPI.VolumeAPI{}

	//volumeClient, err := volumeclient.NewClient()
	//if err != nil {
	//	return nil, err
	//}

	systemAPI := windowssystemAPI.New()

	//systemClient, err := systemClient.NewClient()
	//if err != nil {
	//	return nil, err
	//}
	return &csiWindowsMounter{
		FSAPI:     &fsAPI,
		DiskAPI:   &diskAPI,
		VolumeAPI: &volumeAPI,
		SystemAPI: &systemAPI,
		Ctx:       ctx,
	}, nil
}

// normalizeWindowsPath normalizes windows path
func normalizeWindowsPath(path string) string {
	normalizedPath := strings.Replace(path, "/", "\\", -1)
	if strings.HasPrefix(normalizedPath, "\\") {
		normalizedPath = "c:" + normalizedPath
	}
	return normalizedPath
}

// ExistsPath - Checks if a path exists. This call does not perform follow link.
func (mounter *csiWindowsMounter) ExistsPath(path string) (bool, error) {
	ctx := mounter.Ctx
	log := logger.GetLogger(ctx)
	isExists, err := mounter.FSAPI.PathExists(normalizeWindowsPath(path))
	if err != nil {
		log.Errorf("Proxy returned error while checking if PathExists: %v", err)
		return false, err
	}
	return isExists, err
}

// Rmdir - delete the given directory
func (mounter *csiWindowsMounter) Rmdir(path string) error {
	err := mounter.FSAPI.Rmdir(normalizeWindowsPath(path), true)
	if err != nil {
		return err
	}
	return nil
}

// MakeDir - Creates a directory.
func (mounter *csiWindowsMounter) MakeDir(pathname string) error {
	ctx := mounter.Ctx
	log := logger.GetLogger(ctx)
	err := mounter.FSAPI.Mkdir(normalizeWindowsPath(pathname))
	if err != nil {
		log.Infof("Error: %v", err)
		return err
	}
	return nil
}

// Gets windows specific disk number from diskId
func (mounter *csiWindowsMounter) GetDiskNumber(diskID string) (string, error) {
	ctx := mounter.Ctx
	log := logger.GetLogger(ctx)
	// Check device is attached
	log.Debug("GetDiskNumber called with diskID: %q", diskID)

	diskIDs, err := mounter.DiskAPI.ListDiskIDs()
	if err != nil {
		log.Debug("Could not get diskids %s", err)
		return "", err
	}
	spew.Dump("disIDs: ", diskIDs)
	for diskNum, diskInfo := range diskIDs {
		log.Infof("found disk number %d, disk info %v", diskNum, diskInfo)
		ID := diskInfo.Page83
		if ID == "" {
			continue
		}
		if ID == diskID {
			log.Infof("Found disk number: %d with diskID: %s", diskNum, diskID)
			return strconv.FormatUint(uint64(diskNum), 10), nil
		}
	}
	return "", errors.New("no matching disks found")
}

// IsLikelyMountPoint - If the directory does not exists, the function will return os.ErrNotExist error.
// If the path exists, call to CSI proxy will check if its a link, if its a link then existence of target
// path is checked.
func (mounter *csiWindowsMounter) IsLikelyNotMountPoint(path string) (bool, error) {
	isExists, err := mounter.ExistsPath(path)
	if err != nil {
		return false, err
	}

	if !isExists {
		return true, os.ErrNotExist
	}

	isSymlink, err := mounter.FSAPI.IsSymlink(normalizeWindowsPath(path))
	if err != nil {
		return false, err
	}
	return !isSymlink, nil
	//TODO check if formatted else error out
}

// FormatAndMount - accepts the source disk number, target path to mount, the fstype to format with and options to be used.
func (mounter *csiWindowsMounter) FormatAndMount(source string, target string, fstype string, options []string) error {
	ctx := mounter.Ctx
	log := logger.GetLogger(ctx)
	diskNum, err := strconv.Atoi(source)
	if err != nil {
		return fmt.Errorf("parse %s failed with error: %v", source, err)
	}
	log.Infof("Disk Number: %d", diskNum)

	if err = mounter.DiskAPI.CreateBasicPartition(uint32(diskNum)); err != nil {
		return err
	}

	// ensure disk is online
	log.Infof("setting disk %d to online", diskNum)
	err = mounter.DiskAPI.SetDiskState(uint32(diskNum), true)
	if err != nil {
		return err
	}

	// List the volumes on the given disk.
	volumeIds, err := mounter.VolumeAPI.ListVolumesOnDisk(uint32(diskNum), 0)
	if err != nil {
		return err
	}

	// TODO: consider partitions and choose the right partition.
	// For now just choose the first volume.
	volumeID := volumeIds[0]
	log.Infof("volumeIdResponse : %v", volumeIds)
	log.Infof("volumeID : %s", volumeID)
	// Check if the volume is formatted.
	isVolumeFormatted, err := mounter.VolumeAPI.IsVolumeFormatted(volumeID)
	if err != nil {
		return err
	}

	// If the volume is not formatted, then format it, else proceed to mount.
	if !isVolumeFormatted {
		log.Infof("volumeID is not formatted : %s", volumeID)
		err = mounter.VolumeAPI.FormatVolume(volumeID)
		if err != nil {
			return err
		}
	}

	err = mounter.VolumeAPI.MountVolume(volumeID, normalizeWindowsPath(target))
	if err != nil {
		return err
	}
	log.Infof("Volume mounted")
	return nil
}

// Unmount - Removes the directory - equivalent to unmount on Linux.
func (mounter *csiWindowsMounter) Unmount(target string) error {
	// unmount internally calls WriteVolumeCache so no need to WriteVolumeCache
	// normalize target path
	target = normalizeWindowsPath(target)
	if exists, err := mounter.ExistsPath(target); !exists {
		return err
	}
	// get volume id
	volumeId, err := mounter.VolumeAPI.GetVolumeIDFromTargetPath(target)
	if err != nil {
		return err
	}

	// unmount volume
	err = mounter.VolumeAPI.UnmountVolume(volumeId, target)
	if err != nil {
		return err
	}

	// remove the target directory
	err = mounter.Rmdir(target)
	if err != nil {
		return err
	}

	// Set disk to offline mode to have a clean state
	diskNumber, err := mounter.VolumeAPI.GetDiskNumberFromVolumeID(volumeId)
	if err != nil {
		return err
	}
	if err = mounter.DiskAPI.SetDiskState(diskNumber, false); err != nil {
		return err
	}
	return nil
}

// Mount just creates a soft link at target pointing to source.
func (mounter *csiWindowsMounter) Mount(source string, target string, fstype string, options []string) error {
	// Mount is called after the format is done.
	// TODO: Confirm that fstype is empty.
	err := mounter.FSAPI.CreateSymlink(normalizeWindowsPath(source), normalizeWindowsPath(target))
	if err != nil {
		return err
	}
	return nil
}

// GetDeviceNameFromMount returns the volume ID for a mount path.
func (mounter *csiWindowsMounter) GetDeviceNameFromMount(mountPath string) (string, error) {
	ctx := mounter.Ctx
	log := logger.GetLogger(ctx)
	volumeID, err := mounter.VolumeAPI.GetVolumeIDFromTargetPath(normalizeWindowsPath(mountPath))
	if err != nil {
		return "", err
	}
	log.Infof("Device path for mount Path: %s: %s", mountPath, volumeID)
	return volumeID, nil
}

// ResizeVolume resizes the volume to the maximum available size.
// sizeInBytes is ignored in this function as windows is not resizing to full capacity
func (mounter *csiWindowsMounter) ResizeVolume(devicePath string, sizeInBytes int64) error {
	// Set disk to online mode before resize
	diskNumber, err := mounter.VolumeAPI.GetDiskNumberFromVolumeID(devicePath)
	if err != nil {
		return err
	}
	if err = mounter.DiskAPI.SetDiskState(diskNumber, true); err != nil {
		return err
	}
	err = mounter.VolumeAPI.ResizeVolume(devicePath, 0)
	return err
}

// Get the size in bytes for Volume
func (mounter *csiWindowsMounter) GetVolumeSizeInBytes(volumeId string) (int64, error) {
	volumeSize, _, err := mounter.VolumeAPI.GetVolumeStats(volumeId)
	if err != nil {
		return -1, err
	}
	return volumeSize, nil
}

// Rescan would trigger an update storage cache via the CSI proxy.
func (mounter *csiWindowsMounter) Rescan() error {
	// Call Rescan from disk APIs of CSI Proxy.
	ctx := mounter.Ctx
	log := logger.GetLogger(ctx)
	log.Infof("Calling CSI Proxy's rescan API")
	if err := mounter.DiskAPI.Rescan(); err != nil {
		return err
	}
	return nil
}

// Get the size of the disk in bytes
func (mounter *csiWindowsMounter) GetDiskTotalBytes(volumeId string) (int64, error) {
	diskNumber, err := mounter.VolumeAPI.GetDiskNumberFromVolumeID(volumeId)
	if err != nil {
		return -1, err
	}

	totalBytes, err := mounter.DiskAPI.GetDiskStats(diskNumber)
	return totalBytes, err
}

// StatFS returns info about volume
func (mounter *csiWindowsMounter) StatFS(path string) (available, capacity, used, inodesFree, inodes, inodesUsed int64, err error) {
	zero := int64(0)

	volumeID, err := mounter.VolumeAPI.GetVolumeIDFromTargetPath(path)
	if err != nil {
		return zero, zero, zero, zero, zero, zero, err
	}

	volumeSize, volumeUsedSize, err := mounter.VolumeAPI.GetVolumeStats(volumeID)
	if err != nil {
		return zero, zero, zero, zero, zero, zero, err
	}
	capacity = volumeSize
	used = volumeUsedSize
	available = capacity - used
	return available, capacity, used, zero, zero, zero, nil
}

// umimplemented methods of mount.Interface
func (mounter *csiWindowsMounter) GetMountRefs(pathname string) ([]string, error) {
	return []string{}, fmt.Errorf("GetMountRefs is not implemented for csiProxyMounter")
}

func (mounter *csiWindowsMounter) GetFSGroup(pathname string) (int64, error) {
	return -1, fmt.Errorf("GetFSGroup is not implemented for csiProxyMounter")
}

func (mounter *csiWindowsMounter) MountSensitive(source string, target string, fstype string, options []string, sensitiveOptions []string) error {
	return fmt.Errorf("MountSensitive is not implemented for csiProxyMounter")
}

func (mounter *csiWindowsMounter) MountSensitiveWithoutSystemd(source string, target string, fstype string, options []string, sensitiveOptions []string) error {
	return fmt.Errorf("MountSensitiveWithoutSystemd is not implemented for csiProxyMounter")
}

func (mounter *csiWindowsMounter) MountSensitiveWithoutSystemdWithMountFlags(source string, target string, fstype string, options []string, sensitiveOptions []string, mountFlags []string) error {
	return mounter.MountSensitive(source, target, fstype, options, sensitiveOptions /* sensitiveOptions */)
}

func (mounter *csiWindowsMounter) List() ([]mount.MountPoint, error) {
	return []mount.MountPoint{}, fmt.Errorf("List not implemented for csiProxyMounter")
}

// GetBIOSSerialNumber - Get bios serial number
func (mounter *csiWindowsMounter) GetBIOSSerialNumber() (string, error) {
	ctx := mounter.Ctx
	log := logger.GetLogger(ctx)

	serialNo, err := mounter.SystemAPI.GetBIOSSerialNumber()
	if err != nil {
		log.Errorf("Proxy returned error while checking serialNoResponse: %v", err)
		return "", err
	}
	return serialNo, err
}
