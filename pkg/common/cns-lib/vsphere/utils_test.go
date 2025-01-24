package vsphere

import (
	"context"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
)

func TestFilterSuspendedDatastoresWhenDatastoreIsSuspended(t *testing.T) {

	customValue := []types.CustomFieldValue{
		{Key: 101},
	}
	CustomFieldStringValue := []types.CustomFieldStringValue{
		{Value: cnsMgrDatastoreSuspended, CustomFieldValue: customValue[0]},
	}
	customValue2 := (types.CustomFieldStringValue)(CustomFieldStringValue[0])
	baseCustomFieldValue := (types.BaseCustomFieldValue)(&customValue2)

	datastoreMoref := types.ManagedObjectReference{Type: "datastore", Value: "datastore-1"}
	datastore := &Datastore{Datastore: object.NewDatastore(nil, datastoreMoref)}
	dsInfo := []*DatastoreInfo{
		{
			Datastore: datastore,
			Info: &types.DatastoreInfo{
				Name: "test-ds",
			},
			CustomValues: []types.BaseCustomFieldValue{baseCustomFieldValue},
		},
	}

	outputDsInfo, err := FilterSuspendedDatastores(context.TODO(), dsInfo)
	assert.NotNil(t, err)
	assert.Equal(t, 0, len(outputDsInfo))
}
func TestFilterSuspendedDatastoresWhenDatastoreIsNotSuspended(t *testing.T) {

	customValue := []types.CustomFieldValue{
		{Key: 101},
	}
	CustomFieldStringValue := []types.CustomFieldStringValue{
		{Value: "randomValue", CustomFieldValue: customValue[0]},
	}
	customValue2 := (types.CustomFieldStringValue)(CustomFieldStringValue[0])
	baseCustomFieldValue := (types.BaseCustomFieldValue)(&customValue2)

	dsInfo := []*DatastoreInfo{
		{
			Info: &types.DatastoreInfo{
				Name: "test-ds",
			},
			CustomValues: []types.BaseCustomFieldValue{baseCustomFieldValue},
		},
	}

	outputDsInfo, err := FilterSuspendedDatastores(context.TODO(), dsInfo)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(outputDsInfo))

}

func TestIsvSphereVersion70U3orAbove(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	aboutInfo := types.AboutInfo{
		Name:                  "VMware vCenter Server",
		FullName:              "VMware vCenter Server 9.0.0 build-24495978",
		Vendor:                "VMware, Inc.",
		Version:               "9.0.0",
		Build:                 "24495978",
		LocaleVersion:         "INTL",
		LocaleBuild:           "000",
		OsType:                "linux-x64",
		ProductLineId:         "vpx",
		ApiType:               "VirtualCenter",
		ApiVersion:            "9.0.0.0.rc1",
		InstanceUuid:          "4394cd59-cd63-43df-bdf5-8a2cee4fe055",
		LicenseProductName:    "VMware VirtualCenter Server",
		LicenseProductVersion: "9.0",
	}
	above70u3, err := IsvSphereVersion70U3orAbove(ctx, aboutInfo)
	if err == nil {
		if !above70u3 {
			t.Fatal("IsvSphereVersion70U3orAbove returned false, expecting true")
		} else {
			t.Log(spew.Sprint("IsvSphereVersion70U3orAbove returned true for aboutInfo", aboutInfo))
		}
	} else {
		t.Fatal("Received error from UseVslmAPIs method")
	}
}
