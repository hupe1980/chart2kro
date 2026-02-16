package transform_test

import (
	"testing"

	"github.com/hupe1980/chart2kro/internal/k8s"
	"github.com/hupe1980/chart2kro/internal/transform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssignResourceIDs_SingleOfEachKind(t *testing.T) {
	resources := []*k8s.Resource{
		makeResource("Deployment", "nginx"),
		makeResource("Service", "nginx-svc"),
		makeResource("ConfigMap", "config"),
	}
	ids, err := transform.AssignResourceIDs(resources, nil)
	require.NoError(t, err)
	assert.Equal(t, "deployment", ids[resources[0]])
	assert.Equal(t, "service", ids[resources[1]])
	assert.Equal(t, "configmap", ids[resources[2]])
}

func TestAssignResourceIDs_MultipleSameKind(t *testing.T) {
	resources := []*k8s.Resource{
		makeResource("Service", "app-main"),
		makeResource("Service", "app-headless"),
	}
	ids, err := transform.AssignResourceIDs(resources, nil)
	require.NoError(t, err)
	assert.Equal(t, "service-main", ids[resources[0]])
	assert.Equal(t, "service-headless", ids[resources[1]])
}

func TestAssignResourceIDs_Overrides(t *testing.T) {
	resources := []*k8s.Resource{
		makeResource("Deployment", "nginx"),
		makeResource("Service", "nginx-svc"),
	}
	overrides := map[string]string{
		"Deployment/nginx": "my-deploy",
	}
	ids, err := transform.AssignResourceIDs(resources, overrides)
	require.NoError(t, err)
	assert.Equal(t, "my-deploy", ids[resources[0]])
	assert.Equal(t, "service", ids[resources[1]])
}

func TestAssignResourceIDs_Sanitization(t *testing.T) {
	resources := []*k8s.Resource{
		makeResource("Deployment", "My_App!Test"),
	}
	ids, err := transform.AssignResourceIDs(resources, nil)
	require.NoError(t, err)
	assert.Equal(t, "deployment", ids[resources[0]])
}

func TestAssignResourceIDs_OverrideSanitization(t *testing.T) {
	resources := []*k8s.Resource{
		makeResource("ConfigMap", "data"),
	}
	overrides := map[string]string{
		"ConfigMap/data": "MY--BAD__ID",
	}
	ids, err := transform.AssignResourceIDs(resources, overrides)
	require.NoError(t, err)
	assert.Equal(t, "my-bad-id", ids[resources[0]])
}

func TestAssignResourceIDs_Empty(t *testing.T) {
	ids, err := transform.AssignResourceIDs(nil, nil)
	require.NoError(t, err)
	require.Empty(t, ids)
}

func TestSanitizeViaAssign(t *testing.T) {
	resources := []*k8s.Resource{
		makeResource("Service", "test"),
	}
	overrides := map[string]string{
		"Service/test": "---UPPER---Case---",
	}
	ids, err := transform.AssignResourceIDs(resources, overrides)
	require.NoError(t, err)
	assert.Equal(t, "upper-case", ids[resources[0]])
}

func TestToCamelCase(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"service.port", "servicePort"},
		{"image.pull-policy", "imagePullPolicy"},
		{"simple", "simple"},
		{"a.b.c", "aBC"},
		{"my_value", "myValue"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, transform.ToCamelCase(tt.input))
		})
	}
}

func TestToPascalCase(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"my-chart", "MyChart"},
		{"nginx", "Nginx"},
		{"hello_world", "HelloWorld"},
		{"multi.word.name", "MultiWordName"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, transform.ToPascalCase(tt.input))
		})
	}
}

func TestAssignResourceIDs_CollisionDetection(t *testing.T) {
	// Two services whose last name segment collides after sanitization.
	resources := []*k8s.Resource{
		makeResource("Service", "my-web"),
		makeResource("Service", "your-web"),
	}

	_, err := transform.AssignResourceIDs(resources, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "collision")
}

func TestToCamelCase_Empty(t *testing.T) {
	assert.Equal(t, "", transform.ToCamelCase(""))
}

func TestToPascalCase_Empty(t *testing.T) {
	assert.Equal(t, "", transform.ToPascalCase(""))
}

func TestToPascalCase_OnlyDelimiters(t *testing.T) {
	assert.Equal(t, "", transform.ToPascalCase("---"))
}
