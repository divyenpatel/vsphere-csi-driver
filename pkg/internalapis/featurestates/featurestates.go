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

package featurestates

import (
	"context"
	"reflect"
	"strconv"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/vsphere-csi-driver/pkg/csi/service/logger"
	"sigs.k8s.io/vsphere-csi-driver/pkg/internalapis"
	featurestatesv1alpha1 "sigs.k8s.io/vsphere-csi-driver/pkg/internalapis/featurestates/v1alpha1"
	k8s "sigs.k8s.io/vsphere-csi-driver/pkg/kubernetes"
)

const (
	// CRDName represent the name of cnscsisvfeaturestate CRD
	CRDName = "cnscsisvfeaturestate.cns.vmware.com"
	// CRDGroupName represent the group of cnscsisvfeaturestate CRD
	CRDGroupName = "cns.vmware.com"
	// CRDSingular represent the singular name of cnscsisvfeaturestate CRD
	CRDSingular = "cnscsisvfeaturestate"
	// CRDPlural represent the plural name of cnscsisvfeaturestates CRD
	CRDPlural = "cnscsisvfeaturestates"
	// WorkLoadNamespaceLabelKey is the label key found on the workload namespace in the supervisor k8s cluster
	WorkLoadNamespaceLabelKey = "vSphereClusterID"
	// SVFeatureStateCRName to be used for CR in workload namespaces
	SVFeatureStateCRName = "svfeaturestates"
	// crUpdateRetryInterval is the interval at which all failed pending CR update/create tasks are executed
	crUpdateRetryInterval = 2 * time.Minute
)

// pendingCRUpdates holds latest update pending to persist on the CR
type pendingCRUpdates struct {
	// lock to acquire before updating pendingCRUpdateTask and pendingCRCreateTask
	lock            *sync.RWMutex
	pendingCRUpdate map[string]*featurestatesv1alpha1.CnsCsiSvFeatureStates
}

var (
	supervisorFeatureStatConfigMapName       string
	supervisorFeatureStateConfigMapNamespace string
	pendingCRUpdatesObj                      *pendingCRUpdates
	k8sClient                                clientset.Interface
	controllerRuntimeClient                  client.Client
)

// StartSvFSSReplicationService Starts SvFSSReplicationService
func StartSvFSSReplicationService(ctx context.Context, svFeatureStatConfigMapName string, svFeatureStateConfigMapNamespace string) error {
	log := logger.GetLogger(ctx)
	log.Info("starting SvFSSReplication service")

	supervisorFeatureStatConfigMapName = svFeatureStatConfigMapName
	supervisorFeatureStateConfigMapNamespace = svFeatureStateConfigMapNamespace
	pendingCRUpdatesObj = &pendingCRUpdates{
		lock: &sync.RWMutex{},
	}
	// This is idempotent if CRD is pre-created then we continue with initialization of svFSSReplicationService
	err := k8s.CreateCustomResourceDefinitionFromSpec(ctx, CRDName, CRDSingular, CRDPlural,
		reflect.TypeOf(featurestatesv1alpha1.CnsCsiSvFeatureStates{}).Name(), CRDGroupName, internalapis.SchemeGroupVersion.Version, apiextensionsv1beta1.NamespaceScoped)
	if err != nil {
		log.Errorf("failed to create CnsCsiSvFeatureStates CRD. Error: %v", err)
		return err
	}

	// Create the kubernetes client
	k8sClient, err = k8s.NewClient(ctx)
	if err != nil {
		log.Errorf("create k8s client failed. Err: %v", err)
		return err
	}
	// get kube config
	config, err := k8s.GetKubeConfig(ctx)
	if err != nil {
		log.Errorf("failed to get kubeconfig. Error: %v", err)
		return err
	}
	// create controller runtime client
	controllerRuntimeClient, err = k8s.NewClientForGroup(ctx, config, CRDGroupName)
	if err != nil {
		log.Errorf("failed to create controllerRuntimeClient. Err: %v", err)
		return err
	}

	// create/update feature state CRs in all workload namespaces
	err = updateFeatureStatesCRInAllWorkLoadNamespaces(ctx, []featurestatesv1alpha1.FeatureState{})
	if err != nil {
		log.Errorf("failed to update feature states CR in the all workload namespaces. err: %v", err)
		return err
	}

	// Create k8s Informer and watch on configmaps and namespaces
	informer := k8s.NewInformer(k8sClient)
	informer.AddConfigMapListener(ctx, k8sClient, svFeatureStateConfigMapNamespace,
		// Add
		nil,
		// Update
		func(oldObj interface{}, newObj interface{}) {
			configMapUpdated(oldObj, newObj)
		},
		// Delete
		nil)
	informer.AddNamespaceListener(
		// Add
		func(obj interface{}) {
			namespaceAdded(obj)
		},
		// Update
		nil,
		// Delete
		nil)
	informer.Listen()
	log.Infof("informer on config-map and namespaces started")

	// Start Retry routine to process failed create or update CRs
	go pendingCRUpdatesObj.startRetryAttempts()
	log.Infof("started background routine to process failed CR updates at regulat interval")
	log.Infof("SvFSSReplicationService is running")
	var stopCh = make(chan bool)
	<-stopCh
	return nil
}

// startRetryAttempts helps process pending failed CR updates
func (pendingCRUpdatesObj *pendingCRUpdates) startRetryAttempts() {
	for {
		func() {
			pendingCRUpdatesObj.lock.Lock()
			defer pendingCRUpdatesObj.lock.Unlock()
			ctx, log := logger.GetNewContextWithLogger()
			log.Infof("performing retry operations on failed feature states CRs")

			for namespace, featurestatesSpec := range pendingCRUpdatesObj.pendingCRUpdate {
				if featurestatesSpec != nil {
					// Check if namespace for the CR is still present on the cluster
					_, err := k8sClient.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
					if err != nil {
						if apierrors.IsNotFound(err) {
							// if Namespace is not present, mark pending task as done
							pendingCRUpdatesObj.pendingCRUpdate[namespace] = nil
							continue
						} else {
							log.Errorf("failed to check if namespace: %q is present on the cluster", namespace)
							continue
						}
					}
					// check if CR is present on the namespace
					featurestateCR := &featurestatesv1alpha1.CnsCsiSvFeatureStates{}
					err = controllerRuntimeClient.Get(ctx, client.ObjectKey{Name: SVFeatureStateCRName, Namespace: namespace}, featurestateCR)
					if err != nil {
						if !apierrors.IsNotFound(err) {
							// attempt to Create the CR
							err = controllerRuntimeClient.Create(ctx, featurestatesSpec)
						}
					} else {
						// Attempt to Update CR
						err = controllerRuntimeClient.Update(ctx, featurestatesSpec)
					}
					// if no error observed, mark pending task as done
					if err == nil {
						pendingCRUpdatesObj.pendingCRUpdate[namespace] = nil
					}
				}
			}
		}()
		time.Sleep(crUpdateRetryInterval)
	}
}

// updateFeatureStatesCRInAllWorkLoadNamespaces helps update latest featureStates to all workload namespaces
func updateFeatureStatesCRInAllWorkLoadNamespaces(ctx context.Context, featurestates []featurestatesv1alpha1.FeatureState) error {
	pendingCRUpdatesObj.lock.Lock()
	defer pendingCRUpdatesObj.lock.Unlock()

	log := logger.GetLogger(ctx)
	log.Info("updating feature states in all workload namespaces")

	// retrieving all workload namespaces
	namespaces, err := k8sClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: WorkLoadNamespaceLabelKey,
	})
	if err != nil {
		log.Errorf("failed to get workload namespaces. Err: %v", err)
		return err
	}

	if len(featurestates) == 0 {
		featurestates, err = getFeatureStates(ctx, k8sClient)
		if err != nil {
			log.Errorf("failed to get latest featurestates. Err: %v", err)
			return err
		}
	}
	// creating and updating CRs in all workload namespaces to push latest feature switch state from SV Config map
	for _, namespace := range namespaces.Items {
		cnsCsiSvFeatureStates := &featurestatesv1alpha1.CnsCsiSvFeatureStates{
			ObjectMeta: metav1.ObjectMeta{Name: SVFeatureStateCRName, Namespace: namespace.Name},
			Spec: featurestatesv1alpha1.CnsCsiSvFeatureStatesSpec{
				FeatureStates: featurestates,
			},
		}
		// check if CR is present on the namespace
		featurestateCR := &featurestatesv1alpha1.CnsCsiSvFeatureStates{}
		err := controllerRuntimeClient.Get(ctx, client.ObjectKey{Name: SVFeatureStateCRName, Namespace: namespace.Name}, featurestateCR)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				// CR is not present on the namespace, attempt to Create the CR
				err = controllerRuntimeClient.Create(ctx, cnsCsiSvFeatureStates)
			}
		} else {
			// Attempt to Update CR, as CR is present and err is nil
			err = controllerRuntimeClient.Update(ctx, cnsCsiSvFeatureStates)
		}
		if err != nil {
			// update any prior CR update for this namespace, to cache latest feature state
			pendingCRUpdatesObj.pendingCRUpdate[namespace.Name] = cnsCsiSvFeatureStates
		} else {
			// remove any prior pending CR update for this namespace, as latest state is applied
			pendingCRUpdatesObj.pendingCRUpdate[namespace.Name] = nil
		}
	}
	return nil
}

// createFeatureStatesCRInWorkLoadNamespace helps create latest featureStates CR in the specified namespace with latest feature states
func createFeatureStatesCRInWorkLoadNamespace(ctx context.Context, namespace string) error {
	pendingCRUpdatesObj.lock.Lock()
	defer pendingCRUpdatesObj.lock.Unlock()

	log := logger.GetLogger(ctx)
	log.Info("creating feature states in the namespaces: %v", namespace)

	featureStates, err := getFeatureStates(ctx, k8sClient)
	if err != nil {
		log.Errorf("failed to get latest featurestates. Err: %v", err)
		return err
	}
	cnsCsiSvFeatureStates := &featurestatesv1alpha1.CnsCsiSvFeatureStates{
		ObjectMeta: metav1.ObjectMeta{Name: SVFeatureStateCRName, Namespace: namespace},
		Spec: featurestatesv1alpha1.CnsCsiSvFeatureStatesSpec{
			FeatureStates: featureStates,
		},
	}
	err = controllerRuntimeClient.Create(ctx, cnsCsiSvFeatureStates)
	if err != nil {
		pendingCRUpdatesObj.pendingCRUpdate[namespace] = cnsCsiSvFeatureStates
	}
	return nil
}

// configMapUpdated is the call back function for config-map informer
func configMapUpdated(oldObj, newObj interface{}) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = logger.NewContextWithLogger(ctx)
	log := logger.GetLogger(ctx)

	newfssConfigMap, ok := oldObj.(*v1.ConfigMap)
	if newfssConfigMap == nil || !ok {
		log.Warnf("configMapUpdated: unrecognized old object %+v", newObj)
		return
	}
	oldfssConfigMap, ok := newObj.(*v1.ConfigMap)
	if oldfssConfigMap == nil || !ok {
		log.Warnf("configMapUpdated: unrecognized new object %+v", newObj)
		return
	}

	if newfssConfigMap.Name == supervisorFeatureStatConfigMapName &&
		newfssConfigMap.Namespace == supervisorFeatureStateConfigMapNamespace {
		if reflect.DeepEqual(newfssConfigMap.Data, oldfssConfigMap.Data) {
			return
		}
		var err error
		var featureStates []featurestatesv1alpha1.FeatureState
		for feature, state := range newfssConfigMap.Data {
			var featureState featurestatesv1alpha1.FeatureState
			featureState.Name = feature
			featureState.Enabled, err = strconv.ParseBool(state)
			if err != nil {
				log.Errorf("failed to parse feature state value: %v for feature: %q", state, feature)
				return
			}
			featureStates = append(featureStates, featureState)
		}
		for {
			// if failed to create or update CR in the namespace, then error is not returned
			// for such cases, CR update will be enqueued to pendingCRUpdatesObj
			err := updateFeatureStatesCRInAllWorkLoadNamespaces(ctx, featureStates)
			if err != nil {
				log.Errorf("failed to update feature states CR in the all workload namespaces. err: %v", err)
			}
		}
	}
}

// configMapAdded adds feature state switch values from configmap that has been created on K8s cluster
func namespaceAdded(obj interface{}) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = logger.NewContextWithLogger(ctx)
	log := logger.GetLogger(ctx)

	namespace, ok := obj.(*v1.Namespace)
	if namespace == nil || !ok {
		log.Warnf("namespaceAdded: unrecognized object %+v", obj)
		return
	}
	if _, ok = namespace.Labels[WorkLoadNamespaceLabelKey]; ok {
		log.Infof("observed new workload namespace: %v", namespace.Name)
		for {
			// if failed to create CR in the namespace, then error is not returned
			// for such case, CR update will be enqueued to pendingCRUpdatesObj
			err := createFeatureStatesCRInWorkLoadNamespace(ctx, namespace.Name)
			if err == nil {
				break
			}
			log.Errorf("failed to create feature states CR in the workload namespace: %q, err: %v", namespace, err)
		}
	}
}

// getFeatureStates returns latest feature states from supervisor
func getFeatureStates(ctx context.Context, k8sClient clientset.Interface) ([]featurestatesv1alpha1.FeatureState, error) {
	log := logger.GetLogger(ctx)
	//  Retrieve SV FeatureStates configmap
	fssConfigMap, err := k8sClient.CoreV1().ConfigMaps(supervisorFeatureStateConfigMapNamespace).Get(ctx, supervisorFeatureStatConfigMapName, metav1.GetOptions{})
	if err != nil {
		log.Errorf("failed to retrive SV featureswitch state from namespace:%q with name: %q", supervisorFeatureStateConfigMapNamespace, supervisorFeatureStatConfigMapName)
		return nil, err
	}

	var featureStates []featurestatesv1alpha1.FeatureState
	for feature, state := range fssConfigMap.Data {
		var featureState featurestatesv1alpha1.FeatureState
		featureState.Name = feature
		featureState.Enabled, err = strconv.ParseBool(state)
		if err != nil {
			log.Errorf("failed to parse feature state value: %v for feature: %q", state, feature)
			return nil, err
		}
		featureStates = append(featureStates, featureState)
	}
	return featureStates, nil
}
