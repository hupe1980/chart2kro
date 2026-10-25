package probe

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestReasonCodesAreDocumented parses every Reason* constant and requires it in the
// troubleshooting guide: the codes are a public contract.
func TestReasonCodesAreDocumented(t *testing.T) {
	doc, err := os.ReadFile("../../site/content/docs/troubleshooting.md")
	require.NoError(t, err)

	f, err := parser.ParseFile(token.NewFileSet(), "classify.go", nil, 0)
	require.NoError(t, err)

	n := 0

	ast.Inspect(f, func(node ast.Node) bool {
		vs, ok := node.(*ast.ValueSpec)
		if !ok || len(vs.Values) != 1 || !strings.HasPrefix(vs.Names[0].Name, "Reason") {
			return true
		}

		lit, ok := vs.Values[0].(*ast.BasicLit)
		if !ok {
			return true
		}

		code, _ := strconv.Unquote(lit.Value)
		require.Contains(t, string(doc), "`"+code+"`", vs.Names[0].Name)

		n++

		return true
	})

	require.Positive(t, n)
}
