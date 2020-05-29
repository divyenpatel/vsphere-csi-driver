package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CnsvSphereVolumeMigration is the Schema for the cnsvspherevolumemigrations API
type CnsvSphereVolumeMigration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec CnsvSphereVolumeMigrationSpec `json:"spec,omitempty"`
}

// CnsvSphereVolumeMigrationSpec defines the desired state of CnsvSphereVolumeMigration
type CnsvSphereVolumeMigrationSpec struct {
	// VolumePath is the vmdk path of the vSphere Volume
	VolumePath string `json:"volumepath"`
	// VolumeName is the name of the volume
	VolumeName string `json:"volumename"`
	// VolumeID is the FCD ID obtained after register volume with CNS.
	VolumeID string `json:"volumeid"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CnsvSphereVolumeMigrationList contains a list of CnsvSphereVolumeMigration
type CnsvSphereVolumeMigrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CnsvSphereVolumeMigration `json:"items"`
}
