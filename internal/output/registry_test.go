package output

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Registry
// ---------------------------------------------------------------------------

func TestRegistry_Register_And_Lookup(t *testing.T) {
	r := NewRegistry()

	var buf bytes.Buffer
	r.Register("test", func(_ string) Writer {
		return NewStdoutWriter(&buf)
	})

	factory, err := r.Writer("test")
	require.NoError(t, err)

	w := factory("")
	require.NoError(t, w.Write([]byte("hello")))
	assert.Equal(t, "hello", buf.String())
}

func TestRegistry_UnknownFormat(t *testing.T) {
	r := NewRegistry()

	_, err := r.Writer("xml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown output format")
	assert.Contains(t, err.Error(), "xml")
}

func TestRegistry_Formats(t *testing.T) {
	r := NewRegistry()
	r.Register("json", func(_ string) Writer { return NewStdoutWriter(nil) })
	r.Register("yaml", func(_ string) Writer { return NewStdoutWriter(nil) })
	r.Register("csv", func(_ string) Writer { return NewStdoutWriter(nil) })

	formats := r.Formats()
	assert.Equal(t, []string{"csv", "json", "yaml"}, formats)
}

func TestRegistry_Overwrite(t *testing.T) {
	r := NewRegistry()

	var buf1, buf2 bytes.Buffer
	r.Register("fmt", func(_ string) Writer { return NewStdoutWriter(&buf1) })
	r.Register("fmt", func(_ string) Writer { return NewStdoutWriter(&buf2) })

	factory, err := r.Writer("fmt")
	require.NoError(t, err)

	w := factory("")
	require.NoError(t, w.Write([]byte("data")))
	assert.Empty(t, buf1.String(), "old writer should NOT receive data")
	assert.Equal(t, "data", buf2.String(), "new writer should receive data")
}

func TestDefaultRegistry(t *testing.T) {
	r := DefaultRegistry()

	formats := r.Formats()
	assert.Contains(t, formats, "yaml")
	assert.Contains(t, formats, "json")
	assert.Contains(t, formats, "stdout")
	assert.Contains(t, formats, "file")
}

func TestDefaultRegistry_YAMLWriterStdout(t *testing.T) {
	r := DefaultRegistry()

	factory, err := r.Writer("yaml")
	require.NoError(t, err)

	w := factory("")
	assert.IsType(t, &StdoutWriter{}, w)
}

func TestDefaultRegistry_YAMLWriterFile(t *testing.T) {
	r := DefaultRegistry()

	factory, err := r.Writer("yaml")
	require.NoError(t, err)

	w := factory("/tmp/test.yaml")
	assert.IsType(t, &FileWriter{}, w)
}

func TestDefaultRegistry_StdoutAlwaysStdout(t *testing.T) {
	r := DefaultRegistry()

	factory, err := r.Writer("stdout")
	require.NoError(t, err)

	w := factory("/ignored/path")
	assert.IsType(t, &StdoutWriter{}, w)
}

func TestRegistry_ErrorMessage_ListsFormats(t *testing.T) {
	r := NewRegistry()
	r.Register("a", func(_ string) Writer { return NewStdoutWriter(nil) })
	r.Register("b", func(_ string) Writer { return NewStdoutWriter(nil) })

	_, err := r.Writer("c")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a, b")
}
