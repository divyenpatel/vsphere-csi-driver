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

package wcpguest

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	testclient "k8s.io/client-go/kubernetes/fake"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"sigs.k8s.io/vsphere-csi-driver/pkg/common/config"
	"sigs.k8s.io/vsphere-csi-driver/pkg/csi/service/common"
	k8s "sigs.k8s.io/vsphere-csi-driver/pkg/kubernetes"
)

const (
	testVolumeName        = "pvc-12345"
	testSupervisorPVCName = "pvcsc-12345"
	testNamespace         = "test-namespace"
	testStorageClass      = "test-storageclass"
)

var (
	isUnitTest          bool
	supervisorNamespace string
)

func configFromSim() (clientset.Interface, error) {
	isUnitTest = true
	supervisorClient := testclient.NewSimpleClientset()
	supervisorNamespace = testNamespace
	return supervisorClient, nil
}

func configFromEnvOrSim() (clientset.Interface, error) {
	cfg := &config.Config{}
	if err := config.FromEnvToGC(cfg); err != nil {
		return configFromSim()
	}
	isUnitTest = false
	// This step is help to format the certificate from env.
	certificate := strings.Replace(cfg.GC.Certificate, `\n`, "\n", -1)
	supervisorClient, err := k8s.NewSupervisorClient(cfg.GC.Endpoint, cfg.GC.Port, certificate, cfg.GC.Token)
	if err != nil {
		return nil, err
	}
	supervisorNamespace = cfg.GC.Namespace
	return supervisorClient, nil
}

type controllerTest struct {
	controller *controller
}

var (
	controllerTestInstance *controllerTest
	onceForControllerTest  sync.Once
)

func getControllerTest(t *testing.T) *controllerTest {
	onceForControllerTest.Do(func() {
		supervisorClient, err := configFromEnvOrSim()
		if err != nil {
			t.Fatal(err)
		}

		c := &controller{
			supervisorClient:    supervisorClient,
			supervisorNamespace: supervisorNamespace,
		}

		controllerTestInstance = &controllerTest{
			controller: c,
		}
	})
	return controllerTestInstance
}

func createVolume(ct *controllerTest, ctx context.Context, reqCreate *csi.CreateVolumeRequest, response chan *csi.CreateVolumeResponse, error chan error) {
	defer close(response)
	defer close(error)
	res, err := ct.controller.CreateVolume(ctx, reqCreate)
	response <- res
	error <- err
}

/*
 * TestGuestCreateVolume creates volume
 */
func TestGuestClusterControllerFlow(t *testing.T) {
	// Create context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ct := getControllerTest(t)

	// Create
	params := make(map[string]string, 0)

	params[common.AttributeSupervisorStorageClass] = testStorageClass
	if v := os.Getenv("SUPERVISOR_STORAGE_CLASS"); v != "" {
		params[common.AttributeSupervisorStorageClass] = v
	}
	capabilities := []*csi.VolumeCapability{
		{
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
	}

	reqCreate := &csi.CreateVolumeRequest{
		Name: testVolumeName,
		CapacityRange: &csi.CapacityRange{
			RequiredBytes: 1 * common.GbInBytes,
		},
		Parameters:         params,
		VolumeCapabilities: capabilities,
	}

	var respCreate *csi.CreateVolumeResponse
	var err error

	if isUnitTest {
		// Invoking CreateVolume in a separate thread and then setting the Status to Bound explicitly
		response := make(chan *csi.CreateVolumeResponse)
		error := make(chan error)

		go createVolume(ct, ctx, reqCreate, response, error)
		time.Sleep(1 * time.Second)
		pvc, _ := ct.controller.supervisorClient.CoreV1().PersistentVolumeClaims(ct.controller.supervisorNamespace).Get(testSupervisorPVCName, metav1.GetOptions{})
		pvc.Status.Phase = "Bound"
		ct.controller.supervisorClient.CoreV1().PersistentVolumeClaims(ct.controller.supervisorNamespace).Update(pvc)
		respCreate, err = <-response, <-error
	} else {
		respCreate, err = ct.controller.CreateVolume(ctx, reqCreate)
		// wait for create volume finish
		time.Sleep(1 * time.Second)
	}

	if err != nil {
		t.Fatal(err)
	}

	supervisorPVCName := respCreate.Volume.VolumeId
	// verify the pvc has been created
	_, err = ct.controller.supervisorClient.CoreV1().PersistentVolumeClaims(ct.controller.supervisorNamespace).Get(supervisorPVCName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Delete
	reqDelete := &csi.DeleteVolumeRequest{
		VolumeId: supervisorPVCName,
	}
	_, err = ct.controller.DeleteVolume(ctx, reqDelete)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for delete volume finish
	time.Sleep(1 * time.Second)
	// Verify the pvc has been deleted
	_, err = ct.controller.supervisorClient.CoreV1().PersistentVolumeClaims(ct.controller.supervisorNamespace).Get(supervisorPVCName, metav1.GetOptions{})
	if !errors.IsNotFound(err) {
		t.Fatal(err)
	}

}
