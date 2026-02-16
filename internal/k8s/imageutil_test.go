package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsImageDigest(t *testing.T) {
	tests := []struct {
		image string
		want  bool
	}{
		{"nginx:latest", false},
		{"nginx:1.25", false},
		{"nginx", false},
		{"nginx@sha256:abc123", true},
		{"registry.io:5000/app@sha256:abc123", true},
		{"registry.io:5000/app:latest", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			assert.Equal(t, tt.want, IsImageDigest(tt.image))
		})
	}
}

func TestHasLatestTag(t *testing.T) {
	tests := []struct {
		image string
		want  bool
	}{
		{"nginx", true},
		{"nginx:latest", true},
		{"nginx:1.25", false},
		{"nginx@sha256:abc123", false},
		{"registry.io:5000/app:latest", true},
		{"registry.io:5000/app:1.0", false},
		{"registry.io:5000/app", true},
		{"org/app", true},
		{"org/app:v1.2.3", false},
		{"", false},
		{"registry.io:5000/app@sha256:abc123", false},
	}
	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			assert.Equal(t, tt.want, HasLatestTag(tt.image))
		})
	}
}
