package output

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// WriterFactory creates a Writer for the given output path.
// When path is empty, the writer should write to stdout.
type WriterFactory func(path string) Writer

// Registry maps format names to WriterFactory functions, enabling
// pluggable output formats for the export command.
type Registry struct {
	mu      sync.RWMutex
	writers map[string]WriterFactory
}

// NewRegistry creates an empty writer registry.
func NewRegistry() *Registry {
	return &Registry{
		writers: make(map[string]WriterFactory),
	}
}

// Register adds a writer factory under the given format name.
// Existing entries for the same name are overwritten.
func (r *Registry) Register(name string, factory WriterFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.writers[name] = factory
}

// Writer returns the factory for the given format, or an error if not found.
func (r *Registry) Writer(name string) (WriterFactory, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	f, ok := r.writers[name]
	if !ok {
		return nil, fmt.Errorf("unknown output format %q (available: %s)", name, r.AvailableFormats())
	}

	return f, nil
}

// Formats returns the sorted list of registered format names.
func (r *Registry) Formats() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.writers))
	for name := range r.writers {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

// AvailableFormats returns a comma-separated string of registered format names.
func (r *Registry) AvailableFormats() string {
	formats := r.Formats()
	if len(formats) == 0 {
		return "none"
	}

	return strings.Join(formats, ", ")
}

// DefaultRegistry returns a registry pre-populated with the built-in
// output formats: yaml, json, stdout, file.
func DefaultRegistry() *Registry {
	r := NewRegistry()

	r.Register("yaml", func(path string) Writer {
		if path == "" {
			return NewStdoutWriter(nil)
		}

		return NewFileWriter(path)
	})

	r.Register("json", func(path string) Writer {
		if path == "" {
			return NewStdoutWriter(nil)
		}

		return NewFileWriter(path)
	})

	r.Register("stdout", func(_ string) Writer {
		return NewStdoutWriter(nil)
	})

	r.Register("file", func(path string) Writer {
		return NewFileWriter(path)
	})

	return r
}
