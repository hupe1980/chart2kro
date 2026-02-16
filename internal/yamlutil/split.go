// Package yamlutil provides shared YAML utilities used across chart2kro.
package yamlutil

import (
	"regexp"
	"strings"
)

// docSeparator matches YAML document separators: a line containing only "---"
// optionally followed by whitespace.
var docSeparator = regexp.MustCompile(`(?m)^---\s*$`)

// SplitDocuments splits a multi-document YAML byte slice into individual
// documents, filtering out empty ones. Each returned slice is a raw YAML
// document without the leading "---" separator.
func SplitDocuments(data []byte) [][]byte {
	parts := docSeparator.Split(string(data), -1)

	var docs [][]byte

	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			docs = append(docs, []byte(part))
		}
	}

	return docs
}

// SplitDocumentsString splits multi-document YAML into individual document
// strings, filtering out empty ones.
func SplitDocumentsString(data []byte) []string {
	parts := docSeparator.Split(string(data), -1)

	docs := make([]string, 0, len(parts))

	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			docs = append(docs, part)
		}
	}

	return docs
}
