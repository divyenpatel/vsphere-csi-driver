/*
Copyright 2026 The Kubernetes Authors.

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

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

// loadCnsRegisterVolumeCRD parses the CRD the syncer actually embeds and applies at
// startup, so these assertions are about the shipped artifact rather than a copy.
func loadCnsRegisterVolumeCRD(t *testing.T) *apiextensionsv1.CustomResourceDefinition {
	t.Helper()
	data, err := EmbedCnsRegisterVolumeCRFile.ReadFile(EmbedCnsRegisterVolumeCRFileName)
	require.NoError(t, err)

	crd := &apiextensionsv1.CustomResourceDefinition{}
	require.NoError(t, yaml.Unmarshal(data, crd))
	return crd
}

func cnsRegisterVolumeStatusSchema(t *testing.T) *apiextensionsv1.JSONSchemaProps {
	t.Helper()
	crd := loadCnsRegisterVolumeCRD(t)
	require.Len(t, crd.Spec.Versions, 1)
	status, ok := crd.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["status"]
	require.True(t, ok, "the CRD must declare a status schema")
	return &status
}

// TestCRDDeclaresPhaseAndLastUpdateTime is the guard for the central hazard of this
// change. The schema is structural, so a status field the API server does not know
// about is silently pruned on write: the controller would appear to publish a phase,
// every read would come back empty, and nothing would report an error. If this test
// fails, the Go type and the shipped CRD have drifted apart.
func TestCRDDeclaresPhaseAndLastUpdateTime(t *testing.T) {
	status := cnsRegisterVolumeStatusSchema(t)

	phase, ok := status.Properties["phase"]
	require.True(t, ok, "status.phase must be declared in the CRD or the API server prunes it")
	assert.Equal(t, "string", phase.Type)

	cnsTaskID, ok := status.Properties["cnsTaskID"]
	require.True(t, ok, "status.cnsTaskID must be declared or the API server prunes it, which would "+
		"make every reconcile resubmit a CNS task instead of resuming the one already running")
	assert.Equal(t, "string", cnsTaskID.Type)

	lastUpdateTime, ok := status.Properties["lastUpdateTime"]
	require.True(t, ok, "status.lastUpdateTime must be declared in the CRD or the API server prunes it")
	assert.Equal(t, "string", lastUpdateTime.Type)
	assert.Equal(t, "date-time", lastUpdateTime.Format,
		"lastUpdateTime must be a date-time so kubectl renders it as an age")
}

// TestCRDStatusRemainsBackwardsCompatible verifies the change is purely additive:
// the fields existing consumers read are untouched, and registered stays required so
// the merge-patch behaviour in patchCnsRegisterVolumeStatus is unchanged.
func TestCRDStatusRemainsBackwardsCompatible(t *testing.T) {
	status := cnsRegisterVolumeStatusSchema(t)

	registered, ok := status.Properties["registered"]
	require.True(t, ok)
	assert.Equal(t, "boolean", registered.Type)

	statusErr, ok := status.Properties["error"]
	require.True(t, ok)
	assert.Equal(t, "string", statusErr.Type)

	assert.Equal(t, []string{"registered"}, status.Required,
		"registered must stay required; patchCnsRegisterVolumeStatus always sends it for that reason")

	// The new fields must not be required, so CRs written by an older controller
	// stay valid.
	assert.NotContains(t, status.Required, "phase")
	assert.NotContains(t, status.Required, "lastUpdateTime")
	assert.NotContains(t, status.Required, "cnsTaskID")
}

// TestCRDSpecIsUnchanged verifies no spec field was disturbed; the spec is the half
// of the API that clients write, so a change there would be a breaking change.
func TestCRDSpecIsUnchanged(t *testing.T) {
	crd := loadCnsRegisterVolumeCRD(t)
	spec := crd.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"]

	for _, field := range []string{
		"pvcName", "volumeID", "accessMode", "diskURLPath",
		"volumeMode", "backingType", "storageClassName",
	} {
		assert.Contains(t, spec.Properties, field, "spec.%s must still be declared", field)
	}
	assert.Equal(t, []string{"pvcName"}, spec.Required)
}

// TestCRDPrinterColumns verifies the phase and publish time are visible from
// kubectl, which is the main way an operator would use them.
func TestCRDPrinterColumns(t *testing.T) {
	crd := loadCnsRegisterVolumeCRD(t)
	columns := crd.Spec.Versions[0].AdditionalPrinterColumns

	byName := map[string]apiextensionsv1.CustomResourceColumnDefinition{}
	for _, c := range columns {
		byName[c.Name] = c
	}

	phase, ok := byName["Phase"]
	require.True(t, ok, "kubectl get cnsregistervolume should show the phase")
	assert.Equal(t, ".status.phase", phase.JSONPath)

	lastUpdate, ok := byName["LastUpdate"]
	require.True(t, ok, "kubectl get cnsregistervolume should show the publish time")
	assert.Equal(t, ".status.lastUpdateTime", lastUpdate.JSONPath)
	assert.Equal(t, "date", lastUpdate.Type)

	registered, ok := byName["Registered"]
	require.True(t, ok, "registered is the completion signal and must stay visible")
	assert.Equal(t, ".status.registered", registered.JSONPath)
}
