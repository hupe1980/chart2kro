package output

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// Writer is the interface for RGD output destinations.
type Writer interface {
	// Write sends serialized bytes to the output destination.
	Write(data []byte) error
}

// StdoutWriter writes serialized YAML/JSON to os.Stdout.
type StdoutWriter struct {
	out io.Writer
}

// NewStdoutWriter creates a writer that sends output to the given writer.
// If w is nil, os.Stdout is used.
func NewStdoutWriter(w io.Writer) *StdoutWriter {
	if w == nil {
		w = os.Stdout
	}

	return &StdoutWriter{out: w}
}

// Write sends data to stdout.
func (sw *StdoutWriter) Write(data []byte) error {
	_, err := sw.out.Write(data)
	if err != nil {
		return fmt.Errorf("writing to stdout: %w", err)
	}

	return nil
}

// FileWriter writes serialized output to a file, creating parent
// directories as needed.
type FileWriter struct {
	path   string
	perm   os.FileMode
	logger *slog.Logger
}

// FileWriterOption configures a FileWriter.
type FileWriterOption func(*FileWriter)

// WithPermissions overrides the default file permissions (0644).
func WithPermissions(perm os.FileMode) FileWriterOption {
	return func(fw *FileWriter) {
		fw.perm = perm
	}
}

// WithLogger sets a logger for the FileWriter.
func WithLogger(logger *slog.Logger) FileWriterOption {
	return func(fw *FileWriter) {
		fw.logger = logger
	}
}

// NewFileWriter creates a writer that writes to the specified file path.
func NewFileWriter(path string, opts ...FileWriterOption) *FileWriter {
	fw := &FileWriter{
		path:   path,
		perm:   0o644,
		logger: slog.Default(),
	}

	for _, opt := range opts {
		opt(fw)
	}

	return fw
}

// Write creates parent directories and writes data to the file.
func (fw *FileWriter) Write(data []byte) error {
	dir := filepath.Dir(fw.path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	// Check if file exists for warning.
	if _, err := os.Stat(fw.path); err == nil {
		fw.logger.Warn("overwriting existing file", slog.String("path", fw.path))
	}

	if err := os.WriteFile(fw.path, data, fw.perm); err != nil {
		return fmt.Errorf("writing file %s: %w", fw.path, err)
	}

	return nil
}

// Path returns the output file path.
func (fw *FileWriter) Path() string {
	return fw.path
}
