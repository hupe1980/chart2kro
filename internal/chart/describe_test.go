package chart

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/chart/common"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
)

func withValues(name, values string, deps ...*chartv2.Chart) *chartv2.Chart {
	ch := &chartv2.Chart{Metadata: &chartv2.Metadata{Name: name}, Raw: []*common.File{{Name: "values.yaml", Data: []byte(values)}}}
	for _, d := range deps {
		ch.AddDependency(d)
	}

	return ch
}

func TestDescriptions(t *testing.T) {
	values := `
# -- Number of replicas
replicaCount: 1

image:
  # -- Image repository,
  # without the tag
  # @default -- nginx
  repository: nginx
  ## @param image.tag Image tag (immutable tags recommended)
  tag: "1.27"

# The Service type.
# Use LoadBalancer only in clouds.
serviceType: ClusterIP

# resources:
#   limits:
#     cpu: 100m
resources: {}

## @param both Bitnami text
# -- helm-docs text wins
both: x

long: y # line comments are not descriptions
`

	sub := withValues("postgresql", "# -- Database name\ndatabase: app\n")
	ch := withValues("app", values, sub)
	ch.Metadata.Dependencies = []*chartv2.Dependency{{Name: "postgresql", Alias: "db"}}

	d := Descriptions(ch)

	require.Equal(t, "Number of replicas", d["replicaCount"])
	require.Equal(t, "Image repository, without the tag", d["image.repository"])
	require.Equal(t, "Image tag (immutable tags recommended)", d["image.tag"])
	require.Equal(t, "The Service type. Use LoadBalancer only in clouds.", d["serviceType"])
	require.NotContains(t, d, "resources", "commented-out YAML is not a description")
	require.Equal(t, "helm-docs text wins", d["both"])
	require.NotContains(t, d, "long")
	require.Equal(t, "Database name", d["db.database"], "subcharts under their alias")
}

func TestDescriptionIsCut(t *testing.T) {
	d := Descriptions(withValues("app", "# -- "+strings.Repeat("word ", 100)+"\nx: 1\n"))
	require.Equal(t, MaxDescription, len([]rune(d["x"])))
	require.True(t, strings.HasSuffix(d["x"], "…"))
}
