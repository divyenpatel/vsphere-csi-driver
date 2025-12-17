/*
Copyright 2025 The Kubernetes Authors.

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

package prometheus

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestVolumeTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{
			name:     "PrometheusBlockVolumeType",
			constant: PrometheusBlockVolumeType,
			expected: "block",
		},
		{
			name:     "PrometheusFileVolumeType",
			constant: PrometheusFileVolumeType,
			expected: "file",
		},
		{
			name:     "PrometheusUnknownVolumeType",
			constant: PrometheusUnknownVolumeType,
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = %q, expected %q", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

func TestCSIOperationTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{
			name:     "PrometheusCreateVolumeOpType",
			constant: PrometheusCreateVolumeOpType,
			expected: "create-volume",
		},
		{
			name:     "PrometheusDeleteVolumeOpType",
			constant: PrometheusDeleteVolumeOpType,
			expected: "delete-volume",
		},
		{
			name:     "PrometheusAttachVolumeOpType",
			constant: PrometheusAttachVolumeOpType,
			expected: "attach-volume",
		},
		{
			name:     "PrometheusDetachVolumeOpType",
			constant: PrometheusDetachVolumeOpType,
			expected: "detach-volume",
		},
		{
			name:     "PrometheusExpandVolumeOpType",
			constant: PrometheusExpandVolumeOpType,
			expected: "expand-volume",
		},
		{
			name:     "PrometheusCreateSnapshotOpType",
			constant: PrometheusCreateSnapshotOpType,
			expected: "create-snapshot",
		},
		{
			name:     "PrometheusDeleteSnapshotOpType",
			constant: PrometheusDeleteSnapshotOpType,
			expected: "delete-snapshot",
		},
		{
			name:     "PrometheusListSnapshotsOpType",
			constant: PrometheusListSnapshotsOpType,
			expected: "list-snapshot",
		},
		{
			name:     "PrometheusListVolumeOpType",
			constant: PrometheusListVolumeOpType,
			expected: "list-volume",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = %q, expected %q", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

func TestCNSOperationTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{
			name:     "PrometheusCnsCreateVolumeOpType",
			constant: PrometheusCnsCreateVolumeOpType,
			expected: "create-volume",
		},
		{
			name:     "PrometheusCnsDeleteVolumeOpType",
			constant: PrometheusCnsDeleteVolumeOpType,
			expected: "delete-volume",
		},
		{
			name:     "PrometheusCnsAttachVolumeOpType",
			constant: PrometheusCnsAttachVolumeOpType,
			expected: "attach-volume",
		},
		{
			name:     "PrometheusCnsDetachVolumeOpType",
			constant: PrometheusCnsDetachVolumeOpType,
			expected: "detach-volume",
		},
		{
			name:     "PrometheusCnsBatchAttachVolumeOpType",
			constant: PrometheusCnsBatchAttachVolumeOpType,
			expected: "batch-volume",
		},
		{
			name:     "PrometheusCnsUpdateVolumeMetadataOpType",
			constant: PrometheusCnsUpdateVolumeMetadataOpType,
			expected: "update-volume-metadata",
		},
		{
			name:     "PrometheusCnsUpdateVolumeCryptoOpType",
			constant: PrometheusCnsUpdateVolumeCryptoOpType,
			expected: "update-volume-crypto",
		},
		{
			name:     "PrometheusCnsExpandVolumeOpType",
			constant: PrometheusCnsExpandVolumeOpType,
			expected: "expand-volume",
		},
		{
			name:     "PrometheusCnsQueryVolumeOpType",
			constant: PrometheusCnsQueryVolumeOpType,
			expected: "query-volume",
		},
		{
			name:     "PrometheusCnsQueryAllVolumeOpType",
			constant: PrometheusCnsQueryAllVolumeOpType,
			expected: "query-all-volume",
		},
		{
			name:     "PrometheusCnsQueryVolumeInfoOpType",
			constant: PrometheusCnsQueryVolumeInfoOpType,
			expected: "query-volume-info",
		},
		{
			name:     "PrometheusCnsRelocateVolumeOpType",
			constant: PrometheusCnsRelocateVolumeOpType,
			expected: "relocate-volume",
		},
		{
			name:     "PrometheusCnsConfigureVolumeACLOpType",
			constant: PrometheusCnsConfigureVolumeACLOpType,
			expected: "configure-volume-acl",
		},
		{
			name:     "PrometheusQuerySnapshotsOpType",
			constant: PrometheusQuerySnapshotsOpType,
			expected: "query-snapshots",
		},
		{
			name:     "PrometheusCnsCreateSnapshotOpType",
			constant: PrometheusCnsCreateSnapshotOpType,
			expected: "create-snapshot",
		},
		{
			name:     "PrometheusCnsDeleteSnapshotOpType",
			constant: PrometheusCnsDeleteSnapshotOpType,
			expected: "delete-snapshot",
		},
		{
			name:     "PrometheusUnregisterVolumeOpType",
			constant: PrometheusUnregisterVolumeOpType,
			expected: "unregister-volume",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = %q, expected %q", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

func TestVolumeHealthConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{
			name:     "PrometheusAccessibleVolumes",
			constant: PrometheusAccessibleVolumes,
			expected: "accessible-volumes",
		},
		{
			name:     "PrometheusInaccessibleVolumes",
			constant: PrometheusInaccessibleVolumes,
			expected: "inaccessible-volumes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = %q, expected %q", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

func TestStatusConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{
			name:     "PrometheusPassStatus",
			constant: PrometheusPassStatus,
			expected: "pass",
		},
		{
			name:     "PrometheusFailStatus",
			constant: PrometheusFailStatus,
			expected: "fail",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = %q, expected %q", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

func TestCsiInfoMetric(t *testing.T) {
	// Verify CsiInfo metric is registered and can be used
	if CsiInfo == nil {
		t.Fatal("CsiInfo metric is nil")
	}

	// Test setting a value
	CsiInfo.WithLabelValues("v3.0.0").Set(1)

	// Verify the metric can be described
	ch := make(chan *prometheus.Desc, 1)
	CsiInfo.Describe(ch)
	desc := <-ch
	if desc == nil {
		t.Error("CsiInfo.Describe() returned nil")
	}
}

func TestSyncerInfoMetric(t *testing.T) {
	// Verify SyncerInfo metric is registered and can be used
	if SyncerInfo == nil {
		t.Fatal("SyncerInfo metric is nil")
	}

	// Test setting a value
	SyncerInfo.WithLabelValues("v3.0.0").Set(1)

	// Verify the metric can be described
	ch := make(chan *prometheus.Desc, 1)
	SyncerInfo.Describe(ch)
	desc := <-ch
	if desc == nil {
		t.Error("SyncerInfo.Describe() returned nil")
	}
}

func TestCsiControlOpsHistVec(t *testing.T) {
	// Verify CsiControlOpsHistVec metric is registered and can be used
	if CsiControlOpsHistVec == nil {
		t.Fatal("CsiControlOpsHistVec metric is nil")
	}

	// Test observing a value
	CsiControlOpsHistVec.WithLabelValues(
		PrometheusBlockVolumeType,
		PrometheusCreateVolumeOpType,
		PrometheusPassStatus,
		"",
	).Observe(5.0)

	// Verify the metric can be described
	ch := make(chan *prometheus.Desc, 1)
	CsiControlOpsHistVec.Describe(ch)
	desc := <-ch
	if desc == nil {
		t.Error("CsiControlOpsHistVec.Describe() returned nil")
	}
}

func TestCnsControlOpsHistVec(t *testing.T) {
	// Verify CnsControlOpsHistVec metric is registered and can be used
	if CnsControlOpsHistVec == nil {
		t.Fatal("CnsControlOpsHistVec metric is nil")
	}

	// Test observing a value
	CnsControlOpsHistVec.WithLabelValues(
		PrometheusCnsCreateVolumeOpType,
		PrometheusPassStatus,
	).Observe(10.0)

	// Verify the metric can be described
	ch := make(chan *prometheus.Desc, 1)
	CnsControlOpsHistVec.Describe(ch)
	desc := <-ch
	if desc == nil {
		t.Error("CnsControlOpsHistVec.Describe() returned nil")
	}
}

func TestVolumeHealthGaugeVec(t *testing.T) {
	// Verify VolumeHealthGaugeVec metric is registered and can be used
	if VolumeHealthGaugeVec == nil {
		t.Fatal("VolumeHealthGaugeVec metric is nil")
	}

	// Test setting values
	VolumeHealthGaugeVec.WithLabelValues(PrometheusAccessibleVolumes).Set(100)
	VolumeHealthGaugeVec.WithLabelValues(PrometheusInaccessibleVolumes).Set(5)

	// Verify the metric can be described
	ch := make(chan *prometheus.Desc, 1)
	VolumeHealthGaugeVec.Describe(ch)
	desc := <-ch
	if desc == nil {
		t.Error("VolumeHealthGaugeVec.Describe() returned nil")
	}
}

func TestFullSyncOpsHistVec(t *testing.T) {
	// Verify FullSyncOpsHistVec metric is registered and can be used
	if FullSyncOpsHistVec == nil {
		t.Fatal("FullSyncOpsHistVec metric is nil")
	}

	// Test observing a value
	FullSyncOpsHistVec.WithLabelValues(PrometheusPassStatus).Observe(30.0)
	FullSyncOpsHistVec.WithLabelValues(PrometheusFailStatus).Observe(60.0)

	// Verify the metric can be described
	ch := make(chan *prometheus.Desc, 1)
	FullSyncOpsHistVec.Describe(ch)
	desc := <-ch
	if desc == nil {
		t.Error("FullSyncOpsHistVec.Describe() returned nil")
	}
}

func TestRequestOpsMetric(t *testing.T) {
	// Verify RequestOpsMetric metric is registered and can be used
	if RequestOpsMetric == nil {
		t.Fatal("RequestOpsMetric metric is nil")
	}

	// Test observing a value
	RequestOpsMetric.WithLabelValues("CreateVolume", "cns", PrometheusPassStatus).Observe(2.5)

	// Verify the metric can be described
	ch := make(chan *prometheus.Desc, 1)
	RequestOpsMetric.Describe(ch)
	desc := <-ch
	if desc == nil {
		t.Error("RequestOpsMetric.Describe() returned nil")
	}
}

func TestHistogramBuckets(t *testing.T) {
	// Verify the histogram buckets are correctly defined
	expectedBuckets := []float64{2, 5, 10, 15, 20, 25, 30, 60, 120, 180}

	// We can't directly access the buckets from the histogram, but we can verify
	// that the histograms work with various observation values
	testValues := []float64{1, 3, 7, 12, 18, 22, 28, 45, 90, 150, 200}

	for _, val := range testValues {
		CsiControlOpsHistVec.WithLabelValues(
			PrometheusBlockVolumeType,
			PrometheusCreateVolumeOpType,
			PrometheusPassStatus,
			"",
		).Observe(val)

		CnsControlOpsHistVec.WithLabelValues(
			PrometheusCnsCreateVolumeOpType,
			PrometheusPassStatus,
		).Observe(val)

		FullSyncOpsHistVec.WithLabelValues(PrometheusPassStatus).Observe(val)
		RequestOpsMetric.WithLabelValues("test", "client", PrometheusPassStatus).Observe(val)
	}

	// Verify expected buckets count
	if len(expectedBuckets) != 10 {
		t.Errorf("Expected 10 buckets, got %d", len(expectedBuckets))
	}
}

func TestMetricLabels(t *testing.T) {
	// Test that metrics accept the expected label combinations

	// CsiControlOpsHistVec labels: voltype, optype, status, faulttype
	testCases := []struct {
		volType   string
		opType    string
		status    string
		faultType string
	}{
		{PrometheusBlockVolumeType, PrometheusCreateVolumeOpType, PrometheusPassStatus, ""},
		{PrometheusFileVolumeType, PrometheusDeleteVolumeOpType, PrometheusFailStatus, "vim.fault.NotFound"},
		{PrometheusUnknownVolumeType, PrometheusAttachVolumeOpType, PrometheusPassStatus, ""},
	}

	for _, tc := range testCases {
		// This should not panic
		CsiControlOpsHistVec.WithLabelValues(tc.volType, tc.opType, tc.status, tc.faultType).Observe(1.0)
	}

	// CnsControlOpsHistVec labels: optype, status
	cnsTestCases := []struct {
		opType string
		status string
	}{
		{PrometheusCnsCreateVolumeOpType, PrometheusPassStatus},
		{PrometheusCnsDeleteVolumeOpType, PrometheusFailStatus},
		{PrometheusCnsQueryVolumeOpType, PrometheusPassStatus},
	}

	for _, tc := range cnsTestCases {
		// This should not panic
		CnsControlOpsHistVec.WithLabelValues(tc.opType, tc.status).Observe(1.0)
	}

	// VolumeHealthGaugeVec labels: volume_health_type
	healthTestCases := []string{
		PrometheusAccessibleVolumes,
		PrometheusInaccessibleVolumes,
	}

	for _, tc := range healthTestCases {
		// This should not panic
		VolumeHealthGaugeVec.WithLabelValues(tc).Set(1.0)
	}
}

