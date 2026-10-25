package rgd

import (
	"context"
	"testing"
	"unicode/utf8"

	"github.com/hupe1980/chart2kro/internal/kroval"
	"github.com/hupe1980/chart2kro/internal/oracle"
)

// FuzzEscapedLiteralRoundTrip: chart content is untrusted, so any string written into
// a field, alone or next to an expression, must come back from kro's runtime unchanged
// and never be evaluated. The seeds run in every `go test`; fuzz with
// `go test ./internal/rgd -run '^$' -fuzz FuzzEscapedLiteralRoundTrip` when touching escaping.
func FuzzEscapedLiteralRoundTrip(f *testing.F) {
	for _, s := range []string{
		"", "plain", "${schema.spec.x}", "$${x}", "${", "}", "${\"${\"}", `"quoted"`, `back\slash`,
		"multi\nline", "tab\there", "ünïcödé", "{{ .Values.x }}", "a=${b} c=${d}", "${}", "$", "${ ",
	} {
		f.Add(s)
	}

	v, err := kroval.New("", nil)
	if err != nil {
		f.Fatal(err)
	}

	f.Fuzz(func(t *testing.T, s string) {
		if !utf8.ValidString(s) || !yamlPrintable(s) {
			t.Skip("outside YAML's printable characters: kro rejects such templates loudly (exit 7)")
		}

		withName := template(segments(s+"-", []hole{{start: len(s) + 1, end: len(s) + 1, expr: refName, str: true}}))

		g, err := v.Validate(context.Background(), rgdWith(nil, configMap(map[string]any{
			"alone": escapeLiteral(s),
			"mixed": withName,
		})))
		if err != nil {
			t.Fatalf("kro rejects the escaped literal %q: %v", s, err)
		}

		nodes, err := oracle.Instantiate(context.Background(), g, "inst", "ns", nil)
		if err != nil {
			t.Fatalf("kro cannot evaluate the escaped literal %q: %v", s, err)
		}

		data := nodes[0].Objects[0]["data"].(map[string]any)

		if got := data["alone"]; got != s {
			t.Fatalf("alone: %q came back as %q", s, got)
		}

		if got, want := data["mixed"], s+"-inst"; got != want {
			t.Fatalf("mixed: want %q, got %q", want, got)
		}
	})
}

// yamlPrintable reports whether s uses only YAML 1.2's printable characters (c-printable).
func yamlPrintable(s string) bool {
	for _, r := range s {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
		case r >= 0x20 && r <= 0x7e, r == 0x85:
		case r >= 0xa0 && r <= 0xd7ff, r >= 0xe000 && r <= 0xfffd, r >= 0x10000 && r <= 0x10ffff:
		default:
			return false
		}
	}

	return true
}
