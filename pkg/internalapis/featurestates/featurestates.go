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
	CRDName = "cnscsisvfeaturestates.cns.vmware.com"
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
	crUpdateRetryInterval = 1 * time.Minute
)

// pendingCRUpdates holds latest update pending to persist on the CR
type pendingCRUpdates struct {
	// lock to acquire before updating pendingCRUpdateTask and pendingCRCreateTask
	lock            *sync.RWMutex
	pendingCRUpdate map[string][]featurestatesv1alpha1.FeatureState
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
	log.Info("Starting SvFSSReplicationService")

	supervisorFeatureStatConfigMapName = svFeatureStatConfigMapName
	supervisorFeatureStateConfigMapNamespace = svFeatureStateConfigMapNamespace
	pendingCRUpdatesObj = &pendingCRUpdates{
		lock:            &sync.RWMutex{},
		pendingCRUpdate: make(map[string][]featurestatesv1alpha1.FeatureState),
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
	pendingCRUpdatesObj.enqueueCRUpdatesForAllWorkloadNamespaces(ctx, getFeatureStates(ctx))
	// Create k8s Informer and watch on configmaps and namespaces
	informer := k8s.NewInformer(k8sClient)
	informer.AddConfigMapListener(ctx, k8sClient, svFeatureStateConfigMapNamespace,
		// Add
		func(Obj interface{}) {
			configMapAdded(Obj)
		},
		// Update
		func(oldObj interface{}, newObj interface{}) {
			configMapUpdated(oldObj, newObj)
		},
		// Delete
		func(obj interface{}) {
			configMapDeleted(obj)
		})
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
	go pendingCRUpdatesObj.processPendingCRUpdates()
	log.Infof("started background routine to process failed CR updates at regular interval")
	log.Infof("SvFSSReplicationService is running")
	var stopCh = make(chan bool)
	<-stopCh
	return nil
}

// processPendingCRUpdates helps process pending CR updates at regular interval
func (pendingCRUpdatesObj *pendingCRUpdates) processPendingCRUpdates() {
	for {
		func() {
			pendingCRUpdatesObj.lock.Lock()
			defer pendingCRUpdatesObj.lock.Unlock()
			ctx, log := logger.GetNewContextWithLogger()
			log.Infof("starting to process pending feature state updates")
			for namespace, featurestates := range pendingCRUpdatesObj.pendingCRUpdate {
				if len(featurestates) != 0 {
					// Check if namespace still present on the cluster
					namespacePresent, err := isNamespacePresent(ctx, namespace)
					if err != nil {
						continue
					}
					if namespacePresent {
						// check if CR is present on the namespace
						featurestateCR := &featurestatesv1alpha1.CnsCsiSvFeatureStates{}
						err = controllerRuntimeClient.Get(ctx, client.ObjectKey{Name: SVFeatureStateCRName, Namespace: namespace}, featurestateCR)
						if err != nil {
							if !apierrors.IsNotFound(err) {
								log.Errorf("failed to get cnsCsiSvFeatureStates CR instance from the namespace: %q, Err: %v", namespace, err)
								continue
							}
							// attempt to Create the CR
							cnsCsiSvFeatureStates := &featurestatesv1alpha1.CnsCsiSvFeatureStates{
								ObjectMeta: metav1.ObjectMeta{Name: SVFeatureStateCRName, Namespace: namespace},
								Spec: featurestatesv1alpha1.CnsCsiSvFeatureStatesSpec{
									FeatureStates: featurestates,
								},
							}
							err = controllerRuntimeClient.Create(ctx, cnsCsiSvFeatureStates)
							if err != nil {
								log.Errorf("failed to create cnsCsiSvFeatureStates CR instance in the namespace: %q, Err: %v", namespace, err)
								continue
							}
							log.Infof("created cnsCsiSvFeatureStates CR instance in the namespace: %q", namespace)
						} else {
							// Attempt to Update CR
							featurestateCR.Spec.FeatureStates = featurestates
							err = controllerRuntimeClient.Update(ctx, featurestateCR)
							if err != nil {
								log.Errorf("failed to update cnsCsiSvFeatureStates CR instance in the namespace: %q, Err: %v", namespace, err)
								continue
							}
							log.Infof("updated cnsCsiSvFeatureStates CR instance in the namespace: %q", namespace)
						}
					} else {
						log.Infof("namespace: %q no longer present on the cluster. clearing pending featurestate updates", namespace)
					}
					pendingCRUpdatesObj.pendingCRUpdate[namespace] = nil
				}
			}
		}()
		time.Sleep(crUpdateRetryInterval)
	}
}

// enqueueCRUpdatesForAllWorkloadNamespaces helps enqueue CR updates for all workload namespaces
func (pendingCRUpdatesObj *pendingCRUpdates) enqueueCRUpdatesForAllWorkloadNamespaces(ctx context.Context, featurestates []featurestatesv1alpha1.FeatureState) {
	pendingCRUpdatesObj.lock.Lock()
	defer pendingCRUpdatesObj.lock.Unlock()
	log := logger.GetLogger(ctx)
	namespaces := getAllWorkloadNamespaces(ctx)
	for _, namespace := range namespaces.Items {
		pendingCRUpdatesObj.pendingCRUpdate[namespace.Name] = featurestates
		log.Infof("enqueued CR updates for workload namespace: %q", namespace)
	}
}

// enqueueCRUpdatesForWorkloadNamespace enqueues CR updates for specified workload namespaces
func (pendingCRUpdatesObj *pendingCRUpdates) enqueueCRUpdatesForNewWorkloadNamespace(ctx context.Context, namespace string) {
	pendingCRUpdatesObj.lock.Lock()
	defer pendingCRUpdatesObj.lock.Unlock()
	log := logger.GetLogger(ctx)
	pendingCRUpdatesObj.pendingCRUpdate[namespace] = getFeatureStates(ctx)
	log.Infof("enqueued CR updates for workload namespace: %q", namespace)
}

// configMapAdded is called when configmap is created
func configMapAdded(obj interface{}) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = logger.NewContextWithLogger(ctx)
	log := logger.GetLogger(ctx)

	fssConfigMap, ok := obj.(*v1.ConfigMap)
	if fssConfigMap == nil || !ok {
		log.Warnf("configMapAdded: unrecognized object %+v", obj)
		return
	}
	if fssConfigMap.Name == supervisorFeatureStatConfigMapName &&
		fssConfigMap.Namespace == supervisorFeatureStateConfigMapNamespace {
		var err error
		var featureStates []featurestatesv1alpha1.FeatureState
		for feature, state := range fssConfigMap.Data {
			var featureState featurestatesv1alpha1.FeatureState
			featureState.Name = feature
			featureState.Enabled, err = strconv.ParseBool(state)
			if err != nil {
				log.Errorf("failed to parse feature state value: %v for feature: %q", state, feature)
			}
			featureStates = append(featureStates, featureState)
		}
		pendingCRUpdatesObj.enqueueCRUpdatesForAllWorkloadNamespaces(ctx, featureStates)
	}
}

// configMapUpdated is called when configmap is updated
func configMapUpdated(oldObj, newObj interface{}) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = logger.NewContextWithLogger(ctx)
	log := logger.GetLogger(ctx)

	newfssConfigMap, ok := newObj.(*v1.ConfigMap)
	if newfssConfigMap == nil || !ok {
		log.Warnf("configMapUpdated: unrecognized old object %+v", newObj)
		return
	}
	oldfssConfigMap, ok := oldObj.(*v1.ConfigMap)
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
			}
			featureStates = append(featureStates, featureState)
		}
		pendingCRUpdatesObj.enqueueCRUpdatesForAllWorkloadNamespaces(ctx, featureStates)
	}
}

// configMapDeleted is called when config-map is deleted
func configMapDeleted(obj interface{}) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = logger.NewContextWithLogger(ctx)
	log := logger.GetLogger(ctx)

	fssConfigMap, ok := obj.(*v1.ConfigMap)
	if fssConfigMap == nil || !ok {
		log.Warnf("configMapAdded: unrecognized object %+v", obj)
		return
	}
	if fssConfigMap.Name == supervisorFeatureStatConfigMapName &&
		fssConfigMap.Namespace == supervisorFeatureStateConfigMapNamespace {
		log.Warnf("supervisor feature switch state configmap %q from the namespace: %q is deleted", supervisorFeatureStatConfigMapName, supervisorFeatureStateConfigMapNamespace)
	}
}

// namespaceAdded adds is called when new namespace is added on the k8s cluster.
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
		pendingCRUpdatesObj.enqueueCRUpdatesForNewWorkloadNamespace(ctx, namespace.Name)
	}
}

// getFeatureStates returns latest feature states from supervisor config-map
func getFeatureStates(ctx context.Context) []featurestatesv1alpha1.FeatureState {
	log := logger.GetLogger(ctx)
	//  Retrieve SV FeatureStates configmap
	var fssConfigMap *v1.ConfigMap
	var err error
	for {
		fssConfigMap, err = k8sClient.CoreV1().ConfigMaps(supervisorFeatureStateConfigMapNamespace).Get(ctx, supervisorFeatureStatConfigMapName, metav1.GetOptions{})
		if err == nil {
			break
		}
		log.Errorf("failed to retrieve SV feature switch state from namespace:%q with name: %q", supervisorFeatureStateConfigMapNamespace, supervisorFeatureStatConfigMapName)
		time.Sleep(time.Second)
	}
	log.Infof("sucessfully retrieved SV feature switch state from namespace:%q with name: %q", supervisorFeatureStateConfigMapNamespace, supervisorFeatureStatConfigMapName)
	var featureStates []featurestatesv1alpha1.FeatureState
	for feature, state := range fssConfigMap.Data {
		var featureState featurestatesv1alpha1.FeatureState
		featureState.Name = feature
		featureState.Enabled, err = strconv.ParseBool(state)
		if err != nil {
			log.Errorf("failed to parse feature state value: %v for feature: %q", state, feature)
		}
		featureStates = append(featureStates, featureState)
	}
	return featureStates
}

// getAllWorkloadNamespaces returns all workload namespaces from the cluster
func getAllWorkloadNamespaces(ctx context.Context) *v1.NamespaceList {
	log := logger.GetLogger(ctx)
	// retrieving all workload namespaces
	var namespaces *v1.NamespaceList
	var err error
	for {
		namespaces, err = k8sClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{LabelSelector: WorkLoadNamespaceLabelKey})
		if err == nil {
			break
		}
		log.Errorf("failed to list workload namespaces. Err: %v", err)
		time.Sleep(time.Second)
	}
	log.Info("successfully retrieved all workload namespaces")
	return namespaces
}

// isNamespacePresent returns true if specified namespace is present on the cluster, else returns false
// if error observed while checking namespace, err is returned
func isNamespacePresent(ctx context.Context, namespace string) (bool, error) {
	log := logger.GetLogger(ctx)
	// Check if namespace for the CR is still present on the cluster
	_, err := k8sClient.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		log.Errorf("failed to check if namespace: %q is present on the cluster", namespace)
		return false, err
	}
	return true, nil
}
