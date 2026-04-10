package s3

import (
	"errors"
	"strings"
	"testing"
)

func TestDetectBackend(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		want     S3Backend
	}{
		// Port-based detection
		{
			name:     "MinIO default port",
			endpoint: "localhost:9000",
			want:     BackendMinIO,
		},
		{
			name:     "ObjectScale HTTP port",
			endpoint: "objectstore.example.com:9020",
			want:     BackendObjectScale,
		},
		{
			name:     "ObjectScale HTTPS port",
			endpoint: "objectstore.example.com:9021",
			want:     BackendObjectScale,
		},
		// Hostname-based detection
		{
			name:     "AWS S3 global endpoint",
			endpoint: "s3.amazonaws.com",
			want:     BackendAWS,
		},
		{
			name:     "AWS S3 regional endpoint",
			endpoint: "s3.us-west-2.amazonaws.com",
			want:     BackendAWS,
		},
		{
			name:     "MinIO hostname",
			endpoint: "minio.local:8080",
			want:     BackendMinIO,
		},
		{
			name:     "ObjectScale hostname",
			endpoint: "objectscale.corp.local:443",
			want:     BackendObjectScale,
		},
		{
			name:     "Dell hostname",
			endpoint: "dell-ecs.corp.local:443",
			want:     BackendObjectScale,
		},
		{
			name:     "s3 dot prefix in hostname",
			endpoint: "s3.mycompany.com",
			want:     BackendAWS,
		},
		// Scheme stripping
		{
			name:     "HTTP scheme stripped for port detection",
			endpoint: "http://localhost:9000",
			want:     BackendMinIO,
		},
		{
			name:     "HTTPS scheme stripped for port detection",
			endpoint: "https://objectstore.example.com:9021",
			want:     BackendObjectScale,
		},
		// Default fallback
		{
			name:     "unknown endpoint defaults to AWS",
			endpoint: "storage.example.com:443",
			want:     BackendAWS,
		},
		{
			name:     "random endpoint defaults to AWS",
			endpoint: "10.0.0.1:8080",
			want:     BackendAWS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectBackend(tt.endpoint)
			if got != tt.want {
				t.Errorf("DetectBackend(%q) = %q, want %q", tt.endpoint, got, tt.want)
			}
		})
	}
}

func TestWrapConnectionError(t *testing.T) {
	baseErr := errors.New("connection refused")

	tests := []struct {
		name            string
		backend         S3Backend
		wantContains    []string
		wantNotContains []string
	}{
		{
			name:    "ObjectScale backend includes hints",
			backend: BackendObjectScale,
			wantContains: []string{
				"objectscale backend",
				"connection refused",
				"port 9020",
				"path-style addressing",
				"ObjectScale namespace",
				"TLS certificate",
				"ObjectScale troubleshooting hints",
			},
		},
		{
			name:    "MinIO backend includes hints",
			backend: BackendMinIO,
			wantContains: []string{
				"minio backend",
				"connection refused",
				"MinIO is running",
				"access key and secret key",
				"MinIO troubleshooting hints",
			},
		},
		{
			name:    "AWS backend has no extra hints",
			backend: BackendAWS,
			wantContains: []string{
				"aws backend",
				"connection refused",
			},
			wantNotContains: []string{
				"troubleshooting hints",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapConnectionError(baseErr, tt.backend)
			if got == nil {
				t.Fatal("wrapConnectionError returned nil")
			}
			msg := got.Error()
			for _, substr := range tt.wantContains {
				if !strings.Contains(msg, substr) {
					t.Errorf("error message should contain %q, got:\n%s", substr, msg)
				}
			}
			for _, substr := range tt.wantNotContains {
				if strings.Contains(msg, substr) {
					t.Errorf("error message should NOT contain %q, got:\n%s", substr, msg)
				}
			}
		})
	}
}

func TestWrapConnectionError_DifferentErrors(t *testing.T) {
	errs := []error{
		errors.New("timeout"),
		errors.New("no such host"),
		errors.New("TLS handshake failure"),
	}

	for _, e := range errs {
		got := wrapConnectionError(e, BackendMinIO)
		if !strings.Contains(got.Error(), e.Error()) {
			t.Errorf("wrapped error should contain original error message %q", e.Error())
		}
	}
}
