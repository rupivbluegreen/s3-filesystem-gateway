package metrics

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRecordNFSOp_Success(t *testing.T) {
	// Reset by reading the current value first, then checking increment.
	before := testutil.ToFloat64(nfsOpsTotal.WithLabelValues("read", "success"))
	RecordNFSOp("read", 100*time.Millisecond, nil)
	after := testutil.ToFloat64(nfsOpsTotal.WithLabelValues("read", "success"))
	if after-before != 1 {
		t.Errorf("nfsOpsTotal(read,success) increment = %v, want 1", after-before)
	}
}

func TestRecordNFSOp_Error(t *testing.T) {
	before := testutil.ToFloat64(nfsOpsTotal.WithLabelValues("write", "error"))
	RecordNFSOp("write", 50*time.Millisecond, errors.New("disk full"))
	after := testutil.ToFloat64(nfsOpsTotal.WithLabelValues("write", "error"))
	if after-before != 1 {
		t.Errorf("nfsOpsTotal(write,error) increment = %v, want 1", after-before)
	}
}

func TestRecordNFSOp_Duration(t *testing.T) {
	// Record an operation and verify the histogram has observations.
	RecordNFSOp("lookup_dur", 200*time.Millisecond, nil)

	// Collect from the histogram vec and check that the observation was recorded.
	count := testutil.CollectAndCount(nfsOpDuration)
	if count == 0 {
		t.Error("nfsOpDuration should have at least one metric after RecordNFSOp")
	}
}

func TestRecordS3Request_Success(t *testing.T) {
	before := testutil.ToFloat64(s3RequestsTotal.WithLabelValues("GetObject", "success"))
	RecordS3Request("GetObject", 30*time.Millisecond, nil)
	after := testutil.ToFloat64(s3RequestsTotal.WithLabelValues("GetObject", "success"))
	if after-before != 1 {
		t.Errorf("s3RequestsTotal(GetObject,success) increment = %v, want 1", after-before)
	}
}

func TestRecordS3Request_Error(t *testing.T) {
	before := testutil.ToFloat64(s3RequestsTotal.WithLabelValues("PutObject", "error"))
	RecordS3Request("PutObject", 30*time.Millisecond, errors.New("access denied"))
	after := testutil.ToFloat64(s3RequestsTotal.WithLabelValues("PutObject", "error"))
	if after-before != 1 {
		t.Errorf("s3RequestsTotal(PutObject,error) increment = %v, want 1", after-before)
	}
}

func TestRecordS3Request_Duration(t *testing.T) {
	RecordS3Request("ListObjects_dur", 150*time.Millisecond, nil)

	count := testutil.CollectAndCount(s3RequestDuration)
	if count == 0 {
		t.Error("s3RequestDuration should have at least one metric after RecordS3Request")
	}
}

func TestRecordCacheHit(t *testing.T) {
	cacheTypes := []string{"metadata", "data", "listing"}
	for _, ct := range cacheTypes {
		t.Run(ct, func(t *testing.T) {
			before := testutil.ToFloat64(cacheHitsTotal.WithLabelValues(ct))
			RecordCacheHit(ct)
			after := testutil.ToFloat64(cacheHitsTotal.WithLabelValues(ct))
			if after-before != 1 {
				t.Errorf("cacheHitsTotal(%s) increment = %v, want 1", ct, after-before)
			}
		})
	}
}

func TestRecordCacheMiss(t *testing.T) {
	cacheTypes := []string{"metadata", "data", "listing"}
	for _, ct := range cacheTypes {
		t.Run(ct, func(t *testing.T) {
			before := testutil.ToFloat64(cacheMissesTotal.WithLabelValues(ct))
			RecordCacheMiss(ct)
			after := testutil.ToFloat64(cacheMissesTotal.WithLabelValues(ct))
			if after-before != 1 {
				t.Errorf("cacheMissesTotal(%s) increment = %v, want 1", ct, after-before)
			}
		})
	}
}

func TestRecordBytesTransferred(t *testing.T) {
	tests := []struct {
		direction string
		bytes     int64
	}{
		{"read", 1024},
		{"write", 2048},
	}

	for _, tt := range tests {
		t.Run(tt.direction, func(t *testing.T) {
			before := testutil.ToFloat64(bytesTransferredTotal.WithLabelValues(tt.direction))
			RecordBytesTransferred(tt.direction, tt.bytes)
			after := testutil.ToFloat64(bytesTransferredTotal.WithLabelValues(tt.direction))
			if got := after - before; got != float64(tt.bytes) {
				t.Errorf("bytesTransferredTotal(%s) increment = %v, want %v", tt.direction, got, tt.bytes)
			}
		})
	}
}

func TestIncrDecrConnections(t *testing.T) {
	before := testutil.ToFloat64(activeConnections)
	IncrConnections()
	after := testutil.ToFloat64(activeConnections)
	if after-before != 1 {
		t.Errorf("IncrConnections: gauge increment = %v, want 1", after-before)
	}

	DecrConnections()
	afterDecr := testutil.ToFloat64(activeConnections)
	if afterDecr-before != 0 {
		t.Errorf("DecrConnections: gauge should return to previous value, got delta = %v", afterDecr-before)
	}
}

func TestMetricsRegistered(t *testing.T) {
	// Verify all metrics are registered with the default registry by collecting.
	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	expectedNames := []string{
		"s3gw_nfs_operations_total",
		"s3gw_nfs_operation_duration_seconds",
		"s3gw_s3_requests_total",
		"s3gw_s3_request_duration_seconds",
		"s3gw_cache_hits_total",
		"s3gw_cache_misses_total",
		"s3gw_active_connections",
		"s3gw_bytes_transferred_total",
	}

	// Ensure at least the expected metrics exist (they may have been touched by previous tests).
	// First, trigger all metrics so they appear in the gather output.
	RecordNFSOp("_test", time.Millisecond, nil)
	RecordS3Request("_test", time.Millisecond, nil)
	RecordCacheHit("_test")
	RecordCacheMiss("_test")
	RecordBytesTransferred("_test", 1)
	IncrConnections()
	DecrConnections()

	metricFamilies, err = prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	found := make(map[string]bool)
	for _, mf := range metricFamilies {
		found[mf.GetName()] = true
	}

	for _, name := range expectedNames {
		if !found[name] {
			t.Errorf("metric %q not found in gathered metrics", name)
		}
	}
}

func TestMetricsOutput(t *testing.T) {
	// Verify that gathering produces valid prometheus text output.
	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather() error: %v", err)
	}
	if len(metricFamilies) == 0 {
		t.Error("expected at least some metric families")
	}

	// Quick check: at least one family should have a HELP string containing "s3gw".
	foundS3GW := false
	for _, mf := range metricFamilies {
		if strings.Contains(mf.GetName(), "s3gw") {
			foundS3GW = true
			break
		}
	}
	if !foundS3GW {
		t.Error("expected at least one metric family with 's3gw' prefix")
	}
}
