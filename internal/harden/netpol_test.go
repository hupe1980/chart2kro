package harden

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/chart2kro/internal/k8s"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestNetworkPolicyGenerator_GeneratesPerWorkload(t *testing.T) {
	labels := map[string]interface{}{"app": "web"}
	deploy := makeDeploymentWithSelector("web", labels, []interface{}{makeContainer("web", "nginx:1.25")})

	svc := makeService("web-svc", labels, []interface{}{
		map[string]interface{}{
			"port":     int64(80),
			"protocol": "TCP",
		},
	})

	gen := NewNetworkPolicyGenerator(nil)
	result := &HardenResult{Resources: []*k8s.Resource{deploy, svc}}

	err := gen.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)

	// Should generate 1 NetworkPolicy for the deployment.
	var netpols []*k8s.Resource

	for _, res := range result.Resources {
		if res.Kind() == "NetworkPolicy" {
			netpols = append(netpols, res)
		}
	}

	assert.Len(t, netpols, 1)
	assert.Equal(t, "web-netpol", netpols[0].Name)
}

func TestNetworkPolicyGenerator_DenyAllDefault(t *testing.T) {
	deploy := makeDeploymentWithSelector("app", map[string]interface{}{"app": "test"}, []interface{}{makeContainer("web", "nginx:1.25")})

	gen := NewNetworkPolicyGenerator(nil)
	result := &HardenResult{Resources: []*k8s.Resource{deploy}}

	err := gen.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)

	var netpol *k8s.Resource

	for _, res := range result.Resources {
		if res.Kind() == "NetworkPolicy" {
			netpol = res
			break
		}
	}

	require.NotNil(t, netpol)

	spec := netpol.Object.Object["spec"].(map[string]interface{})
	policyTypes := spec["policyTypes"].([]interface{})
	assert.Contains(t, policyTypes, "Ingress")
	assert.Contains(t, policyTypes, "Egress")
}

func TestNetworkPolicyGenerator_DNSEgressAllowed(t *testing.T) {
	deploy := makeDeploymentWithSelector("app", map[string]interface{}{"app": "test"}, []interface{}{makeContainer("web", "nginx:1.25")})

	gen := NewNetworkPolicyGenerator(nil)
	result := &HardenResult{Resources: []*k8s.Resource{deploy}}

	err := gen.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)

	var netpol *k8s.Resource

	for _, res := range result.Resources {
		if res.Kind() == "NetworkPolicy" {
			netpol = res
			break
		}
	}

	require.NotNil(t, netpol)

	spec := netpol.Object.Object["spec"].(map[string]interface{})
	egress := spec["egress"].([]interface{})
	assert.Len(t, egress, 1, "should have DNS egress rule")

	dnsRule := egress[0].(map[string]interface{})
	ports := dnsRule["ports"].([]interface{})
	assert.Len(t, ports, 2, "should allow UDP and TCP DNS") // port 53 UDP + TCP
}

func TestNetworkPolicyGenerator_ServiceMatchingIngress(t *testing.T) {
	labels := map[string]interface{}{"app": "web"}
	deploy := makeDeploymentWithSelector("web", labels, []interface{}{makeContainer("web", "nginx:1.25")})

	svc := makeService("web-svc", labels, []interface{}{
		map[string]interface{}{
			"port":     int64(8080),
			"protocol": "TCP",
		},
	})

	gen := NewNetworkPolicyGenerator(nil)
	result := &HardenResult{Resources: []*k8s.Resource{deploy, svc}}

	err := gen.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)

	var netpol *k8s.Resource

	for _, res := range result.Resources {
		if res.Kind() == "NetworkPolicy" {
			netpol = res
			break
		}
	}

	require.NotNil(t, netpol)

	spec := netpol.Object.Object["spec"].(map[string]interface{})
	ingress, hasIngress := spec["ingress"].([]interface{})
	assert.True(t, hasIngress, "should have ingress rules from matching service")
	assert.Len(t, ingress, 1)

	rule := ingress[0].(map[string]interface{})
	ports := rule["ports"].([]interface{})
	assert.Len(t, ports, 1)

	port := ports[0].(map[string]interface{})
	assert.Equal(t, int64(8080), port["port"])
}

func TestNetworkPolicyGenerator_NoServiceNoIngress(t *testing.T) {
	deploy := makeDeploymentWithSelector("app", map[string]interface{}{"app": "test"}, []interface{}{makeContainer("web", "nginx:1.25")})

	gen := NewNetworkPolicyGenerator(nil)
	result := &HardenResult{Resources: []*k8s.Resource{deploy}}

	err := gen.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)

	var netpol *k8s.Resource

	for _, res := range result.Resources {
		if res.Kind() == "NetworkPolicy" {
			netpol = res
			break
		}
	}

	require.NotNil(t, netpol)

	spec := netpol.Object.Object["spec"].(map[string]interface{})
	_, hasIngress := spec["ingress"]
	assert.False(t, hasIngress, "should not have ingress when no services match")
}

func TestNetworkPolicyGenerator_ServiceWithoutPorts(t *testing.T) {
	labels := map[string]interface{}{"app": "web"}
	deploy := makeDeploymentWithSelector("web", labels, []interface{}{makeContainer("web", "nginx:1.25")})

	// Service has matching selector but no ports.
	svc := makeService("web-svc", labels, nil)

	gen := NewNetworkPolicyGenerator(nil)
	result := &HardenResult{Resources: []*k8s.Resource{deploy, svc}}

	err := gen.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)

	var netpol *k8s.Resource

	for _, res := range result.Resources {
		if res.Kind() == "NetworkPolicy" {
			netpol = res
			break
		}
	}

	require.NotNil(t, netpol)

	spec := netpol.Object.Object["spec"].(map[string]interface{})
	_, hasIngress := spec["ingress"]
	assert.False(t, hasIngress, "should not have ingress when service has no ports")
}

func TestNetworkPolicyGenerator_Name(t *testing.T) {
	gen := NewNetworkPolicyGenerator(nil)
	assert.Equal(t, "network-policy-generator", gen.Name())
}

func TestSelectorsOverlap(t *testing.T) {
	tests := []struct {
		name   string
		svc    map[string]interface{}
		pod    map[string]interface{}
		expect bool
	}{
		{"match", map[string]interface{}{"app": "web"}, map[string]interface{}{"app": "web"}, true},
		{"no match", map[string]interface{}{"app": "api"}, map[string]interface{}{"app": "web"}, false},
		{"subset match", map[string]interface{}{"app": "web"}, map[string]interface{}{"app": "web", "tier": "frontend"}, true},
		{"empty svc", map[string]interface{}{}, map[string]interface{}{"app": "web"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, selectorsOverlap(tt.svc, tt.pod))
		})
	}
}

func TestNetworkPolicyGenerator_MultipleWorkloads(t *testing.T) {
	deploy1 := makeDeploymentWithSelector("web", map[string]interface{}{"app": "web"}, []interface{}{makeContainer("web", "nginx:1.25")})
	deploy2 := makeDeploymentWithSelector("api", map[string]interface{}{"app": "api"}, []interface{}{makeContainer("api", "node:18")})

	gen := NewNetworkPolicyGenerator(nil)
	result := &HardenResult{Resources: []*k8s.Resource{deploy1, deploy2}}

	err := gen.Apply(context.Background(), result.Resources, result)
	require.NoError(t, err)

	var netpols int
	for _, res := range result.Resources {
		if res.Kind() == "NetworkPolicy" {
			netpols++
		}
	}

	assert.Equal(t, 2, netpols, "should generate one NetworkPolicy per workload")
}

func TestExtractMatchLabels_FallbackToName(t *testing.T) {
	// Workload without spec.selector.matchLabels → should fall back to {"app": name}.
	deploy := makeDeployment("my-app", []interface{}{makeContainer("web", "nginx:1.25")})

	labels := extractMatchLabels(deploy)
	assert.Equal(t, map[string]interface{}{"app": "my-app"}, labels)
}

func TestExtractServiceSelector_NilObject(t *testing.T) {
	svc := &k8s.Resource{
		GVK:  schema.GroupVersionKind{Version: "v1", Kind: "Service"},
		Name: "svc",
	}

	assert.Nil(t, extractServiceSelector(svc))
}

func TestExtractServicePorts_NoSpec(t *testing.T) {
	svc := &k8s.Resource{
		GVK:  schema.GroupVersionKind{Version: "v1", Kind: "Service"},
		Name: "svc",
		Object: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Service",
				"metadata":   map[string]interface{}{"name": "svc"},
			},
		},
	}

	ports := extractServicePorts(svc)
	assert.Nil(t, ports)
}

func TestExtractMatchLabels_NilObject(t *testing.T) {
	deploy := &k8s.Resource{
		GVK:  schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		Name: "orphan",
	}

	labels := extractMatchLabels(deploy)
	assert.Equal(t, map[string]interface{}{"app": "orphan"}, labels)
}
