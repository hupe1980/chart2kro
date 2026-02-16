package yamlutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitDocuments(t *testing.T) {
	tests := []struct {
		name string
		data string
		want int
	}{
		{"empty", "", 0},
		{"single doc", "apiVersion: v1\nkind: Service\n", 1},
		{"two docs", "apiVersion: v1\nkind: Service\n---\napiVersion: apps/v1\nkind: Deployment\n", 2},
		{"leading separator", "---\napiVersion: v1\nkind: Service\n", 1},
		{"trailing separator", "apiVersion: v1\nkind: Service\n---\n", 1},
		{"separator with trailing spaces", "apiVersion: v1\n---   \napiVersion: apps/v1\n", 2},
		{"empty doc between separators", "apiVersion: v1\n---\n\n---\napiVersion: apps/v1\n", 2},
		{"whitespace-only doc", "apiVersion: v1\n---\n   \n---\napiVersion: apps/v1\n", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docs := SplitDocuments([]byte(tt.data))
			assert.Len(t, docs, tt.want)
		})
	}
}

func TestSplitDocumentsString(t *testing.T) {
	data := []byte("apiVersion: v1\nkind: Service\n---\napiVersion: apps/v1\nkind: Deployment\n")
	docs := SplitDocumentsString(data)
	assert.Len(t, docs, 2)
	assert.Contains(t, docs[0], "Service")
	assert.Contains(t, docs[1], "Deployment")
}

func TestSplitDocumentsString_Empty(t *testing.T) {
	docs := SplitDocumentsString([]byte(""))
	assert.Empty(t, docs)
}
