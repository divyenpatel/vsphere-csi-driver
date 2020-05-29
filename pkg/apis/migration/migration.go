package migration

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	cnstypes "github.com/vmware/govmomi/cns/types"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	migration "sigs.k8s.io/vsphere-csi-driver/pkg/apis/migration/v1alpha1"
	"sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vsphere"
	"sigs.k8s.io/vsphere-csi-driver/pkg/csi/service/common"
	"sigs.k8s.io/vsphere-csi-driver/pkg/csi/service/logger"
	k8s "sigs.k8s.io/vsphere-csi-driver/pkg/kubernetes"
)

const (
	timeout  = 60 * time.Second
	pollTime = 5 * time.Second
)

// ErrVolumeNotRegistered is returned when requested volumepath is not found from cache and K8S CR List
var ErrVolumeNotRegistered = errors.New("volume is not registered to CNS")

// VolumeMigrationService exposes interfaces to manage VolumeInfo cache and CR
// for VolumePath, VolumeID and VolumeName mapping
type VolumeMigrationService interface {
	// GetVolumeInfo returns CnsvSphereVolumeMigrationSpec for given VolumePath from local cache
	// if CnsvSphereVolumeMigrationSpec is not available in the cache, K8s CR look up will be performed, if that too
	// can not retrieve VolumeInfo, ErrVolumeNotRegistered will be returned
	GetVolumeInfo(ctx context.Context, volumePath string) (*migration.CnsvSphereVolumeMigrationSpec, error)

	// SaveVolumeInfo helps create CR for given cnsvSphereVolumeMigration
	// this func also update local cache with supplied cnsvSphereVolumeMigration, after successful creation of CR
	SaveVolumeInfo(ctx context.Context, cnsvSphereVolumeMigration *migration.CnsvSphereVolumeMigration) error

	// DeleteVolumeInfo helps delete cached mapping of volumePath to VolumeInfo and
	// also helps delete K8s Custom Resource for specified volumePath
	DeleteVolumeInfo(ctx context.Context, volumePath string) error

	// LoadAllVolumeInfo helps cache all CR for CnsvSphereVolumeMigration
	LoadAllVolumeInfo(ctx context.Context) error

	// Register Volume takes in-tree VolumePath and helps register Volume with CNS
	RegisterVolume(ctx context.Context, volumePath string, manager *common.Manager) (*migration.CnsvSphereVolumeMigration, error)
}

// VolumeMigration holds migrated volume information and provides functionality around it.
type volumeMigration struct {
	// Volume Path to volumeInfo map
	volumePathToVolumeInfo sync.Map
	// migrationClient helps operate on CnsvSphereVolumeMigration custom resource
	migrationClient client.Client
}

// onceForInTreeVolumeMigrationService is used for initializing the VolumeMigrationService singleton.
var onceForVolumeMigrationService sync.Once

// volumeMigrationInstance is instance of volumeMigration and implements interface for VolumeMigrationService
var volumeMigrationInstance *volumeMigration

// GetVolumeMigrationService returns the singleton VolumeMigrationService
func GetVolumeMigrationService(ctx context.Context) VolumeMigrationService {
	log := logger.GetLogger(ctx)
	onceForVolumeMigrationService.Do(func() {
		log.Info("Initializing volume migration service...")
		volumeMigrationInstance = &volumeMigration{
			volumePathToVolumeInfo: sync.Map{},
		}
		var migrationClientErr error
		for {
			config, err := k8s.GetKubeConfig(ctx)
			if err != nil {
				log.Errorf("failed to get kubeconfig. err: %v", err)
			}
			volumeMigrationInstance.migrationClient, migrationClientErr = k8s.NewClientForGroup(ctx, config, "cns.vmware.com")
			if migrationClientErr == nil {
				break
			} else {
				log.Errorf("failed to create migrationClient. Err: %v", err)
			}
		}
		for {
			createVolumeMigrationCRDError := createrCnsvSphereVolumeMigrationCRD(ctx)
			if createVolumeMigrationCRDError == nil {
				break
			} else {
				log.Errorf("failed to create volume migration CRD. Error: %+v", createVolumeMigrationCRDError)
			}
		}
		log.Info("volume migration service initialized")
	})
	return volumeMigrationInstance
}

// createrCnsvSphereVolumeMigrationCRD creates the CRD for CnsvSphereVolumeMigration
func createrCnsvSphereVolumeMigrationCRD(ctx context.Context) error {
	log := logger.GetLogger(ctx)
	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		log.Errorf("failed to get Kubernetes config. Err: %+v", err)
		return err
	}
	apiextensionsClientSet, err := apiextensionsclient.NewForConfig(cfg)
	if err != nil {
		log.Errorf("failed to create Kubernetes client using config. Err: %+v", err)
		return err
	}
	crdName := reflect.TypeOf(migration.CnsvSphereVolumeMigration{}).Name()
	crd := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cnsvspherevolumemigrations.cns.vmware.com",
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:   migration.SchemeGroupVersion.Group,
			Version: migration.SchemeGroupVersion.Version,
			Scope:   apiextensionsv1beta1.NamespaceScoped,
			Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
				Plural: "cnsvspherevolumemigrations",
				Kind:   crdName,
			},
		},
	}
	_, err = apiextensionsClientSet.ApiextensionsV1beta1().CustomResourceDefinitions().Create(crd)
	if err == nil {
		log.Infof("%q CRD created successfully", crdName)
	} else if apierrors.IsAlreadyExists(err) {
		log.Debugf("%q CRD already exists", crdName)
		return nil
	} else {
		log.Errorf("failed to create %q CRD with err: %+v", crdName, err)
		return err
	}

	// CRD takes some time to be established
	// Creating an instance of non-established runs into errors. So, wait for CRD to be created
	err = wait.Poll(pollTime, timeout, func() (bool, error) {
		crd, err = apiextensionsClientSet.ApiextensionsV1beta1().CustomResourceDefinitions().Get(crdName, metav1.GetOptions{})
		if err != nil {
			log.Errorf("failed to get %q CRD with err: %+v", crdName, err)
			return false, err
		}
		for _, cond := range crd.Status.Conditions {
			switch cond.Type {
			case apiextensionsv1beta1.Established:
				if cond.Status == apiextensionsv1beta1.ConditionTrue {
					return true, err
				}
			case apiextensionsv1beta1.NamesAccepted:
				if cond.Status == apiextensionsv1beta1.ConditionFalse {
					log.Debugf("Name conflict while waiting for %q CRD creation", cond.Reason)
				}
			}
		}
		return false, err
	})

	// If there is an error, delete the object to keep it clean.
	if err != nil {
		log.Infof("Cleanup %q CRD because the CRD created was not successfully established. Error: %+v", crdName, err)
		deleteErr := apiextensionsClientSet.ApiextensionsV1beta1().CustomResourceDefinitions().Delete(crdName, nil)
		if deleteErr != nil {
			log.Errorf("failed to delete %q CRD with error: %+v", crdName, deleteErr)
		}
	}
	return err
}

// GetVolumeInfo returns CnsvSphereVolumeMigrationSpec for given VolumePath from local cache
// if CnsvSphereVolumeMigrationSpec is not available in the cache, K8s CR look up will be performed, if that too
// can not retrieve VolumeInfo, ErrVolumeNotRegistered will be returned
func (volumeMigration *volumeMigration) GetVolumeInfo(ctx context.Context, volumePath string) (*migration.CnsvSphereVolumeMigrationSpec, error) {
	log := logger.GetLogger(ctx)
	info, found := volumeMigration.volumePathToVolumeInfo.Load(volumePath)
	if found {
		log.Infof("CnsvSphereVolumeMigrationSpec found from the cache for for VolumePath: %q", volumePath)
		return info.(*migration.CnsvSphereVolumeMigrationSpec), nil
	}
	volumeMigrationCRList := &migration.CnsvSphereVolumeMigrationList{}
	err := volumeMigration.migrationClient.List(ctx, volumeMigrationCRList)
	if err != nil {
		log.Errorf("failed to get volumeMigration CR list")
		return nil, err
	}
	for _, object := range volumeMigrationCRList.Items {
		if object.Spec.VolumePath == volumePath {
			log.Infof("found CR for VolumePath: %q", volumePath)
			volumeMigration.volumePathToVolumeInfo.Store(volumePath, object.Spec)
			log.Debugf("cached VolumeMigrationSpec for VolumePath: %q", volumePath)
			return &object.Spec, nil
		}
	}
	log.Infof("Could not retrieve CnsvSphereVolumeMigrationSpec from cache and K8s CR for Volume Path: %q. volume may not be registered", volumePath)
	return nil, ErrVolumeNotRegistered
}

// SaveVolumeInfo helps create CR for given cnsvSphereVolumeMigration
// this func also update local cache with supplied cnsvSphereVolumeMigration, after successful creation of CR
func (volumeMigration *volumeMigration) SaveVolumeInfo(ctx context.Context, cnsvSphereVolumeMigration *migration.CnsvSphereVolumeMigration) error {
	log := logger.GetLogger(ctx)
	log.Infof("creating CR for cnsvSphereVolumeMigration: %+v", cnsvSphereVolumeMigration)
	err := volumeMigration.migrationClient.Create(ctx, cnsvSphereVolumeMigration)
	if err != nil {
		log.Errorf("failed to create CR for cnsvSphereVolumeMigration. Error: %v", err)
		return err
	}
	log.Infof("successfully created CR for cnsvSphereVolumeMigration: %v", cnsvSphereVolumeMigration)
	volumeMigration.volumePathToVolumeInfo.Store(cnsvSphereVolumeMigration.Spec.VolumePath, cnsvSphereVolumeMigration)
	log.Infof("successfully updated cache for cnsvSphereVolumeMigrationSpec: %v", cnsvSphereVolumeMigration)
	return nil
}

// DeleteVolumeInfo helps delete cached mapping of volumePath to VolumeInfo and
// also helps delete K8s Custom Resource for specified volumePath
func (volumeMigration *volumeMigration) DeleteVolumeInfo(ctx context.Context, volumePath string) error {
	log := logger.GetLogger(ctx)
	volumeMigrationCRList := &migration.CnsvSphereVolumeMigrationList{}
	err := volumeMigration.migrationClient.List(ctx, volumeMigrationCRList)
	if err != nil {
		log.Errorf("failed to get volumeMigration CR list")
		return err
	}
	for _, object := range volumeMigrationCRList.Items {
		if object.Spec.VolumePath == volumePath {
			volumeMigration.migrationClient.Delete(ctx, &object)
			log.Infof("successfully deleted CR for volumeMigration having volumePath: %q", volumePath)
			break
		}
	}
	volumeMigration.volumePathToVolumeInfo.Delete(volumePath)
	log.Infof("successfully deleted volumeInfo from cache for volumePath: %q", volumePath)
	return nil
}

// LoadAllVolumeInfo helps cache all CR for CnsvSphereVolumeMigration
func (volumeMigration *volumeMigration) LoadAllVolumeInfo(ctx context.Context) error {
	log := logger.GetLogger(ctx)
	volumeMigrationCRList := &migration.CnsvSphereVolumeMigrationList{}
	err := volumeMigration.migrationClient.List(ctx, volumeMigrationCRList)
	if err != nil {
		log.Errorf("failed to get volumeMigration CR list")
		return err
	}
	for _, object := range volumeMigrationCRList.Items {
		volumeMigration.volumePathToVolumeInfo.Store(object.Spec.VolumePath, object.Spec)
	}
	log.Infof("successfully loaded all CR for CnsvSphereVolumeMigration in the cache")
	return nil
}

// Register Volume takes in-tree VolumePath and helps register Volume with CNS
func (volumeMigration *volumeMigration) RegisterVolume(ctx context.Context, volumePath string, manager *common.Manager) (*migration.CnsvSphereVolumeMigration, error) {
	log := logger.GetLogger(ctx)
	uuid, err := uuid.NewUUID()
	if err != nil {
		log.Errorf("failed to generate uuid")
		return nil, err
	}
	cnsvSphereVolumeMigration := migration.CnsvSphereVolumeMigration{
		ObjectMeta: metav1.ObjectMeta{Name: uuid.String()},
	}
	vc, err := common.GetVCenter(ctx, manager)
	if err != nil {
		log.Errorf("failed to get vCenter from Manager, err: %+v", err)
		return nil, err
	}

	re := regexp.MustCompile(`\[([^\[\]]*)\]`)
	if !re.MatchString(volumePath) {
		msg := fmt.Sprintf("failed to extract datastore name from in-tree volume path: %q", volumePath)
		log.Errorf(msg)
		return nil, errors.New(msg)
	}
	datastoreName := re.FindAllString(volumePath, -1)[0]
	vmdkPath := strings.TrimSpace(strings.Trim(volumePath, datastoreName))
	datastoreName = strings.Trim(strings.Trim(datastoreName, "["), "]")

	// Format:
	// https://<vc_ip>/folder/<vm_vmdk_path>?dcPath=<datacenterName>&dsName=<datastoreName>
	backingDiskURLPath := "https://" + vc.Config.Host + "/folder/" +
		vmdkPath + "?dcPath=" + vc.Config.DatacenterPaths[0] + "&dsName=" + datastoreName

	log.Infof("Registering volume: %q using backingDiskURLPath :%q", volumePath, backingDiskURLPath)
	var containerClusterArray []cnstypes.CnsContainerCluster
	containerCluster := vsphere.GetContainerCluster(manager.CnsConfig.Global.ClusterID, manager.CnsConfig.VirtualCenter[vc.Config.Host].User, cnstypes.CnsClusterFlavorVanilla)
	containerClusterArray = append(containerClusterArray, containerCluster)
	createSpec := &cnstypes.CnsVolumeCreateSpec{
		Name:       uuid.String(),
		VolumeType: common.BlockVolumeType,
		Metadata: cnstypes.CnsVolumeMetadata{
			ContainerCluster:      containerCluster,
			ContainerClusterArray: containerClusterArray,
		},
		BackingObjectDetails: &cnstypes.CnsBlockBackingDetails{
			BackingDiskUrlPath: backingDiskURLPath,
		},
	}
	log.Debugf("vSphere CNS driver registering volume %q with create spec %+v", volumePath, spew.Sdump(createSpec))
	volumeID, err := manager.VolumeManager.CreateVolume(ctx, createSpec)
	if err != nil {
		log.Errorf("failed to register volume %q with error %+v", volumePath, err)
		return nil, err
	}
	log.Infof("successfully registered volume %q as container volume with ID: %q", volumePath, volumeID.Id)
	cnsvSphereVolumeMigration.Spec = migration.CnsvSphereVolumeMigrationSpec{
		VolumePath: volumePath,
		VolumeID:   volumeID.Id,
		VolumeName: uuid.String(),
	}
	return &cnsvSphereVolumeMigration, nil
}
