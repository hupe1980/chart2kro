// Command minify turns raw OpenAPI v3 group-version documents into the embedded catalog
// form: components only, string descriptions removed, gzipped.
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func strip(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, x := range t {
			if _, isStr := x.(string); k == "description" && isStr {
				continue
			}
			out[k] = strip(x)
		}
		return out
	case []any:
		for i := range t {
			t[i] = strip(t[i])
		}
		return t
	default:
		return v
	}
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: minify SRC_DIR DST_DIR")
		os.Exit(2)
	}

	src, dst := os.Args[1], os.Args[2]
	if err := os.MkdirAll(dst, 0o750); err != nil {
		panic(err)
	}

	files, _ := filepath.Glob(filepath.Join(src, "*_openapi.json"))
	total := 0

	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			panic(err)
		}

		var doc map[string]any
		if err := json.Unmarshal(b, &doc); err != nil {
			panic(fmt.Errorf("%s: %w", f, err))
		}

		out, _ := json.Marshal(strip(map[string]any{"openapi": doc["openapi"], "components": doc["components"]}))

		var gz bytes.Buffer
		w, _ := gzip.NewWriterLevel(&gz, gzip.BestCompression)
		_, _ = w.Write(out)
		_ = w.Close()

		name := filepath.Join(dst, strings.TrimSuffix(filepath.Base(f), "")+".gz")
		if err := os.WriteFile(name, gz.Bytes(), 0o600); err != nil {
			panic(err)
		}

		total += gz.Len()
	}

	fmt.Printf("%d documents, %d bytes\n", len(files), total)
}
