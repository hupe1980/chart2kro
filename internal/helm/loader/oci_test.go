package loader

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOCILoader_Load_InvalidPrefix(t *testing.T) {
	loader := NewOCILoader()
	_, err := loader.Load(context.Background(), "docker://invalid", LoadOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must start with oci://")
}

func TestExtractHost(t *testing.T) {
	tests := []struct {
		ref  string
		want string
	}{
		{"oci://ghcr.io/org/chart:1.0.0", "ghcr.io"},
		{"oci://registry.example.com/charts/nginx", "registry.example.com"},
		{"oci://localhost:5000/test", "localhost:5000"},
		{"oci://singlehost", "singlehost"},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			assert.Equal(t, tt.want, extractHost(tt.ref))
		})
	}
}

// Note: Full OCI load tests require a running OCI registry, which is beyond
// unit testing scope. Integration tests with a mock registry or testcontainers
// should be added in a dedicated integration test suite.
func TestOCILoader_New(t *testing.T) {
	loader := NewOCILoader()
	assert.NotNil(t, loader)
	assert.NotNil(t, loader.archive)
}

func TestOCILoader_Load_InvalidCAFile(t *testing.T) {
	loader := NewOCILoader()
	_, err := loader.Load(context.Background(), "oci://ghcr.io/org/chart:1.0.0", LoadOptions{
		CaFile: "/nonexistent/ca.pem",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "configuring OCI HTTP client")
}

func TestOCILoader_Load_InvalidCertKeyPair(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	require.NoError(t, os.WriteFile(certFile, []byte("invalid"), 0o600))
	require.NoError(t, os.WriteFile(keyFile, []byte("invalid"), 0o600))

	loader := NewOCILoader()
	_, err := loader.Load(context.Background(), "oci://ghcr.io/org/chart:1.0.0", LoadOptions{
		CertFile: certFile,
		KeyFile:  keyFile,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "configuring OCI HTTP client")
}
